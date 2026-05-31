package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"Votan/internal/config"
	"Votan/internal/engine"
	"Votan/internal/obs"
	"Votan/internal/server"
	"Votan/internal/storage"
	"Votan/internal/web"
	"Votan/internal/youtube"
)

// version is the build version, injected at release time via
//
//	-ldflags "-X main.version=v1.2.3"
//
// and defaults to "dev" for local builds.
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Cancelled on Ctrl+C / SIGTERM — the single source of shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("Votan starting", "version", version, "go", runtime.Version())

	configPath := ".env"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	maxHead, maxBody := web.ScanSkins()
	slog.Info("assets scanned", "max_head", maxHead, "max_body", maxBody)

	db, err := storage.InitDB(cfg.DBPath)
	if err != nil {
		return err
	}
	// Runs last: by the time we get here the game loop has stopped, so this
	// flushes any queued writes and closes the DB cleanly.
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("storage close", "err", err)
		}
	}()

	// OBS is optional; without it the engine uses a no-op Scene.
	var scene engine.Scene = engine.NopScene{}
	if cfg.OBSAddr != "" && cfg.OBSPass != "" {
		client, err := obs.NewClient(cfg.OBSAddr, cfg.OBSPass)
		if err != nil {
			slog.Warn("obs: connection failed, running without OBS", "err", err)
		} else {
			slog.Info("obs: connected")
			scene = client
			defer func() {
				if err := client.Close(); err != nil {
					slog.Warn("obs close", "err", err)
				}
			}()
		}
	}

	game := engine.NewGame(db, scene, engine.Config{
		AdminSecret: cfg.AdminSecret,
		MaxHeadID:   maxHead,
		MaxBodyID:   maxBody,
	})
	if err := game.RestorePlayers(ctx); err != nil {
		slog.Warn("could not restore players", "err", err)
	}

	gameDone := make(chan struct{})
	go func() {
		defer close(gameDone)
		game.Run(ctx)
	}()

	if cfg.YouTubeVideoID != "" {
		go youtube.ListenChat(ctx, cfg.YouTubeVideoID, game.Commands())
		slog.Info("listening to YouTube chat", "video", cfg.YouTubeVideoID)
	} else {
		slog.Info("YOUTUBE_VIDEO_ID not set; chat disabled")
	}

	srv := server.New(cfg, game, maxHead, maxBody)
	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Run(ctx) }()

	go openAdminPanel(ctx, cfg.Addr)

	// Block until a shutdown signal or a fatal server error.
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-srvDone:
		if err != nil {
			slog.Error("server error", "err", err)
		}
		stop() // unwind everything else
		<-gameDone
		return err
	}

	// Graceful shutdown order: server (already reacting to ctx) -> game loop ->
	// deferred OBS/DB close.
	if err := <-srvDone; err != nil {
		slog.Error("server stopped with error", "err", err)
	}
	<-gameDone
	slog.Info("shutdown complete")
	return nil
}

// openAdminPanel waits until the listener actually accepts connections (instead
// of a racy fixed sleep), then opens the operator's browser.
func openAdminPanel(ctx context.Context, addr string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	url := "http://" + addr + "/admin.html"
	slog.Info("opening admin panel", "url", url)
	if err := openBrowser(url); err != nil {
		slog.Warn("could not auto-open browser", "err", err)
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}
}
