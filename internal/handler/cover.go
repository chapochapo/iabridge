package handler

import (
	"net/http"

	"iabridge/internal/archive"
)

func cover() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("identifier")
		if !archive.ValidIdentifier(id) {
			http.Error(w, "invalid identifier", http.StatusBadRequest)
			return
		}
		if err := archive.ProxyCover(w, id); err != nil {
			http.Error(w, "cover unavailable", http.StatusBadGateway)
		}
	}
}
