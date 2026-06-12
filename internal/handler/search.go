package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
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

		view := r.URL.Query().Get("view")
		if view != "grid" && view != "compact" {
			view = "list"
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
			Downloads  int
		}
		type pageData struct {
			Query      string
			Results    []searchResultView
			Page       int
			Total      int
			PrevURL    string
			NextURL    string
			ShowCovers bool
			View       string
			Facets     map[string][]archive.FacetField
		}

		type facetAcc struct {
			order  []string
			counts map[string]int
		}
		newAcc := func() *facetAcc { return &facetAcc{counts: make(map[string]int)} }
		accs := map[string]*facetAcc{
			"mediatype":  newAcc(),
			"year":       newAcc(),
			"creator":    newAcc(),
			"subject":    newAcc(),
			"collection": newAcc(),
			"language":   newAcc(),
		}
		addVal := func(field, v string) {
			if v == "" {
				return
			}
			a := accs[field]
			if a.counts[v] == 0 {
				a.order = append(a.order, v)
			}
			a.counts[v]++
		}

		views := make([]searchResultView, len(results.Results))
		for i, r := range results.Results {
			var creator string
			if sl := toStringSlice(r.Creator); len(sl) > 0 {
				creator = sl[0]
				addVal("creator", creator)
			}
			addVal("mediatype", r.Mediatype)
			if len(r.Date) >= 4 {
				if _, err := strconv.Atoi(r.Date[:4]); err == nil {
					addVal("year", r.Date[:4])
				}
			}
			for _, s := range toStringSlice(r.Subject) {
				addVal("subject", s)
			}
			for _, c := range toStringSlice(r.Collections) {
				addVal("collection", c)
			}
			for _, l := range toStringSlice(r.Language) {
				addVal("language", l)
			}
			views[i] = searchResultView{
				Identifier: r.Identifier,
				Title:      r.Title,
				Mediatype:  r.Mediatype,
				Creator:    creator,
				Date:       r.Date,
				Downloads:  r.Downloads,
			}
		}

		facets := make(map[string][]archive.FacetField)
		for field, acc := range accs {
			if len(acc.order) == 0 {
				continue
			}
			fields := make([]archive.FacetField, len(acc.order))
			for i, v := range acc.order {
				fields[i] = archive.FacetField{Value: v, Count: acc.counts[v]}
			}
			sort.Slice(fields, func(i, j int) bool { return fields[i].Count > fields[j].Count })
			if len(fields) > 6 {
				fields = fields[:6]
			}
			facets[field] = fields
		}

		viewParam := ""
		if view != "list" {
			viewParam = "&view=" + url.QueryEscape(view)
		}

		data := pageData{
			Query:      q,
			Results:    views,
			Page:       page,
			Total:      results.Total,
			ShowCovers: cfg.ShowCovers,
			View:       view,
			Facets:     facets,
		}
		if page > 1 {
			data.PrevURL = fmt.Sprintf("/search?q=%s&page=%d%s", url.QueryEscape(q), page-1, viewParam)
		}
		if page*searchRows < results.Total {
			data.NextURL = fmt.Sprintf("/search?q=%s&page=%d%s", url.QueryEscape(q), page+1, viewParam)
		}

		render(w, tmpl, "search.html", data)
	}
}
