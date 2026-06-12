// Package archive provides a client for the archive.org public APIs.
//
// Search API:  https://archive.org/advancedsearch.php?output=json
// Metadata API: https://archive.org/metadata/{identifier}
// Cover image:  https://archive.org/services/img/{identifier}
package archive

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

const (
	searchBase      = "https://archive.org/advancedsearch.php"
	metaBase        = "https://archive.org/metadata"
	coverBase       = "https://archive.org/services/img"
	maxResponseBody = 10 << 20 // 10 MB
)

var (
	httpClient   = &http.Client{Timeout: 15 * time.Second}
	identifierRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

// SearchResult represents a single item returned by the search API.
type SearchResult struct {
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Mediatype   string `json:"mediatype"`
	Creator     any    `json:"creator"`     // string or []string
	Date        string `json:"date"`
	Description any    `json:"description"` // string or []string
	Downloads   int    `json:"downloads"`
	Subject     any    `json:"subject"`     // string or []string
	Collections any    `json:"collection"`  // string or []string
	Language    any    `json:"language"`    // string or []string
}

// FacetField is a single value+count pair used to build sidebar filters.
type FacetField struct {
	Value string
	Count int
}

// SearchResponse wraps the archive.org advancedsearch response.
type SearchResponse struct {
	Results []SearchResult
	Total   int
}

// Item represents the full metadata for a single archive.org item.
type Item struct {
	Metadata  ItemMetadata
	Files     []ItemFile
}

// ItemMetadata holds the fields from the metadata API response.
type ItemMetadata struct {
	Identifier  string   `json:"identifier"`
	Title       string   `json:"title"`
	Mediatype   string   `json:"mediatype"`
	Creator     string   `json:"creator"`
	Date        string   `json:"date"`
	Addeddate   string   `json:"addeddate"`
	Uploader    string   `json:"uploader"`
	Description any      `json:"description"` // string or []string
	Subject     any      `json:"subject"` // string or []string
	Scanner     string   `json:"scanner"`
	Year        string   `json:"year"`
	Collections any      `json:"collection"` // string or []string
	// Views and favorites come from a separate stats endpoint; left for future phase
}

// ItemFile represents a single file within an archive.org item.
type ItemFile struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Size    string `json:"size"`
}

// TorrentURL returns the .torrent URL for an item if one exists in its file list,
// or an empty string if none is found.
func TorrentURL(identifier string, files []ItemFile) string {
	for _, f := range files {
		if f.Format == "Archive BitTorrent" {
			return fmt.Sprintf("https://archive.org/download/%s/%s", identifier, url.PathEscape(f.Name))
		}
	}
	return ""
}

// ValidIdentifier reports whether id is a valid archive.org identifier.
func ValidIdentifier(id string) bool {
	return identifierRe.MatchString(id)
}

// ProxyCover fetches the cover image for id from archive.org and writes it to w,
// forwarding the Content-Type header. Response body is capped at 5 MB.
func ProxyCover(w http.ResponseWriter, id string) error {
	resp, err := httpClient.Get(fmt.Sprintf("%s/%s", coverBase, id))
	if err != nil {
		return fmt.Errorf("cover request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover request: unexpected status %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	io.Copy(w, io.LimitReader(resp.Body, 5<<20))
	return nil
}

// Search queries the archive.org search API.
func Search(query string, page, rows int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("output", "json")
	params.Set("rows", fmt.Sprintf("%d", rows))
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("fl[]", "identifier,title,mediatype,creator,date,description,downloads,subject,collection,language")

	resp, err := httpClient.Get(fmt.Sprintf("%s?%s", searchBase, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request: unexpected status %d", resp.StatusCode)
	}

	var raw struct {
		Response struct {
			Docs     []SearchResult `json:"docs"`
			NumFound int            `json:"numFound"`
		} `json:"response"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("search decode: %w", err)
	}

	return &SearchResponse{
		Results: raw.Response.Docs,
		Total:   raw.Response.NumFound,
	}, nil
}

// GetItem fetches full metadata and file list for a single item.
func GetItem(identifier string) (*Item, error) {
	if !identifierRe.MatchString(identifier) {
		return nil, fmt.Errorf("invalid identifier %q", identifier)
	}

	resp, err := httpClient.Get(fmt.Sprintf("%s/%s", metaBase, identifier))
	if err != nil {
		return nil, fmt.Errorf("metadata request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata request: unexpected status %d", resp.StatusCode)
	}

	var raw struct {
		Metadata ItemMetadata `json:"metadata"`
		Files    []ItemFile   `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("metadata decode: %w", err)
	}

	return &Item{
		Metadata: raw.Metadata,
		Files:    raw.Files,
	}, nil
}
