package main

import (
	"fmt"
	"log"
	"os"

	"Votan/internal/engine"
	"Votan/internal/obs"
	"Votan/internal/storage"
	"Votan/internal/websocket"
	"Votan/internal/youtube"

	"github.com/joho/godotenv"
)

func main() {
	// 1. Завантажуємо конфігурацію з файлу .env
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ Не знайдено файл .env, використовуються системні змінні оточення.")
	}

	// 2. Ініціалізуємо базу даних SQLite
	db, err := storage.InitDB("votan_game.db")
	if err != nil {
		log.Fatal("❌ Помилка ініціалізації БД:", err)
	}
	defer db.Close()
	fmt.Println("✅ База даних успішно підключена.")

	// 3. Підключаємо керування OBS WebSocket
	obsAddr := os.Getenv("OBS_ADDR")
	obsPass := os.Getenv("OBS_PASS")
	obsClient, err := obs.NewClient(obsAddr, obsPass)
	if err != nil {
		log.Printf("⚠️ OBS не підключено (%v). Гра працюватиме без автоматизації сцени.", err)
	} else {
		fmt.Println("✅ OBS WebSocket підключено успішно!")
	}

	// 4. Ініціалізуємо ядро гри
	gameLoop := engine.NewGame(db, obsClient)

	// ДОДАНО: Відновлюємо гравців з бази даних (зберігає стан після рестарту)
	gameLoop.RestorePlayersFromDB()

	// 5. Запускаємо ігровий цикл (тіки) в окремій горутині
	go gameLoop.Start()

	// 6. Підключаємо прослуховування YouTube чату (Скрапер "Всі повідомлення")
	videoID := os.Getenv("YOUTUBE_VIDEO_ID")
	if videoID != "" {
		go youtube.ListenChat(videoID, gameLoop.CommandChan)
	} else {
		fmt.Println("⚠️ YOUTUBE_VIDEO_ID не знайдено в .env. Грати можна тільки через адмінку.")
	}

	// 7. Запускаємо WebSocket сервер для трансляції екрану гри
	fmt.Println("🚀 Ефірний сервер запущено! Відкрий http://localhost:8080")
	websocket.StartServer(gameLoop)
}
