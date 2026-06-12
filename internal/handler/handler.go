// Package handler registers all HTTP routes for iabridge.
//
// Routes:
//
//	GET  /{$}                    Landing page — search bar only
//	GET  /search                 Search results page
//	GET  /details/{identifier}   Item detail page
//	GET  /api/cover/{identifier} Cover image proxy (audio only)
//	POST /api/qbittorrent/add    Add torrent to qBittorrent
//	POST /api/ia/download        Run ia download subprocess
//	GET  /api/downloads          Download history JSON (LAN only)
//	GET  /downloads              Downloads page (LAN only)
//	GET  /static/                Embedded static assets (SVG icons)
package handler

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"iabridge/internal/config"
	"iabridge/internal/downloads"
	"iabridge/internal/middleware"
)

//go:embed tmpl
var tmplFS embed.FS

//go:embed static
var staticFS embed.FS

// Register wires all routes onto mux.
func Register(mux *http.ServeMux, cfg *config.Config, store *downloads.Store) {
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"creatorSearchURL": func(creator string) string {
			creator = strings.ReplaceAll(creator, `"`, `\"`)
			return "/search?q=" + url.QueryEscape(`creator:"`+creator+`"`)
		},
		"subjectSearchURL": func(subject string) string {
			subject = strings.ReplaceAll(subject, `"`, `\"`)
			return "/search?q=" + url.QueryEscape(`subject:"`+subject+`"`)
		},
		"facetURL": func(q, field, value, view string) string {
			value = strings.ReplaceAll(value, `"`, `\"`)
			newQ := q + ` ` + field + `:"` + value + `"`
			u := "/search?q=" + url.QueryEscape(newQ)
			if view != "" && view != "list" {
				u += "&view=" + url.QueryEscape(view)
			}
			return u
		},
	}).ParseFS(tmplFS, "tmpl/*.html"))

	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /{$}", landing(tmpl))
	mux.HandleFunc("GET /search", search(tmpl, cfg))
	mux.HandleFunc("GET /details/{identifier}", details(tmpl, cfg))
	mux.HandleFunc("GET /api/cover/{identifier}", cover())
	mux.HandleFunc("POST /api/qbittorrent/add", qbtAdd(cfg))
	mux.HandleFunc("POST /api/ia/download", iaDownload(cfg, store))

	lanOnly := func(h http.Handler) http.Handler { return middleware.LANOnly(cfg.AllowedNet, h) }
	mux.Handle("GET /api/downloads", lanOnly(downloadsAPI(store)))
	mux.Handle("POST /api/downloads/clear", lanOnly(clearDownloads(store)))
	mux.Handle("POST /api/downloads/delete", lanOnly(deleteDownloads(store, cfg)))
	mux.Handle("GET /downloads", lanOnly(downloadsPage(tmpl, store)))
}

// render executes a named template into a buffer before writing the response,
// so a template error doesn't result in a partial HTML page being sent.
func render(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
