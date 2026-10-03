package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ImageSource returns the canonical source for icon or banner.
// Empty source with a URL is url; any other empty source is default.
func (n NIP11Section) ImageSource(asset string) string {
	source, raw := n.imageField(asset)
	source = strings.TrimSpace(source)
	switch source {
	case NIP11ImageSourceDefault, NIP11ImageSourceUpload, NIP11ImageSourceURL:
		return source
	default:
		if strings.TrimSpace(raw) != "" {
			return NIP11ImageSourceURL
		}
		return NIP11ImageSourceDefault
	}
}

func (n NIP11Section) imageField(asset string) (source, raw string) {
	if asset == NIP11AssetBanner {
		return n.BannerSource, n.Banner
	}
	return n.IconSource, n.Icon
}

// NIP11AssetDir is the directory beside the JSON config that holds uploaded images.
func NIP11AssetDir(cfgPath string) string {
	return filepath.Join(filepath.Dir(cfgPath), "nip11-assets")
}

// NIP11AssetFile is nip11-assets/icon or nip11-assets/banner next to the JSON config.
func NIP11AssetFile(cfgPath, asset string) (string, error) {
	if strings.TrimSpace(cfgPath) == "" {
		return "", errors.New("config: path is empty")
	}
	switch asset {
	case NIP11AssetIcon, NIP11AssetBanner:
	default:
		return "", fmt.Errorf("config: unknown nip11 asset %q", asset)
	}
	return filepath.Join(NIP11AssetDir(cfgPath), asset), nil
}

// WriteNIP11Asset stores uploaded image bytes next to the JSON config.
func WriteNIP11Asset(cfgPath, asset string, data []byte) error {
	path, err := NIP11AssetFile(cfgPath, asset)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return WriteFileAtomic(path, data)
}

// PruneNIP11Assets removes stored images whose source is no longer upload.
func PruneNIP11Assets(cfgPath string, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	var errs []error
	if cfg.NIP11.ImageSource(NIP11AssetIcon) != NIP11ImageSourceUpload {
		errs = append(errs, removeNIP11Asset(cfgPath, NIP11AssetIcon))
	}
	if cfg.NIP11.ImageSource(NIP11AssetBanner) != NIP11ImageSourceUpload {
		errs = append(errs, removeNIP11Asset(cfgPath, NIP11AssetBanner))
	}
	return errors.Join(errs...)
}

// RequireNIP11UploadFiles rejects source=upload when the stored file is missing or over the cap.
func RequireNIP11UploadFiles(cfgPath string, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	for _, asset := range []string{NIP11AssetIcon, NIP11AssetBanner} {
		if cfg.NIP11.ImageSource(asset) != NIP11ImageSourceUpload {
			continue
		}
		path, err := NIP11AssetFile(cfgPath, asset)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("config: nip11 %s source is upload but no image is stored", asset)
			}
			return err
		}
		max, err := NIP11UploadMax(asset)
		if err != nil {
			return err
		}
		if info.Size() == 0 || info.Size() > int64(max) {
			return fmt.Errorf("config: nip11 %s upload is missing or exceeds %d bytes", asset, max)
		}
	}
	return nil
}

func removeNIP11Asset(cfgPath, asset string) error {
	path, err := NIP11AssetFile(cfgPath, asset)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ValidateNIP11Upload accepts png, jpeg, and webp within the per-asset size cap.
func ValidateNIP11Upload(asset string, data []byte) error {
	max, err := NIP11UploadMax(asset)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("config: nip11 upload is empty")
	}
	if len(data) > max {
		return fmt.Errorf("config: nip11 %s upload exceeds %d bytes", asset, max)
	}
	if looksLikeSVG(data) {
		return errors.New("config: nip11 uploads must be png, jpeg, or webp")
	}
	switch SniffNIP11ImageType(data) {
	case "image/png", "image/jpeg", "image/webp":
		return nil
	default:
		return errors.New("config: nip11 uploads must be png, jpeg, or webp")
	}
}

// NIP11UploadMax is the byte cap for an uploaded icon or banner.
func NIP11UploadMax(asset string) (int, error) {
	switch asset {
	case NIP11AssetIcon:
		return NIP11IconMaxBytes, nil
	case NIP11AssetBanner:
		return NIP11BannerMaxBytes, nil
	default:
		return 0, fmt.Errorf("config: unknown nip11 asset %q", asset)
	}
}

// SniffNIP11ImageType reports a content type Congee will serve, or empty when unknown.
func SniffNIP11ImageType(data []byte) string {
	if looksLikeSVG(data) {
		return "image/svg+xml"
	}
	ct := http.DetectContentType(data)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	switch ct {
	case "image/png", "image/jpeg", "image/webp":
		return ct
	default:
		return ""
	}
}

func looksLikeSVG(data []byte) bool {
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	t := bytes.TrimSpace(sample)
	if bytes.HasPrefix(t, []byte{0xEF, 0xBB, 0xBF}) {
		t = bytes.TrimSpace(t[3:])
	}
	lower := bytes.ToLower(t)
	if bytes.HasPrefix(lower, []byte("<?xml")) {
		return bytes.Contains(lower, []byte("<svg"))
	}
	return bytes.HasPrefix(lower, []byte("<svg"))
}

func validateAbsoluteHTTPURL(field, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("config: %s must be an absolute http or https URL", field)
	}
	return raw, nil
}
