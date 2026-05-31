package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Game rules. These are genuinely constant and shared across packages, so they
// stay as compile-time constants (immutable, race-free) rather than being
// injected as state.
const (
	MaxX            = 20
	MaxY            = 35
	MaxStepsPerTurn = 35

	BossMaxHP     = 100
	BossHitDamage = 10

	VoteDuration  = 60 * time.Second
	VoteResultTTL = 10 * time.Second

	Attack5GDuration = 30 * time.Second
	Debuff5GDuration = 1 * time.Minute

	PlayerTimeout = 20 * time.Minute
	ChatBubbleTTL = 10 * time.Second
)

// Defaults for the injected runtime configuration.
const (
	// DefaultAddr binds to loopback on purpose: the admin panel and the
	// /api/config endpoint expose local secrets, so the server must not be
	// reachable from the network. See README "Known limitations".
	DefaultAddr      = "127.0.0.1:8080"
	DefaultDBPath    = "votan.db"
	DefaultWebDir    = "./web/public"
	DefaultAssetsDir = "./web/public/assets"
)

// Config is the runtime configuration. It is loaded once and injected into the
// components that need it; nothing here is global mutable state.
type Config struct {
	Addr           string
	OBSAddr        string
	OBSPass        string
	YouTubeVideoID string
	AdminSecret    string

	DBPath    string
	WebDir    string
	AssetsDir string

	// ActivePath is the dotenv file this config was loaded from, so the admin
	// panel can persist edits back to the same file.
	ActivePath string
}

// Load reads a dotenv-style file and returns the resulting Config. A missing
// file is not an error: sane defaults are returned so the app runs on a fresh
// checkout. Any other read error is propagated to the caller.
func Load(path string) (*Config, error) {
	cfg := &Config{
		Addr:       DefaultAddr,
		DBPath:     DefaultDBPath,
		WebDir:     DefaultWebDir,
		AssetsDir:  DefaultAssetsDir,
		ActivePath: path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil // first run: use defaults
		}
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "OBS_ADDR":
			cfg.OBSAddr = val
		case "OBS_PASS":
			cfg.OBSPass = val
		case "YOUTUBE_VIDEO_ID":
			cfg.YouTubeVideoID = val
		case "ADMIN_SECRET":
			cfg.AdminSecret = val
		case "LISTEN_ADDR":
			if val != "" {
				cfg.Addr = val
			}
		}
	}
	return cfg, nil
}

var (
	headRe = regexp.MustCompile(`^head_(\d+)\.png$`)
	bodyRe = regexp.MustCompile(`^body_(\d+)\.png$`)
)

// ScanAssets discovers the highest head_N / body_N skin ids in dir so the
// available range isn't hardcoded and can grow by dropping files in. A missing
// directory yields (0, 0, nil).
func ScanAssets(dir string) (maxHead, maxBody int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("config: scan assets %q: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if m := headRe.FindStringSubmatch(name); m != nil {
			if id, _ := strconv.Atoi(m[1]); id > maxHead {
				maxHead = id
			}
		} else if m := bodyRe.FindStringSubmatch(name); m != nil {
			if id, _ := strconv.Atoi(m[1]); id > maxBody {
				maxBody = id
			}
		}
	}
	return maxHead, maxBody, nil
}
