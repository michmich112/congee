package plugin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func installFromURL(destRoot, url, wantSHA string) (*Manifest, string, error) {
	man, pkgDir, cleanup, err := fetchAndExtractURL(url, wantSHA)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()
	final := filepath.Join(destRoot, man.ID)
	if err := replacePackageKeepData(pkgDir, final); err != nil {
		return nil, "", err
	}
	return man, final, nil
}

func fetchAndExtractURL(url, wantSHA string) (*Manifest, string, func(), error) {
	nop := func() {}
	tmp, err := os.MkdirTemp("", "congee-plugin-dl-*")
	if err != nil {
		return nil, "", nop, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	fail := func(err error) (*Manifest, string, func(), error) {
		cleanup()
		return nil, "", nop, err
	}
	archive := filepath.Join(tmp, "pkg")
	if err := downloadFile(url, archive); err != nil {
		return fail(err)
	}
	sum, err := fileSHA256(archive)
	if err != nil {
		return fail(err)
	}
	if strings.TrimSpace(wantSHA) == "" {
		return fail(fmt.Errorf("sha256 required for url install"))
	}
	if !strings.EqualFold(sum, strings.TrimSpace(wantSHA)) {
		return fail(fmt.Errorf("sha256 mismatch: got %s want %s", sum, strings.TrimSpace(wantSHA)))
	}
	extractDir := filepath.Join(tmp, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fail(err)
	}
	if err := extractArchive(archive, extractDir); err != nil {
		return fail(err)
	}
	pkgDir, err := findPackageRoot(extractDir)
	if err != nil {
		return fail(err)
	}
	man, err := loadManifest(pkgDir)
	if err != nil {
		return fail(err)
	}
	return man, pkgDir, cleanup, nil
}

func pluginDirExists(root, id string) bool {
	if root == "" || id == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(root, id))
	return err == nil && st.IsDir()
}

func installFromDir(destRoot, src string) (*Manifest, string, error) {
	man, err := loadManifest(src)
	if err != nil {
		return nil, "", err
	}
	final := filepath.Join(destRoot, man.ID)
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return nil, "", err
	}
	absFinal, err := filepath.Abs(final)
	if err != nil {
		return nil, "", err
	}
	if absSrc == absFinal {
		return man, final, nil
	}
	if err := replacePackageKeepData(src, final); err != nil {
		return nil, "", err
	}
	return man, final, nil
}

// replacePackageKeepData copies src over dest, preserving dest/data across upgrades.
func replacePackageKeepData(src, final string) error {
	dataDir := filepath.Join(final, "data")
	var keep string
	if st, err := os.Stat(dataDir); err == nil && st.IsDir() {
		tmp, err := os.MkdirTemp(filepath.Dir(final), "plugin-data-*")
		if err != nil {
			return err
		}
		keep = filepath.Join(tmp, "data")
		if err := os.Rename(dataDir, keep); err != nil {
			_ = os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.RemoveAll(final); err != nil {
		if keep != "" {
			_ = os.MkdirAll(final, 0o755)
			_ = os.Rename(keep, dataDir)
		}
		return err
	}
	if err := copyDir(src, final); err != nil {
		if keep != "" {
			_ = os.MkdirAll(final, 0o755)
			_ = os.Rename(keep, filepath.Join(final, "data"))
		}
		return err
	}
	if keep != "" {
		_ = os.RemoveAll(filepath.Join(final, "data"))
		if err := os.MkdirAll(final, 0o755); err != nil {
			return err
		}
		if err := os.Rename(keep, filepath.Join(final, "data")); err != nil {
			return err
		}
		_ = os.RemoveAll(filepath.Dir(keep))
	}
	return nil
}

func downloadFile(url, dest string) error {
	c := &http.Client{Timeout: 120 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractArchive(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 4)
	n, _ := f.Read(head)
	_, _ = f.Seek(0, io.SeekStart)
	if n >= 2 && head[0] == 0x1f && head[1] == 0x8b {
		return extractTarGz(f, dest)
	}
	if n >= 4 && head[0] == 'P' && head[1] == 'K' {
		_ = f.Close()
		return extractZip(archive, dest)
	}
	// uncompressed tar
	return extractTar(f, dest)
}

func extractTarGz(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	return extractTar(gz, dest)
}

func extractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := writeTarEntry(dest, hdr, tr); err != nil {
			return err
		}
	}
}

func writeTarEntry(dest string, hdr *tar.Header, r io.Reader) error {
	name := filepath.Clean(hdr.Name)
	if strings.HasPrefix(name, "..") {
		return fmt.Errorf("tar path escapes: %s", hdr.Name)
	}
	target := filepath.Join(dest, name)
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return err
		}
		_, err = io.Copy(f, r)
		_ = f.Close()
		return err
	default:
		return nil
	}
}

func extractZip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") {
			return fmt.Errorf("zip path escapes: %s", f.Name)
		}
		target := filepath.Join(dest, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func findPackageRoot(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "plugin.json")); err == nil {
		return dir, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(p, "plugin.json")); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("plugin.json not found in archive")
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		base := filepath.Base(path)
		if base == ".git" || base == "node_modules" || base == ".svelte-kit" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		_ = out.Close()
		return err
	})
}
