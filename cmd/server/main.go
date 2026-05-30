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

// ConfigData is the settings payload exchanged with the frontend.
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
	// === 1. STARTUP PROMPT ===
	fmt.Println("===================================================")
	fmt.Println("       VOTAN - INTERACTIVE STREAM GAME")
	fmt.Println("===================================================")

	configFile := ".env"

	if len(os.Args) > 1 {
		configFile = os.Args[1]
		fmt.Printf("Using config file: %s\n", configFile)
	} else {
		fmt.Print("Enter a config file path (or press Enter for '.env'): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			configFile = input
		}
	}

	fmt.Printf("Starting server with config: %s...\n", configFile)

	// === 2. INITIALIZATION ===
	config.Load(configFile)

	// Scan skin assets at runtime.
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
			log.Println("OBS connected")
		}
	}

	game := engine.NewGame(db, obsClient)
	go game.Start()

	if config.YouTubeVideoID != "" {
		go youtube.ListenChat(config.YouTubeVideoID, game.CommandChan)
		log.Printf("Listening to YouTube chat (id: %s)\n", config.YouTubeVideoID)
	} else {
		log.Println("YOUTUBE_VIDEO_ID is not set; chat will not be read.")
	}

	// === 3. HTTP + BROWSER ===
	fs := http.FileServer(http.Dir("./web/public"))
	http.Handle("/", fs)

	http.HandleFunc("/api/config", configHandler)

	// Reports asset counts to the frontend.
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

	fmt.Println("Opening the Demiurge panel...")
	openBrowser("http://localhost:8080/admin.html")

	select {}
}
