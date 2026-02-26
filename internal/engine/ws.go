package engine

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Дозволяємо підключення з будь-якого джерела (для локального UI)
	},
}

// HandleWebSocket керує підключеннями клієнтів (як гри, так і адмінки)
func HandleWebSocket(g *Game, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	// 1. Горутина для читання команд (з адмінки або фліппера)
	go func() {
		for {
			var cmd Command
			if err := conn.ReadJSON(&cmd); err != nil {
				return // Клієнт відключився
			}
			g.CommandChan <- cmd
		}
	}()

	// 2. Цикл відправки стану гри на фронтенд (кожні 100мс)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Використовуємо for range замість select (виправлено попередження S1000)
	for range ticker.C {
		state := g.GetState()
		if err := conn.WriteJSON(state); err != nil {
			return // Якщо браузер закрили - виходимо
		}
	}
}
