package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"Votan/internal/engine"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Дозволяємо підключення з будь-якого джерела (для локального HTML)
	},
}

// StartServer запускає WebSocket на порту 8080
func StartServer(game *engine.Game) {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("❌ Помилка WebSocket:", err)
			return
		}
		defer conn.Close()

		fmt.Println("🟢 OBS (або браузер) підключено до гри!")

		// Безкінечний цикл відправки стану гри раз на секунду
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			<-ticker.C
			state := game.GetState()
			
			// Пакуємо в JSON і відправляємо
			jsonState, _ := json.Marshal(state)
			if err := conn.WriteMessage(websocket.TextMessage, jsonState); err != nil {
				fmt.Println("🔴 Клієнт відключився")
				break
			}
		}
	})

	fmt.Println("📡 WebSocket сервер запущено на ws://localhost:8080/ws")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("❌ Помилка запуску сервера:", err)
	}
}