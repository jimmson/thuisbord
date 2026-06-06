// Package web wires the cached data into server-rendered HTML for the kiosk
// tablet. Handlers only ever read the in-memory caches, so a slow or failed
// upstream never blocks or blanks the page.
package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"time"

	"thuisbord/internal/cache"
	"thuisbord/internal/config"
	"thuisbord/internal/opzet"
	"thuisbord/internal/ovapi"
	"thuisbord/internal/weather"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server renders the dashboard from the widget caches.
type Server struct {
	cfg     config.Config
	bus     *cache.Cache[ovapi.Board]
	trash   *cache.Cache[[]opzet.Pickup]
	weather *cache.Cache[weather.Report]
	tmpl    *template.Template
}

// NewServer loads icons, parses templates, and returns a ready Server.
func NewServer(cfg config.Config, bus *cache.Cache[ovapi.Board], trash *cache.Cache[[]opzet.Pickup], wx *cache.Cache[weather.Report]) (*Server, error) {
	if err := loadIcons(); err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"icon":    iconHTML,
		"binIcon": binIcon,
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, bus: bus, trash: trash, weather: wx, tmpl: tmpl}, nil
}

// Handler returns the HTTP mux for the dashboard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	return mux
}

type pageData struct {
	Title          string
	RefreshSeconds int
	Clock          string
	Date           string
	Weather        WeatherView
	Bus            BusView
	Trash          TrashView
}

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().In(s.cfg.Location)

	board, busAt, busOK := s.bus.Snapshot()
	pickups, trashAt, trashOK := s.trash.Snapshot()
	report, wxAt, wxOK := s.weather.Snapshot()

	data := pageData{
		Title:          "Thuisbord",
		RefreshSeconds: s.cfg.RefreshSeconds,
		Clock:          now.Format("15:04"),
		Date:           dutchLongDate(now),
		Weather:        buildWeatherView(report, wxAt, wxOK),
		Bus:            buildBusView(board, s.cfg.Location, s.cfg.WalkOffset, s.cfg.MaxDepartures, now, busAt, busOK),
		Trash:          buildTrashView(pickups, s.cfg.Location, now, trashAt, trashOK, s.cfg.TrashWeeks),
	}

	// Render to a buffer first so a template error never emits a half-written 200.
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
