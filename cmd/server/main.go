package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"Votan/internal/config"
	"Votan/internal/engine"
	"Votan/internal/obs"
	"Votan/internal/storage"
	"Votan/internal/youtube"
)

// Структура для передачі налаштувань у фронтенд
type ConfigData struct {
	YoutubeID string `json:"youtube_id"`
	ObsAddr   string `json:"obs_addr"`
	ObsPass   string `json:"obs_pass"`
	AdminSec  string `json:"admin_sec"`
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Printf("server: failed to auto-open browser: %v", err)
	}
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	envFile := config.ActivePath

	if r.Method == "GET" {
		data := ConfigData{
			YoutubeID: config.YouTubeVideoID,
			ObsAddr:   config.OBSAddr,
			ObsPass:   config.OBSPass,
			AdminSec:  config.AdminSecret,
		}
		json.NewEncoder(w).Encode(data)
		return
	}

	if r.Method == "POST" {
		var req ConfigData
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		content := fmt.Sprintf("YOUTUBE_VIDEO_ID=%s\nOBS_ADDR=%s\nOBS_PASS=%s\nADMIN_SECRET=%s\n",
			req.YoutubeID, req.ObsAddr, req.ObsPass, req.AdminSec)

		os.WriteFile(envFile, []byte(content), 0644)
		w.WriteHeader(http.StatusOK)
	}
}

func main() {
	// === 1. ІНТЕРФЕЙС ЗАПУСКУ ===
	fmt.Println("===================================================")
	fmt.Println("       VOTAN - ІНТЕРАКТИВНА ГРА ДЛЯ СТРІМУ")
	fmt.Println("===================================================")

	configFile := ".env"

	if len(os.Args) > 1 {
		configFile = os.Args[1]
		fmt.Printf("📂 Знайдено файл конфігурації: %s\n", configFile)
	} else {
		fmt.Print("📝 Вкажіть шлях до файлу конфігурації (або натисніть Enter для '.env'): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			configFile = input
		}
	}

	fmt.Printf("🚀 Запуск сервера з конфігом: %s...\n", configFile)

	// === 2. ІНІЦІАЛІЗАЦІЯ ===
	config.Load(configFile)

	// Динамічне сканування одягу
	config.ScanAssets("./web/public/assets")

	db, err := storage.InitDB("votan.db")
	if err != nil {
		log.Fatalf("server: failed to connect to database: %v", err)
	}
	defer db.Close()

	var obsClient *obs.Client
	if config.OBSAddr != "" && config.OBSPass != "" {
		obsClient, err = obs.NewClient(config.OBSAddr, config.OBSPass)
		if err != nil {
			log.Printf("server: OBS connection failed: %v", err)
		} else {
			log.Println("✅ OBS успішно підключено")
		}
	}

	game := engine.NewGame(db, obsClient)
	go game.Start()

	if config.YouTubeVideoID != "" {
		go youtube.ListenChat(config.YouTubeVideoID, game.CommandChan)
		log.Printf("✅ Слухаємо чат YouTube (ID: %s)\n", config.YouTubeVideoID)
	} else {
		log.Println("⚠️ YOUTUBE_VIDEO_ID не встановлено. Чат не читається.")
	}

	// === 3. МЕРЕЖА ТА БРАУЗЕР ===
	fs := http.FileServer(http.Dir("./web/public"))
	http.Handle("/", fs)

	http.HandleFunc("/api/config", configHandler)

	// Новий роут для передачі кількості картинок на фронтенд
	http.HandleFunc("/api/assets", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]int{
			"maxHead": config.MaxHeadID,
			"maxBody": config.MaxBodyID,
		})
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		engine.HandleWebSocket(game, w, r)
	})

	go func() {
		port := ":8080"
		if err := http.ListenAndServe(port, nil); err != nil {
			log.Fatalf("server: http server crashed: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	fmt.Println("🌐 Відкриваю Панель Деміурга...")
	openBrowser("http://localhost:8080/admin.html")

	select {}
}
