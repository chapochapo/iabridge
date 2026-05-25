package handler

import (
	"html/template"
	"net/http"
)

func landing(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, tmpl, "landing.html", nil)
	}
}
