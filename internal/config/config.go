// Package config loads and validates iabridge configuration from a config.env file.
// Path to the file is read from CONFIG_PATH env var, defaulting to ./config.env.
//
// Required fields: ALLOWED_CIDR, QBITTORRENT_URL, QBITTORRENT_USER,
//                  QBITTORRENT_PASS, QBITTORRENT_SAVE_PATHS
// Optional fields: PORT (default 8090), IA_BIN (auto-detected), SHOW_COVERS (default false)
//
// ALLOWED_CIDR must be valid CIDR notation (e.g. 192.168.1.0/24).
// It is parsed into a net.IPNet at startup and passed to the LANOnly middleware.
//
// QBITTORRENT_SAVE_PATHS is a comma-separated list of absolute paths.
// Each path is validated: must be absolute, cleaned via filepath.Clean,
// and must exist on disk (os.Stat). Invalid paths cause startup to halt.
//
// If IA_BIN is empty, Load() locates `ia` via exec.LookPath.
// If required fields are missing or invalid, Load() returns a descriptive error.
package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds validated runtime configuration.
type Config struct {
	Port             string
	AllowedNet       *net.IPNet // parsed from ALLOWED_CIDR
	AllowedCIDR      string     // raw string, for display on config page
	QbittorrentURL   string
	QbittorrentUser  string
	QbittorrentPass  string
	QbittorrentPaths []string // validated absolute paths from QBITTORRENT_SAVE_PATHS
	IABin            string   // absolute path to ia binary
	ShowCovers       bool
	DataDir          string // directory containing config.env — used for downloads.json
}

// Load reads config from the file at CONFIG_PATH (default: ./config.env),
// validates all required fields, and returns a ready-to-use Config.
func Load() (*Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "./config.env"
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	vals := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, _ := strings.Cut(line, "=")
		vals[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Validate required fields up front so the error lists everything at once.
	var missing []string
	for _, k := range []string{"ALLOWED_CIDR", "QBITTORRENT_URL", "QBITTORRENT_USER", "QBITTORRENT_PASS", "QBITTORRENT_SAVE_PATHS"} {
		if vals[k] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required config fields: %s", strings.Join(missing, ", "))
	}

	cfg := &Config{
		Port:            "8090",
		AllowedCIDR:     vals["ALLOWED_CIDR"],
		QbittorrentURL:  vals["QBITTORRENT_URL"],
		QbittorrentUser: vals["QBITTORRENT_USER"],
		QbittorrentPass: vals["QBITTORRENT_PASS"],
		DataDir:         filepath.Dir(absPath),
	}

	_, ipNet, err := net.ParseCIDR(vals["ALLOWED_CIDR"])
	if err != nil {
		return nil, fmt.Errorf("invalid ALLOWED_CIDR %q: %w", vals["ALLOWED_CIDR"], err)
	}
	cfg.AllowedNet = ipNet

	for _, p := range strings.Split(vals["QBITTORRENT_SAVE_PATHS"], ",") {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "." {
			continue
		}
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("save path %q is not absolute", p)
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("save path %q: %w", p, err)
		}
		cfg.QbittorrentPaths = append(cfg.QbittorrentPaths, p)
	}
	if len(cfg.QbittorrentPaths) == 0 {
		return nil, fmt.Errorf("QBITTORRENT_SAVE_PATHS contains no valid paths")
	}

	if v := vals["PORT"]; v != "" {
		cfg.Port = v
	}

	if v := vals["SHOW_COVERS"]; v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid SHOW_COVERS %q: must be true or false", v)
		}
		cfg.ShowCovers = b
	}

	if v := vals["IA_BIN"]; v != "" {
		cfg.IABin = v
	} else {
		bin, err := exec.LookPath("ia")
		if err != nil {
			return nil, fmt.Errorf("IA_BIN not set and 'ia' not found in PATH: %w", err)
		}
		cfg.IABin = bin
	}

	return cfg, nil
}
