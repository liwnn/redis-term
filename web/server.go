package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/liwnn/redisterm/config"
	"github.com/liwnn/redisterm/datasource"
	"github.com/liwnn/redisterm/datasource/mongo"
	"github.com/liwnn/redisterm/datasource/mysql"
	"github.com/liwnn/redisterm/datasource/redisapi"
)

//go:embed index.html
var staticFiles embed.FS

// devIndexPath is the on-disk index.html next to this source file. When it
// exists (i.e. running from the repo via `go run`), the server reads it live
// so a browser refresh picks up edits without a rebuild. In a deployed binary
// the path is absent and the server falls back to the embedded copy.
var devIndexPath = func() string {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(self), "index.html")
}()

// indexHTML returns the page source, preferring the live on-disk file in dev.
// The bool reports whether it came from disk (so caching can be disabled).
func indexHTML() ([]byte, bool) {
	if devIndexPath != "" {
		if b, err := os.ReadFile(devIndexPath); err == nil {
			return b, true
		}
	}
	b, _ := staticFiles.ReadFile("index.html")
	return b, false
}

// Server serves a single-page web UI backed by a Datasource.
type Server struct {
	cfg *config.Config

	mu      sync.Mutex
	sources map[string]datasource.Datasource
}

// NewServer new
func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg:     cfg,
		sources: make(map[string]datasource.Datasource),
	}
}

// source returns a cached datasource for the config at index, built by kind.
func (s *Server) source(index int) (datasource.Datasource, error) {
	conf := s.cfg.GetConfig(index)
	cacheKey := strconv.Itoa(index)

	s.mu.Lock()
	defer s.mu.Unlock()
	if ds, ok := s.sources[cacheKey]; ok {
		return ds, nil
	}
	var ds datasource.Datasource
	switch conf.Kind {
	case "mongo":
		ds = mongo.NewMongoSource(conf.MongoURI())
	case "mysql":
		ds = mysql.NewMySQLSource(conf.Host, conf.Port, conf.User, conf.Auth)
	default:
		address := fmt.Sprintf("%v:%v", conf.Host, conf.Port)
		ds = datasource.NewRedisSource(address, conf.Auth)
	}
	if err := ds.Open(); err != nil {
		return nil, err
	}
	s.sources[cacheKey] = ds
	return ds, nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func intParam(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// connInfo describes a configured connection for the UI. It carries the full
// field set so the edit form can be pre-filled (auth included — this UI is a
// local admin tool).
type connInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "redis", "mongo", or "mysql"
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Auth string `json:"auth"`
	URI  string `json:"uri"`
	DB   string `json:"db"`
}

// handleConns lists configured connections with their backend kind, plus the
// index of the connection selected on the last run so the UI can restore it.
func (s *Server) handleConns(w http.ResponseWriter, r *http.Request) {
	conns := make([]connInfo, 0, s.cfg.Count())
	for i := 0; i < s.cfg.Count(); i++ {
		conf := s.cfg.GetConfig(i)
		kind := conf.Kind
		if kind == "" {
			kind = "redis"
		}
		conns = append(conns, connInfo{
			Name: conf.Name, Kind: kind, Host: conf.Host, Port: conf.Port,
			User: conf.User, Auth: conf.Auth, URI: conf.URI, DB: conf.DB,
		})
	}
	writeJSON(w, map[string]interface{}{
		"conns":    conns,
		"selected": s.cfg.LastSelectedIndex(),
	})
}

// connBody is the JSON payload for creating/updating a connection. Index < 0
// means add a new connection; otherwise update the one at Index.
type connBody struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	User  string `json:"user"`
	Auth  string `json:"auth"`
	URI   string `json:"uri"`
	DB    string `json:"db"`
}

// toConfig converts a request body to a stored config. Redis keeps an empty Kind
// for backward compatibility (older configs predate the field).
func (b connBody) toConfig() redisapi.RedisConfig {
	kind := b.Kind
	if kind == "redis" {
		kind = ""
	}
	return redisapi.RedisConfig{
		Name: b.Name, Host: b.Host, Port: b.Port, User: b.User,
		Auth: b.Auth, Kind: kind, URI: b.URI, DB: b.DB,
	}
}

// decodeConnBody reads and validates the JSON body (Name required).
func decodeConnBody(r *http.Request) (connBody, error) {
	var b connBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		return b, err
	}
	if strings.TrimSpace(b.Name) == "" {
		return b, fmt.Errorf("name is required")
	}
	return b, nil
}

// dropSources closes and clears every cached datasource. Called after a config
// add/update/delete because the cache is keyed by index, which shifts on delete;
// surviving connections lazily reconnect on next use.
func (s *Server) dropSources() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ds := range s.sources {
		ds.Close()
	}
	s.sources = make(map[string]datasource.Datasource)
}

// handleConnSave adds a new connection (index < 0) or updates an existing one.
func (s *Server) handleConnSave(w http.ResponseWriter, r *http.Request) {
	b, err := decodeConnBody(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	conf := b.toConfig()
	if b.Index < 0 || b.Index >= s.cfg.Count() {
		s.cfg.Add(conf)
	} else {
		s.cfg.Update(conf, b.Index)
	}
	s.dropSources()
	if err := s.cfg.Save(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleConnDelete removes the connection at the given index.
func (s *Server) handleConnDelete(w http.ResponseWriter, r *http.Request) {
	index := intParam(r, "conn", -1)
	if index < 0 || index >= s.cfg.Count() {
		writeErr(w, fmt.Errorf("invalid connection index"))
		return
	}
	s.cfg.Remove(index)
	s.dropSources()
	if err := s.cfg.Save(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleConnTest opens a throwaway connection to verify the posted config is
// reachable, then closes it. Mirrors the TUI's Test button.
func (s *Server) handleConnTest(w http.ResponseWriter, r *http.Request) {
	b, err := decodeConnBody(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	conf := b.toConfig()
	var ds datasource.Datasource
	switch conf.Kind {
	case "mongo":
		ds = mongo.NewMongoSource(conf.MongoURI())
	case "mysql":
		ds = mysql.NewMySQLSource(conf.Host, conf.Port, conf.User, conf.Auth)
	default:
		ds = datasource.NewRedisSource(fmt.Sprintf("%v:%v", conf.Host, conf.Port), conf.Auth)
	}
	if err := ds.Open(); err != nil {
		writeErr(w, err)
		return
	}
	ds.Close()
	writeJSON(w, map[string]bool{"ok": true})
}

// handleSelect persists the last-selected connection so the next page load
// restores it.
func (s *Server) handleSelect(w http.ResponseWriter, r *http.Request) {
	s.cfg.SaveLastSelected(intParam(r, "conn", 0))
	writeJSON(w, map[string]bool{"ok": true})
}

// handleContainers lists the second tree level (dbs / databases).
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	names, err := ds.Containers()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, names)
}

// handleEntries lists entries in a container, optionally filtered by a glob match.
func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	container := r.URL.Query().Get("container")
	entries, err := ds.Entries(container)
	if err != nil {
		writeErr(w, err)
		return
	}
	if match := r.URL.Query().Get("match"); match != "" && match != "*" {
		entries = filterMatch(entries, match)
	}
	writeJSON(w, entries)
}

// contentResp is the JSON shape of an entry's content.
type contentResp struct {
	Entry     string     `json:"entry"`
	Kind      string     `json:"kind"` // "text" | "table"
	Type      string     `json:"type"`
	Columns   []string   `json:"columns"`
	Rows      [][]string `json:"rows"`
	CellTypes [][]string `json:"cellTypes"`
	Text      string     `json:"text"`
	Total     int        `json:"total"`
	ReadOnly  []string   `json:"readOnly"`
}

// handleContent returns the right-pane payload for an entry.
func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	container := r.URL.Query().Get("container")
	entry := r.URL.Query().Get("entry")
	if entry == "" {
		writeErr(w, fmt.Errorf("missing entry"))
		return
	}
	c, err := ds.Content(container, entry, datasource.Page{
		Skip:  intParam(r, "skip", 0),
		Limit: intParam(r, "limit", 0),
		Query: r.URL.Query().Get("query"),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	kind := "table"
	if c.Kind == datasource.KindText {
		kind = "text"
	}
	writeJSON(w, contentResp{
		Entry: entry, Kind: kind, Type: c.Type,
		Columns: c.Columns, Rows: c.Rows, CellTypes: c.CellTypes,
		Text: c.Text, Total: c.Total, ReadOnly: c.ReadOnly,
	})
}

// handleUpdate writes a single table cell change back to the backend.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Container   string   `json:"container"`
		Entry       string   `json:"entry"`
		Columns     []string `json:"columns"`
		Row         int      `json:"row"`
		Column      int      `json:"column"`
		OldRow      []string `json:"oldRow"`
		OldRowTypes []string `json:"oldRowTypes"`
		Value       string   `json:"value"`
		OldType     string   `json:"oldType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Entry == "" {
		writeErr(w, fmt.Errorf("missing entry"))
		return
	}
	err = ds.Update(body.Container, body.Entry, datasource.Edit{
		Columns:     body.Columns,
		Row:         body.Row,
		Column:      body.Column,
		OldRow:      body.OldRow,
		OldRowTypes: body.OldRowTypes,
		Value:       body.Value,
		OldType:     body.OldType,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleDrop removes an entry (redis key / mongo collection) from a container.
func (s *Server) handleDrop(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Container string `json:"container"`
		Entry     string `json:"entry"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Entry == "" {
		writeErr(w, fmt.Errorf("missing entry"))
		return
	}
	if err := ds.DropEntry(body.Container, body.Entry); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleDropDB drops a container (mongo database / redis db flush).
func (s *Server) handleDropDB(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Container string `json:"container"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Container == "" {
		writeErr(w, fmt.Errorf("missing container"))
		return
	}
	if err := ds.DropContainer(body.Container); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleCmd runs a backend-native command.
func (s *Server) handleCmd(w http.ResponseWriter, r *http.Request) {
	ds, err := s.source(intParam(r, "conn", 0))
	if err != nil {
		writeErr(w, err)
		return
	}
	container := r.URL.Query().Get("container")
	var body struct {
		Cmd string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	reply, err := ds.Command(container, body.Cmd)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"reply": reply})
}

// filterMatch keeps items matching a pattern. A pattern containing '*' is a
// glob (case-sensitive, anchored); a plain pattern is a case-insensitive
// substring search so typing "user" finds "user:1" without needing wildcards.
func filterMatch(items []string, pattern string) []string {
	out := items[:0:0]
	if strings.Contains(pattern, "*") {
		for _, it := range items {
			if globMatch(pattern, it) {
				out = append(out, it)
			}
		}
		return out
	}
	needle := strings.ToLower(pattern)
	for _, it := range items {
		if strings.Contains(strings.ToLower(it), needle) {
			out = append(out, it)
		}
	}
	return out
}

// globMatch reports whether s matches pattern where '*' matches any run.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// Handler returns the http handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/conns", s.handleConns)
	mux.HandleFunc("/api/conn/save", s.handleConnSave)
	mux.HandleFunc("/api/conn/delete", s.handleConnDelete)
	mux.HandleFunc("/api/conn/test", s.handleConnTest)
	mux.HandleFunc("/api/select", s.handleSelect)
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/entries", s.handleEntries)
	mux.HandleFunc("/api/content", s.handleContent)
	mux.HandleFunc("/api/update", s.handleUpdate)
	mux.HandleFunc("/api/drop", s.handleDrop)
	mux.HandleFunc("/api/dropdb", s.handleDropDB)
	mux.HandleFunc("/api/cmd", s.handleCmd)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, fromDisk := indexHTML()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if fromDisk {
			w.Header().Set("Cache-Control", "no-store")
		}
		_, _ = w.Write(b)
	})
	return mux
}

// Run starts the http server on addr.
func (s *Server) Run(addr string) error {
	fmt.Printf("redis-term web listening on http://%s\n", addr)
	return http.ListenAndServe(addr, s.Handler())
}
