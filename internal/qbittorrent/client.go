// Package qbittorrent provides a minimal client for the qBittorrent Web UI API.
package qbittorrent

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Add authenticates to qBittorrent and adds a torrent by URL to savePath.
func Add(baseURL, username, password, torrentURL, savePath string) error {
	sid, err := login(baseURL, username, password)
	if err != nil {
		return err
	}
	return addTorrent(baseURL, sid, torrentURL, savePath)
}

func login(baseURL, username, password string) (string, error) {
	form := url.Values{"username": {username}, "password": {password}}
	req, err := http.NewRequest("POST", baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("qbittorrent login: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", baseURL)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("qbittorrent login: %w", err)
	}
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if c.Name == "SID" {
			return c.Value, nil
		}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return "", fmt.Errorf("qbittorrent login: no SID cookie (response: %s)", strings.TrimSpace(string(body)))
}

func addTorrent(baseURL, sid, torrentURL, savePath string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("urls", torrentURL)
	mw.WriteField("savepath", savePath)
	mw.Close()

	req, err := http.NewRequest("POST", baseURL+"/api/v2/torrents/add", &body)
	if err != nil {
		return fmt.Errorf("qbittorrent add: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Referer", baseURL)
	req.AddCookie(&http.Cookie{Name: "SID", Value: sid})

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent add: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent add: unexpected status %d", resp.StatusCode)
	}
	result, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if s := strings.TrimSpace(string(result)); s != "Ok." {
		return fmt.Errorf("qbittorrent add: %s", s)
	}
	return nil
}
