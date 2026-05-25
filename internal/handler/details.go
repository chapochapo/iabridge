package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"iabridge/internal/archive"
	"iabridge/internal/config"
)

type detailsData struct {
	Item        *archive.Item
	TorrentURL  string
	CoverURL    string
	SavePaths   []string
	Subjects    []string
	Collections []string
	Description template.HTML
	TotalSize   string
}

func details(tmpl *template.Template, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("identifier")
		if !archive.ValidIdentifier(id) {
			http.Error(w, "invalid identifier", http.StatusBadRequest)
			return
		}

		item, err := archive.GetItem(id)
		if err != nil {
			http.Error(w, "could not fetch item: "+err.Error(), http.StatusBadGateway)
			return
		}

		data := detailsData{
			Item:        item,
			TorrentURL:  archive.TorrentURL(item.Metadata.Identifier, item.Files),
			SavePaths:   cfg.QbittorrentPaths,
			Subjects:    toSubjectSlice(item.Metadata.Subject),
			Collections: toStringSlice(item.Metadata.Collections),
			Description: sanitizeHTML(strings.Join(toStringSlice(item.Metadata.Description), "\n")),
			TotalSize:   formatSize(totalBytes(item.Files)),
		}
		if cfg.ShowCovers && item.Metadata.Mediatype == "audio" {
			data.CoverURL = "/api/cover/" + id
		}

		render(w, tmpl, "details.html", data)
	}
}

// toSubjectSlice normalises archive.org subject fields. The API returns
// subjects as either a []string or a single semicolon-delimited string
// (e.g. "Mark Tribe; Patrick Harrison; protest"). Both forms are split into
// individual trimmed values.
func toSubjectSlice(v any) []string {
	raw := toStringSlice(v)
	var out []string
	for _, s := range raw {
		for _, part := range strings.Split(s, ";") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// toStringSlice normalises archive.org metadata fields that may be a single
// string or a JSON array of strings into a consistent []string.
func toStringSlice(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func totalBytes(files []archive.ItemFile) int64 {
	var total int64
	for _, f := range files {
		if n, err := strconv.ParseInt(f.Size, 10, 64); err == nil {
			total += n
		}
	}
	return total
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
