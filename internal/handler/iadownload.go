package handler

import (
	"encoding/json"
	"net/http"

	"iabridge/internal/archive"
	"iabridge/internal/config"
	"iabridge/internal/downloads"
)

func iaDownload(cfg *config.Config, store *downloads.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Identifier string `json:"identifier"`
			SavePath   string `json:"save_path"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if !archive.ValidIdentifier(req.Identifier) {
			jsonError(w, "invalid identifier", http.StatusBadRequest)
			return
		}

		// Fetch title and mediatype from archive.org — not from the request body,
		// so user-supplied strings never reach the store.
		item, err := archive.GetItem(req.Identifier)
		if err != nil {
			jsonError(w, "item not found: "+err.Error(), http.StatusNotFound)
			return
		}

		entry, err := store.Start(req.Identifier, item.Metadata.Title, item.Metadata.Mediatype, req.SavePath, cfg.QbittorrentPaths)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}
}
