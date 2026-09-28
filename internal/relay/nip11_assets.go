package relay

import (
	_ "embed"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/michmich112/congee/internal/config"
	"github.com/rs/zerolog"
)

//go:embed assets/congee-logo.svg
var defaultIconSVG []byte

//go:embed assets/default-banner.svg
var defaultBannerSVG []byte

func (s *Server) serveNIP11Asset(w http.ResponseWriter, r *http.Request, asset string) {
	if s.cfg != nil && s.cfg.NIP11.CORSAllowAnyOrigin {
		writeNIP11CORSResponse(w)
	}
	source := config.NIP11ImageSourceDefault
	if s.cfg != nil {
		source = s.cfg.NIP11.ImageSource(asset)
	}
	if source == config.NIP11ImageSourceURL {
		http.NotFound(w, r)
		return
	}
	data, contentType, err := s.nip11AssetBytes(asset, source)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) nip11AssetBytes(asset, source string) ([]byte, string, error) {
	if source == config.NIP11ImageSourceUpload {
		path, err := config.NIP11AssetFile(s.configPath, asset)
		if err != nil {
			s.log.Warn().Err(err).Str("asset", asset).Msg("nip11 upload path")
			return nil, "", err
		}
		data, err := readNIP11Upload(path, asset)
		if err != nil {
			if os.IsNotExist(err) {
				s.log.Warn().Str("path", path).Str("asset", asset).Msg("nip11 upload file missing")
			} else {
				s.log.Warn().Err(err).Str("path", path).Str("asset", asset).Msg("nip11 upload read failed")
			}
			return nil, "", err
		}
		contentType := config.SniffNIP11ImageType(data)
		if contentType == "" || contentType == "image/svg+xml" {
			s.log.Warn().Str("path", path).Str("asset", asset).Msg("nip11 upload is not png, jpeg, or webp")
			return nil, "", errors.New("nip11 upload type")
		}
		return data, contentType, nil
	}
	if asset == config.NIP11AssetBanner {
		return defaultBannerSVG, "image/svg+xml", nil
	}
	return defaultIconSVG, "image/svg+xml", nil
}

func readNIP11Upload(path, asset string) ([]byte, error) {
	max, err := config.NIP11UploadMax(asset)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > int64(max) {
		return nil, errors.New("nip11 upload exceeds size cap")
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > max {
		return nil, errors.New("nip11 upload exceeds size cap")
	}
	return data, nil
}

// ReadNIP11Preview returns the bytes the relay would serve for a hosted icon or banner.
// External URL mode returns ErrNIP11ImageExternal.
func ReadNIP11Preview(cfgPath string, section config.NIP11Section, asset string) ([]byte, string, error) {
	s := &Server{configPath: cfgPath, log: zerolog.Nop()}
	source := section.ImageSource(asset)
	if source == config.NIP11ImageSourceURL {
		return nil, "", ErrNIP11ImageExternal
	}
	return s.nip11AssetBytes(asset, source)
}

// ErrNIP11ImageExternal means the image is an operator URL, not a hosted file.
var ErrNIP11ImageExternal = errors.New("nip11 image is an external url")
