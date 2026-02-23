package main

import (
	"fmt"
	"log"
	"os"

	"Votan/internal/engine"
	"Votan/internal/storage"
	"Votan/internal/websocket" // Пакет, який ми зараз створимо
	"Votan/internal/youtube"   // Пакет для чату

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("📜 Сервер «Слов’яни проти Ящерів» стартує...")

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

	// Перевірка наявності ключів
	if videoID == "" || apiKey == "" {
		log.Println("⚠️ Попередження: YouTube ключі не задані. Працюватиме лише Адмін-панель.")
	} else {
		// 5. Запускаємо прослуховування чату (у фоні)
		go youtube.ListenChat(videoID, apiKey, gameLoop.CommandChan)
	}

	// 6. Запускаємо WebSocket сервер (він заблокує main, щоб програма не закрилась)
	// Передаємо gameLoop, щоб сервер мав доступ до CommandChan та GetState()
	websocket.StartServer(gameLoop)
}
