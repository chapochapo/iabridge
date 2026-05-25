package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"iabridge/internal/archive"
	"iabridge/internal/config"
)

const searchRows = 20

func search(tmpl *template.Template, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		page := 1
		if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
			page = p
		}

		results, err := archive.Search(q, page, searchRows)
		if err != nil {
			http.Error(w, "search failed: "+err.Error(), http.StatusBadGateway)
			return
		}

		type searchResultView struct {
			Identifier string
			Title      string
			Mediatype  string
			Creator    string
			Date       string
		}
		type pageData struct {
			Query      string
			Results    []searchResultView
			Page       int
			Total      int
			PrevURL    string
			NextURL    string
			ShowCovers bool
		}

		views := make([]searchResultView, len(results.Results))
		for i, r := range results.Results {
			var creator string
			if sl := toStringSlice(r.Creator); len(sl) > 0 {
				creator = sl[0]
			}
			views[i] = searchResultView{
				Identifier: r.Identifier,
				Title:      r.Title,
				Mediatype:  r.Mediatype,
				Creator:    creator,
				Date:       r.Date,
			}
		}

		data := pageData{
			Query:      q,
			Results:    views,
			Page:       page,
			Total:      results.Total,
			ShowCovers: cfg.ShowCovers,
		}
		if page > 1 {
			data.PrevURL = fmt.Sprintf("/search?q=%s&page=%d", url.QueryEscape(q), page-1)
		}
		if page*searchRows < results.Total {
			data.NextURL = fmt.Sprintf("/search?q=%s&page=%d", url.QueryEscape(q), page+1)
		}

		render(w, tmpl, "search.html", data)
	}
}
