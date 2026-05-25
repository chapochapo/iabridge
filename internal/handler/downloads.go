package handler

import (
	"encoding/json"
	"html/template"
	"net/http"

	"iabridge/internal/downloads"
)

func downloadsAPI(store *downloads.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(store.List())
	}
}

func downloadsPage(tmpl *template.Template, store *downloads.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, tmpl, "downloads.html", store.List())
	}
}

func clearDownloads(store *downloads.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Identifiers []string `json:"identifiers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := store.Remove(body.Identifiers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteDownloads(store *downloads.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Identifiers []string `json:"identifiers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := store.Delete(body.Identifiers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
