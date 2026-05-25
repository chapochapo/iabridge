package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"iabridge/internal/config"
	"iabridge/internal/downloads"
	"iabridge/internal/qbittorrent"
)

func qbtAdd(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TorrentURL string `json:"torrent_url"`
			SavePath   string `json:"save_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if !strings.HasPrefix(req.TorrentURL, "https://archive.org/download/") {
			jsonError(w, "invalid torrent URL", http.StatusBadRequest)
			return
		}
		if !downloads.PathAllowed(req.SavePath, cfg.QbittorrentPaths) {
			jsonError(w, "save path not in allowed list", http.StatusBadRequest)
			return
		}

		if err := qbittorrent.Add(cfg.QbittorrentURL, cfg.QbittorrentUser, cfg.QbittorrentPass, req.TorrentURL, req.SavePath); err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
