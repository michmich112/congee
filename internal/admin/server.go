// Package admin implements the optional HTTP admin server when ENABLE_ADMIN_UI is set.
//
// Authenticated JSON API (Bearer ADMIN_PASSWORD or X-Admin-Token) under /api/:
//
//	GET    /api/config           — raw JSON file
//	PUT    /api/config           — replace file (validated, atomic, changelog)
//	GET    /api/config/changelog — recent config changes (?limit=)
//	GET    /api/audit            — audit rows (?limit,&offset,&since,&until,&action,&pubkey,&kind); JSON {entries,total}
//	GET    /api/audit/kinds      — distinct kinds from recent audit rows (?scan_limit=); JSON {kinds:[]int}
//	GET    /api/events/{id}     — single stored Nostr event by hex id (404 if not in DB)
//	GET    /api/nips             — known NIPs + enabled flags
//	PATCH  /api/nips             — body {"nip":N,"enabled":bool}; response includes restart_required
//	GET    /api/stats            — relay connection count, ports, relay_version (binary)
//	GET    /api/relay-identity   — relay pubkey_hex and npub (read-only)
//	POST   /api/migration/start  — copy sqlite↔postgres with SSE progress; optional make_target_primary to rewrite config
//
// Non-API GET requests: CONGEE_ENV dev|development|local reverse-proxies to http://127.0.0.1:5173;
// if Vite is unreachable, falls back to web/admin/build when index.html exists (e.g. after make ui-build).
// Otherwise static files from web/admin/build with SPA fallback to index.html.
package admin

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// Server is the standalone admin HTTP server (API + static UI or Vite proxy).
//
// All JSON APIs are mounted under /api/ and require auth when ENABLE_ADMIN_UI is on
// (see RequireAdminAuth): Authorization: Bearer <ADMIN_PASSWORD> or X-Admin-Token.
//
// Endpoints (strip prefix /api):
//
//	GET/PUT  /config           — raw JSON file; PUT validates, atomic write, changelog row
//	GET      /config/changelog — recent config change records (?limit=)
//	GET      /audit            — audit log (?limit,&offset,&since,&until,&action,&pubkey,&kind); body {entries,total}
//	GET      /audit/kinds      — distinct kinds from recent audit rows (?scan_limit=); body {kinds:[]int}
//	GET      /events/{id}      — stored event JSON for admin UI (ephemeral / missing → 404)
//	GET      /nips             — known NIPs + enabled flags from config
//	PATCH    /nips             — toggle optional NIP; restart_required in response
//	GET      /stats            — relay connection count, ports, relay_version
//	GET      /relay-identity   — relay pubkey_hex and npub (read-only)
//	POST     /migration/start  — data migration (SSE); body may set make_target_primary to update config
//
// Non-API routes: CONGEE_ENV dev|development|local → reverse proxy to Vite :5173 (GET/HEAD only);
// otherwise static files from web/admin/build with SPA fallback to index.html.
type Server struct {
	cfg       *config.Config
	cfgPath   string
	store     storage.Store
	relay     *relay.Server
	relayID   *relayidentity.Identity
	log       zerolog.Logger
	password  string
	staticDir string
	devProxy  *httputil.ReverseProxy
	static    http.Handler // SPA file server (production, or dev fallback when Vite is down)

	cfgMu sync.Mutex
	http  *http.Server
}

// NewServer builds an admin server. staticDir is the filesystem root for production
// assets (e.g. web/admin/build). relaySrv may be nil (stats will show 0 connections).
// scheduleRestart is invoked after a successful config write or NIP toggle when the
// running process should be replaced (nil in tests or when restart is disabled).
func NewServer(cfg *config.Config, cfgPath string, store storage.Store, relaySrv *relay.Server, log zerolog.Logger, password, staticDir string, scheduleRestart func(), relayID *relayidentity.Identity) *Server {
	s := &Server{
		cfg:       cfg,
		cfgPath:   cfgPath,
		store:     store,
		relay:     relaySrv,
		relayID:   relayID,
		log:       log,
		password:  password,
		staticDir: staticDir,
	}
	spaFS := spaFileSystem{dir: http.Dir(s.staticDir)}
	s.static = s.onlyGET(serveAdminStatic(s.staticDir, http.FileServer(spaFS)))

	if isDevEnv() {
		target, _ := url.Parse("http://127.0.0.1:5173")
		p := httputil.NewSingleHostReverseProxy(target)
		// Avoid net/http's "http: proxy error" log line and support one-terminal dev:
		// if Vite is not running, serve a prior `make ui-build` output when present.
		p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			s.log.Warn().Err(err).Str("vite_url", target.String()).Msg("admin dev proxy unreachable; trying static build")
			if hasAdminStaticIndex(s.staticDir) {
				s.static.ServeHTTP(w, r)
				return
			}
			http.Error(w, "Vite dev server is not running ("+target.Host+"). In another terminal run: cd web/admin && npm run dev\nOr build once with: make ui-build — then either keep CONGEE_ENV=development (fallback) or use production mode to serve only static files.", http.StatusBadGateway)
		}
		s.devProxy = p
	}
	mux := http.NewServeMux()
	// API prefix: all handlers below are mounted at /api/ (StripPrefix removes "/api").
	//   GET|PUT  /api/config           — raw JSON file; PUT validates, atomic write, changelog row
	//   GET      /api/config/changelog — recent config change rows (?limit=)
	//   GET      /api/audit            — audit log (?limit=&offset=&since=&until=&action=&pubkey=&kind=); {entries,total}
	//   GET      /api/audit/kinds      — distinct kinds from recent audit rows (?scan_limit=); {kinds:[]}
	//   GET      /api/events/{id}      — one event from storage by id
	//   GET      /api/nips             — known NIPs + enabled flags
	//   PATCH    /api/nips             — toggle optional NIP; { "nip": N, "enabled": bool }; restart_required
	//   GET      /api/stats            — relay connection count, ports (placeholders OK)
	//   GET      /api/relay-identity   — relay pubkey_hex and npub
	//   POST     /api/migration/start  — sqlite/postgres copy; SSE progress events
	api := http.NewServeMux()
	api.HandleFunc("GET /config", handleGetConfig(cfgPath).ServeHTTP)
	api.HandleFunc("PUT /config", handlePutConfig(cfgPath, &s.cfgMu, store, scheduleRestart, relayID).ServeHTTP)
	api.HandleFunc("GET /config/changelog", handleConfigChangelog(store).ServeHTTP)
	api.HandleFunc("GET /audit/kinds", HandleAuditKinds(store).ServeHTTP)
	api.HandleFunc("GET /audit", HandleAudit(store).ServeHTTP)
	api.HandleFunc("GET /events/{id}", handleGetEvent(store).ServeHTTP)
	api.HandleFunc("GET /nips", handleNIPsGet(cfgPath).ServeHTTP)
	api.HandleFunc("PATCH /nips", handleNIPsPatch(cfgPath, &s.cfgMu, store, scheduleRestart).ServeHTTP)
	api.HandleFunc("GET /stats", handleStats(cfg, relaySrv).ServeHTTP)
	api.Handle("GET /relay-identity", handleRelayIdentity(relayID))
	api.HandleFunc("POST /migration/start", handleMigrationStart(s.log, s.cfgPath, &s.cfgMu, scheduleRestart, relayID))

	mux.Handle("/api/", RequireAdminAuth(password, http.StripPrefix("/api", api)))

	if isDevEnv() {
		mux.HandleFunc("/", s.serveDevProxy)
	} else {
		mux.Handle("/", s.static)
	}

	addr := ":" + strconv.Itoa(cfg.Admin.Port)
	s.http = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return s
}

// ListenAndServe binds the admin port and blocks until the server stops.
func (s *Server) ListenAndServe() error {
	s.log.Info().Str("addr", s.http.Addr).Msg("admin listening")
	return s.http.ListenAndServe()
}

// Shutdown stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) serveDevProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.devProxy.ServeHTTP(w, r)
}

func hasAdminStaticIndex(staticDir string) bool {
	st, err := os.Stat(filepath.Join(staticDir, "index.html"))
	return err == nil && !st.IsDir()
}

func (s *Server) onlyGET(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveAdminStatic handles GET / and HEAD / without http.FileServer: spaFileSystem maps Open("/")
// to index.html (a file), but net/http rejects that combination with:
//
//	http: attempting to traverse a non-directory
//
// Routing "/" through FileServer would also conflict with FileServer's redirect from
// "/index.html" to canonical "./". Serving the root file explicitly avoids both issues.
func serveAdminStatic(staticDir string, fileServer http.Handler) http.Handler {
	indexPath := filepath.Join(staticDir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			http.ServeFile(w, r, indexPath)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// spaFileSystem serves real files from dir and falls back to /index.html for client-side routes.
type spaFileSystem struct {
	dir http.FileSystem
}

func (s spaFileSystem) Open(name string) (http.File, error) {
	if name == "/" {
		return s.dir.Open("/index.html")
	}
	f, err := s.dir.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			return s.dir.Open("/index.html")
		}
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if st.IsDir() {
		_ = f.Close()
		return s.dir.Open("/index.html")
	}
	return f, nil
}
