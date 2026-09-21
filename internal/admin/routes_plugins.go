package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/plugin"
)

func registerPluginRoutes(api *http.ServeMux, s *Server) {
	api.HandleFunc("GET /plugins", s.handlePluginsList)
	api.HandleFunc("POST /plugins/install", s.handlePluginsInstall)
	api.HandleFunc("GET /plugins/{id}", s.handlePluginGet)
	api.HandleFunc("POST /plugins/{id}/enable", s.handlePluginEnable)
	api.HandleFunc("POST /plugins/{id}/disable", s.handlePluginDisable)
	api.HandleFunc("POST /plugins/{id}/uninstall", s.handlePluginUninstall)
	api.HandleFunc("GET /plugins/{id}/settings", s.handlePluginGetSettings)
	api.HandleFunc("PUT /plugins/{id}/settings", s.handlePluginPutSettings)
	api.HandleFunc("GET /plugins/{id}/status", s.handlePluginStatus)
	api.HandleFunc("GET /plugins/{id}/intercept-log", s.handlePluginGetInterceptLog)
	api.HandleFunc("PUT /plugins/{id}/intercept-log", s.handlePluginPutInterceptLog)
	api.HandleFunc("POST /plugins/{id}/actions/{action}", s.handlePluginAction)
	api.HandleFunc("GET /plugins/{id}/ui/{path...}", s.handlePluginUI)
}

func (s *Server) handlePluginsList(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, map[string]any{"plugins": []any{}, "metrics": plugin.Metrics{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plugins": s.plugins.Snapshot(),
		"metrics": s.plugins.MetricsSnapshot(),
	})
}

func (s *Server) handlePluginsInstall(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, `{"error":"plugins disabled"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		URL    string `json:"url"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Enable bool   `json:"enable"`
		Wipe   bool   `json:"wipe"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	var man any
	var err error
	if strings.TrimSpace(body.Path) != "" {
		man, err = s.plugins.InstallLocal(body.Path, body.Enable)
	} else if strings.TrimSpace(body.URL) != "" {
		man, err = s.plugins.InstallURL(r.Context(), body.URL, body.SHA256, body.Enable)
	} else {
		http.Error(w, `{"error":"url or path required"}`, http.StatusBadRequest)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = s.persistPluginConfigLocked(r, "install plugin")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "manifest": man})
}

func (s *Server) handlePluginGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.plugins == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	for _, p := range s.plugins.Snapshot() {
		if p.ID == id {
			st, ready, _ := s.plugins.StatusJSON(r.Context(), id)
			writeJSON(w, http.StatusOK, map[string]any{"plugin": p, "status": json.RawMessage(st), "ready": ready})
			return
		}
	}
	http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
}

func (s *Server) handlePluginEnable(w http.ResponseWriter, r *http.Request) {
	s.pluginToggle(w, r, true)
}

func (s *Server) handlePluginDisable(w http.ResponseWriter, r *http.Request) {
	s.pluginToggle(w, r, false)
}

func (s *Server) pluginToggle(w http.ResponseWriter, r *http.Request, enable bool) {
	if s.plugins == nil {
		http.Error(w, `{"error":"plugins disabled"}`, http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	var err error
	if enable {
		err = s.plugins.Enable(id)
	} else {
		err = s.plugins.Disable(id)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = s.persistPluginConfigLocked(r, "toggle plugin")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePluginUninstall(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, `{"error":"plugins disabled"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		WipeData bool `json:"wipe_data"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	id := r.PathValue("id")
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if err := s.plugins.Uninstall(id, body.WipeData); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = s.persistPluginConfigLocked(r, "uninstall plugin")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePluginGetSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.cfg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"settings": map[string]any{}})
		return
	}
	it, idx := config.PluginItemByID(s.cfg, id)
	if idx < 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	raw := it.Settings
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	raw = config.RedactSecretsForLog(mustWrapSettings(raw))
	writeJSON(w, http.StatusOK, map[string]any{"settings": json.RawMessage(rawSettingsOnly(raw))})
}

func (s *Server) handlePluginPutSettings(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, `{"error":"plugins disabled"}`, http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
		return
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if err := s.plugins.ApplySettings(r.Context(), id, body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = s.persistPluginConfigLocked(r, "plugin settings")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePluginStatus(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ready": false})
		return
	}
	raw, ready, err := s.plugins.StatusJSON(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":               ready,
		"status":              json.RawMessage(raw),
		"relay_database_type": s.cfg.Database.Type,
	})
}

func (s *Server) pluginKnown(id string) bool {
	if s.plugins == nil || id == "" {
		return false
	}
	for _, p := range s.plugins.Snapshot() {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) handlePluginGetInterceptLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, plugin.InterceptLogSnapshot{
			Limit:   config.DefaultPluginInterceptLogSize,
			Entries: []plugin.InterceptLogEntry{},
		})
		return
	}
	if !s.pluginKnown(id) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.plugins.InterceptLogSnapshot(id))
}

func (s *Server) handlePluginPutInterceptLog(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, `{"error":"plugins disabled"}`, http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if !s.pluginKnown(id) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	var body struct {
		Limit *int `json:"limit"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.Limit == nil {
		http.Error(w, `{"error":"limit required"}`, http.StatusBadRequest)
		return
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	limit := s.plugins.SetInterceptLogLimit(*body.Limit)
	_ = s.persistPluginConfigLocked(r, "plugin intercept log size")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "limit": limit})
}

func (s *Server) handlePluginAction(w http.ResponseWriter, r *http.Request) {
	if s.plugins == nil {
		http.Error(w, `{"error":"plugins disabled"}`, http.StatusServiceUnavailable)
		return
	}
	payload, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	out, err := s.plugins.AdminAction(r.Context(), r.PathValue("id"), r.PathValue("action"), payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(out) == 0 {
		out = []byte(`{"ok":true}`)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *Server) handlePluginUI(w http.ResponseWriter, r *http.Request) {
	s.servePluginUI(w, r, r.PathValue("id"), r.PathValue("path"))
}

func (s *Server) handlePluginUIPublic(w http.ResponseWriter, r *http.Request) {
	s.servePluginUI(w, r, r.PathValue("id"), r.PathValue("path"))
}

// writePluginUIAccessHeaders lets a sandboxed iframe (opaque origin "null") load
// Vite ES modules from this host. Without CORS, Chrome blocks
// plugin-ui/{id}/assets/*.js with origin 'null'.
func writePluginUIAccessHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	switch origin {
	case "", "*":
		w.Header().Set("Access-Control-Allow-Origin", "*")
	default:
		// Includes the string "null" from sandboxed iframes without allow-same-origin.
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	if reqHdr := r.Header.Get("Access-Control-Request-Headers"); reqHdr != "" {
		w.Header().Set("Access-Control-Allow-Headers", reqHdr)
	}
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
}

func (s *Server) servePluginUI(w http.ResponseWriter, r *http.Request, id, rel string) {
	writePluginUIAccessHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.plugins == nil {
		http.NotFound(w, r)
		return
	}
	ui := s.plugins.UIDir(id)
	if rel == "" || rel == "." || strings.HasSuffix(r.URL.Path, "/") {
		rel = "index.html"
	}
	rel = filepath.Clean(rel)
	if strings.HasPrefix(rel, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	fp := filepath.Join(ui, rel)
	if !strings.HasPrefix(fp, ui) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if st, err := os.Stat(fp); err != nil || st.IsDir() {
		fp = filepath.Join(ui, "index.html")
	}
	// Opaque iframe origin is not 'self'; allow http(s) so module scripts and CSS load.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data: http: https:; style-src 'unsafe-inline' http: https:; script-src 'unsafe-inline' http: https:; font-src http: https:; connect-src 'none'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, fp)
}

func (s *Server) persistPluginConfigLocked(r *http.Request, summary string) error {
	if s.plugins == nil {
		return nil
	}
	if err := s.plugins.PersistConfig(); err != nil {
		return err
	}
	if s.store == nil {
		return nil
	}
	data, _ := json.Marshal(s.cfg)
	return config.SaveConfigChange(r.Context(), s.store, summary, string(config.RedactSecretsForLog(data)))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func mustWrapSettings(raw []byte) []byte {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	b, _ := json.Marshal(map[string]any{"plugins": map[string]any{"items": []any{map[string]any{"settings": m}}}})
	return b
}

func rawSettingsOnly(redactedWrap []byte) []byte {
	var m map[string]any
	if json.Unmarshal(redactedWrap, &m) != nil {
		return []byte(`{}`)
	}
	plugins, _ := m["plugins"].(map[string]any)
	items, _ := plugins["items"].([]any)
	if len(items) == 0 {
		return []byte(`{}`)
	}
	item, _ := items[0].(map[string]any)
	st, _ := item["settings"]
	b, err := json.Marshal(st)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}
