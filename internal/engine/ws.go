package engine

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // accept any origin (local UI only)
	},
}

// HandleWebSocket serves both the game overlay and the admin dashboard.
func HandleWebSocket(g *Game, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	// 1. Reader goroutine for incoming commands (from the admin panel or Flipper).
	go func() {
		for {
			var cmd Command
			if err := conn.ReadJSON(&cmd); err != nil {
				return // client disconnected
			}
			g.CommandChan <- cmd
		}
	}()

	// 2. Push game state to the client every 100 ms.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		state := g.GetState()
		if err := conn.WriteJSON(state); err != nil {
			return // browser closed
		}
	}
}
