package engine

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// The server binds to loopback (see config.DefaultAddr), so any origin
	// reaching it is already on this machine. Allowing all origins is safe
	// under that threat model and keeps the OBS browser source working.
	CheckOrigin: func(*http.Request) bool { return true },
}

// HandleWebSocket serves both the overlay and the admin dashboard. It returns
// when the client disconnects or when ctx is cancelled (server shutdown).
//
// gorilla/websocket requires a single concurrent writer: only the loop below
// writes, and the reader goroutine never writes.
func HandleWebSocket(ctx context.Context, g *Game, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws: upgrade failed", "err", err)
		return
	}
	defer func() { _ = conn.Close() }()

	// Reader: forward inbound commands until the socket errors or we shut down.
	go func() {
		for {
			var cmd Command
			if err := conn.ReadJSON(&cmd); err != nil {
				return
			}
			select {
			case g.commands <- cmd:
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.WriteJSON(g.GetState()); err != nil {
				return // client closed; defer closes the conn, unblocking reader
			}
		}
	}
}
