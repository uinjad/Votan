package main

import (
	"fmt"
	"log"
	"os"

	"Votan/internal/engine"
	"Votan/internal/storage"
	"Votan/internal/websocket"
	"Votan/internal/youtube"

	"github.com/joho/godotenv" // Додали пакет для .env
)

func main() {
	fmt.Println("Сервер «Слов’яни проти Ящерів» стартує...")

	// 1. Завантажуємо секрети з .env
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ Файл .env не знайдено, використовуються системні змінні оточення")
	}

	// 2. Ініціалізуємо базу даних
	db, err := storage.InitDB("votan_game.db")
	if err != nil {
		log.Fatal("Критична помилка БД:", err)
	}
	defer db.Close()

	// 3. Ініціалізуємо рушій
	gameLoop := engine.NewGame(db)
	go gameLoop.Start()

	// 4. Дістаємо ключі з оточення
	videoID := os.Getenv("YOUTUBE_VIDEO_ID")
	apiKey := os.Getenv("YOUTUBE_API_KEY")

	if videoID == "" || apiKey == "" {
		log.Fatal("❌ КРИТИЧНО: YOUTUBE_VIDEO_ID або YOUTUBE_API_KEY не задані в .env")
	}

	// 5. Запускаємо прослуховування чату
	go youtube.ListenChat(videoID, apiKey, gameLoop.CommandChan)

	// 6. Запускаємо WebSocket
	websocket.StartServer(gameLoop)
}