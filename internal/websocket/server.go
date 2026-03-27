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
		return true // Дозволяємо підключення з локального HTML
	},
}

// Зберігаємо всі активні з'єднання (OBS, браузери, Мок)
var clients = make(map[*websocket.Conn]bool)

// StartServer запускає WebSocket та веб-сервер на порту 8080
func StartServer(game *engine.Game) {

	// 1. Роздаємо статичні файли (index.html, mock.html, папку assets)
	// Це дозволить тобі відкривати http://localhost:8080/mock.html
	http.Handle("/", http.FileServer(http.Dir("./web/public")))

	// 2. Обробка WebSocket підключень
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Помилка WebSocket:", err)
			return
		}
		defer conn.Close()

		fmt.Println("Клієнт підключився (OBS або Мок)!")

		// Додаємо клієнта в загальний список
		clients[conn] = true
		defer delete(clients, conn) // Видалимо, коли він відключиться

		// ЦИКЛ ЧИТАННЯ: Слухаємо команди від mock.html
		for {
			var cmd engine.Command
			err := conn.ReadJSON(&cmd)
			if err != nil {
				// Якщо браузер закрили
				fmt.Println("Клієнт відключився")
				break
			}

			// Якщо прийшла команда від Мока, кидаємо її в рушій гри
			if cmd.PlayerID != "" {
				game.CommandChan <- cmd
				fmt.Printf("🛠 Мок-команда: %s від %s\n", cmd.Action, cmd.PlayerName)
			}
		}
	})

	// 3. БЕЗКІНЕЧНА РОЗСИЛКА (в окремому потоці)
	go func() {
		// Оновлюємо гру трохи швидше для плавності (10 разів на сек)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			<-ticker.C
			state := game.GetState()
			jsonState, _ := json.Marshal(state)

			// Відправляємо новий кадр всім підключеним клієнтам
			for client := range clients {
				if err := client.WriteMessage(websocket.TextMessage, jsonState); err != nil {
					client.Close()
					delete(clients, client)
				}
			}
		}
	}()

	fmt.Println("Веб-сервер та WebSocket запущено на http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Помилка запуску сервера:", err)
	}
}
