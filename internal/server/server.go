package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"Votan/internal/config"
	"Votan/internal/engine"
	"Votan/internal/web"
)

const shutdownTimeout = 5 * time.Second

// Server wires the HTTP layer: static assets, the config/asset APIs and the
// game WebSocket. It binds to a loopback address by default because the config
// endpoint exposes local secrets to the admin panel.
type Server struct {
	cfgMu sync.RWMutex
	cfg   *config.Config

	game    *engine.Game
	maxHead int
	maxBody int

	http    *http.Server
	baseCtx context.Context
}

// configPayload is the settings DTO exchanged with the admin panel. Field tags
// match the existing frontend (web/public/admin.html).
type configPayload struct {
	YoutubeID string `json:"youtube_id"`
	ObsAddr   string `json:"obs_addr"`
	ObsPass   string `json:"obs_pass"`
	AdminSec  string `json:"admin_sec"`
}

func New(cfg *config.Config, game *engine.Game, maxHead, maxBody int) *Server {
	s := &Server{
		cfg:     cfg,
		game:    game,
		maxHead: maxHead,
		maxBody: maxBody,
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(web.Assets()))
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/assets", s.handleAssets)
	mux.HandleFunc("/ws", s.handleWS)

	s.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Run starts serving and blocks until ctx is cancelled, then shuts the server
// down gracefully. The base ctx is handed to each WebSocket handler so live
// connections close on shutdown.
func (s *Server) Run(ctx context.Context) error {
	s.baseCtx = ctx

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server: listening", "addr", s.http.Addr)
		err := s.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server: listen: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: shutdown: %w", err)
		}
		slog.Info("server: stopped")
		return nil
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	// Pass the application ctx (not r.Context()): a hijacked WebSocket conn
	// must stay alive for its own lifetime and only close on real shutdown.
	engine.HandleWebSocket(s.baseCtx, s.game, w, r)
}

func (s *Server) handleAssets(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{
		"maxHead": s.maxHead,
		"maxBody": s.maxBody,
	}); err != nil {
		slog.Error("server: encode assets", "err", err)
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Localhost-only endpoint: returns the local OBS/admin secrets to the
		// operator's own panel. Safe because the server binds to loopback.
		s.cfgMu.RLock()
		payload := configPayload{
			YoutubeID: s.cfg.YouTubeVideoID,
			ObsAddr:   s.cfg.OBSAddr,
			ObsPass:   s.cfg.OBSPass,
			AdminSec:  s.cfg.AdminSecret,
		}
		s.cfgMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("server: encode config", "err", err)
		}

	case http.MethodPost:
		var req configPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := s.persistConfig(req); err != nil {
			slog.Error("server: persist config", "err", err)
			http.Error(w, "could not save config", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// stripNewlines removes characters that would let a field break out of its
// line in the dotenv file and inject extra keys (e.g. a crafted OBS password
// containing "\nLISTEN_ADDR=0.0.0.0:8080" would expose the secrets endpoint).
func stripNewlines(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func (s *Server) persistConfig(req configPayload) error {
	content := fmt.Sprintf(
		"YOUTUBE_VIDEO_ID=%s\nOBS_ADDR=%s\nOBS_PASS=%s\nADMIN_SECRET=%s\n",
		stripNewlines(req.YoutubeID), stripNewlines(req.ObsAddr),
		stripNewlines(req.ObsPass), stripNewlines(req.AdminSec),
	)

	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	// 0600: the file holds the OBS password and admin secret.
	if err := os.WriteFile(s.cfg.ActivePath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %q: %w", s.cfg.ActivePath, err)
	}

	// Reflect edits in the in-memory copy the GET handler serves. Changes take
	// full effect (engine auth, OBS/YouTube) only after a restart.
	s.cfg.YouTubeVideoID = req.YoutubeID
	s.cfg.OBSAddr = req.ObsAddr
	s.cfg.OBSPass = req.ObsPass
	s.cfg.AdminSecret = req.AdminSec
	return nil
}
