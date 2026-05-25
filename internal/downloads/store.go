// Package downloads manages the history and live state of ia CLI downloads.
//
// State is kept in memory and flushed to downloads.json in the same directory
// as config.env on every status change. The file is read once at startup to
// restore history across restarts.
//
// downloads.json is written atomically: to a temp file in the same directory,
// then renamed over the target — so a crash during write never corrupts history.
//
// Concurrent access is safe: a single mutex guards all reads and writes.
package downloads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Status values for a download entry.
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusError   = "error"
)

// identifierRe is the allowlist for archive.org identifiers.
// Only alphanumeric characters, hyphens, underscores, and dots are permitted.
var identifierRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// Entry represents one ia download, past or present.
type Entry struct {
	Identifier string    `json:"identifier"`
	Title      string    `json:"title"`
	Mediatype  string    `json:"mediatype"`
	SavePath   string    `json:"save_path"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	Progress   string    `json:"progress,omitempty"` // last output line; in-memory only
}

// Store holds all download entries and manages persistence.
type Store struct {
	mu       sync.Mutex
	entries  []*Entry
	dataFile string // absolute path to downloads.json
	iaBin    string // absolute path to ia binary
}

// New creates a Store. dataDir is the directory that also holds config.env.
// iaBin is the absolute path to the ia CLI binary.
// Existing history is loaded from downloads.json if present.
func New(dataDir, iaBin string) (*Store, error) {
	s := &Store{
		dataFile: filepath.Join(dataDir, "downloads.json"),
		iaBin:    iaBin,
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load download history: %w", err)
	}
	// Mark any entries left in "running" state as errored —
	// they were interrupted by a previous restart.
	for _, e := range s.entries {
		if e.Status == StatusRunning {
			e.Status = StatusError
			e.Error = "interrupted by restart"
			e.FinishedAt = time.Now()
		}
	}
	if err := s.flush(); err != nil {
		return nil, fmt.Errorf("flush on startup: %w", err)
	}
	return s, nil
}

// Start validates inputs, records a new entry, and launches `ia download`
// in a goroutine. Returns the new entry or an error if validation fails.
//
// allowedPaths is the whitelist from config — the submitted savePath must
// be an exact match against one of these values.
func (s *Store) Start(identifier, title, mediatype, savePath string, allowedPaths []string) (*Entry, error) {
	// Validate identifier — must match allowlist regexp
	if !identifierRe.MatchString(identifier) {
		return nil, fmt.Errorf("invalid identifier %q: only [a-zA-Z0-9_.-] allowed", identifier)
	}

	// Validate save path — exact match against config whitelist only
	if !PathAllowed(savePath, allowedPaths) {
		return nil, fmt.Errorf("save path %q is not in the allowed paths list", savePath)
	}

	entry := &Entry{
		Identifier: identifier,
		Title:      title,
		Mediatype:  mediatype,
		SavePath:   savePath,
		StartedAt:  time.Now(),
		Status:     StatusRunning,
	}

	s.mu.Lock()
	s.entries = append(s.entries, entry)
	if err := s.flush(); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("flush on start: %w", err)
	}
	s.mu.Unlock()

	go s.run(entry)
	return entry, nil
}

// List returns a copy of all entries, newest first.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	for i, e := range s.entries {
		out[i] = *e
	}
	// Reverse so newest is first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Remove deletes entries with the given identifiers from history. Files on
// disk are not touched.
func (s *Store) Remove(identifiers []string) error {
	set := make(map[string]bool, len(identifiers))
	for _, id := range identifiers {
		set[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := s.entries[:0]
	for _, e := range s.entries {
		if !set[e.Identifier] {
			remaining = append(remaining, e)
		}
	}
	s.entries = remaining
	return s.flush()
}

// Delete removes entries with the given identifiers from history and also
// deletes the downloaded files from disk (savepath/identifier/).
func (s *Store) Delete(identifiers []string) error {
	set := make(map[string]bool, len(identifiers))
	for _, id := range identifiers {
		set[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []string
	var remaining []*Entry
	for _, e := range s.entries {
		if !set[e.Identifier] {
			remaining = append(remaining, e)
			continue
		}
		dir := filepath.Join(e.SavePath, e.Identifier)
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, err.Error())
			remaining = append(remaining, e) // keep entry if disk delete failed
		}
	}
	s.entries = remaining
	if err := s.flush(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("some deletes failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// run executes `ia download` and updates the entry status on completion.
// Each argument is passed as a separate string — never via shell interpolation.
// Output is streamed line-by-line into Entry.Progress for live display.
func (s *Store) run(e *Entry) {
	cmd := exec.Command(s.iaBin, "download", e.Identifier, "--destdir", e.SavePath)
	cmd.Env = safeEnv()

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		s.mu.Lock()
		e.Status = StatusError
		e.Error = err.Error()
		e.FinishedAt = time.Now()
		s.flush()
		s.mu.Unlock()
		return
	}

	var outBuf strings.Builder
	var scanDone sync.WaitGroup
	scanDone.Add(1)
	go func() {
		defer scanDone.Done()
		scanner := bufio.NewScanner(pr)
		scanner.Split(splitCRLF)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			outBuf.WriteString(line + "\n")
			s.mu.Lock()
			e.Progress = line
			s.mu.Unlock()
		}
	}()

	err := cmd.Wait()
	pw.Close()
	scanDone.Wait()
	pr.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	e.FinishedAt = time.Now()
	e.Progress = ""
	if err != nil {
		e.Status = StatusError
		e.Error = fmt.Sprintf("%v: %s", err, truncate(outBuf.String(), 500))
	} else {
		e.Status = StatusDone
	}
	if err := s.flush(); err != nil {
		log.Printf("downloads: failed to persist entry %s: %v", e.Identifier, err)
	}
}

// splitCRLF is a bufio.SplitFunc that treats \r and \n (and \r\n) as line
// terminators. This handles tqdm's carriage-return-based progress output.
func splitCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\r' || b == '\n' {
			end := i + 1
			if b == '\r' && end < len(data) && data[end] == '\n' {
				end++
			}
			return end, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// load reads downloads.json into memory. Missing file is not an error.
func (s *Store) load() error {
	data, err := os.ReadFile(s.dataFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.entries)
}

// flush writes entries to downloads.json atomically.
// Must be called with s.mu held.
func (s *Store) flush() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.dataFile)
	tmp, err := os.CreateTemp(dir, ".downloads-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.dataFile)
}

// PathAllowed returns true if candidate is an exact match for one of allowed.
func PathAllowed(candidate string, allowed []string) bool {
	for _, p := range allowed {
		if p == candidate {
			return true
		}
	}
	return false
}

// truncate shortens s to at most n runes, appending "…" if cut.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// safeEnv returns a copy of the current environment with variables that could
// alter subprocess behaviour stripped out. ia is a Python tool; PYTHONPATH,
// PYTHONSTARTUP, etc. can redirect or inject code into its runtime.
func safeEnv() []string {
	blocked := map[string]bool{
		"PYTHONPATH":      true,
		"PYTHONSTARTUP":   true,
		"PYTHONHOME":      true,
		"LD_PRELOAD":      true,
		"LD_LIBRARY_PATH": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		if idx := strings.IndexByte(kv, '='); idx > 0 && blocked[kv[:idx]] {
			continue
		}
		env = append(env, kv)
	}
	return env
}
