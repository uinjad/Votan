package config

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Ігрові константи
const (
	MaxX             = 20
	MaxY             = 35
	MaxStepsPerTurn  = 35
	BossMaxHP        = 50
	BossHitDamage    = 10
	VoteDuration     = 60 * time.Second
	VoteResultTTL    = 10 * time.Second
	Attack5GDuration = 30 * time.Second
	Debuff5GDuration = 1 * time.Minute
	PlayerTimeout    = 10 * time.Minute
	ChatBubbleTTL    = 10 * time.Second
)

// Змінні конфігурації (тепер ліміти одягу тут, бо вони динамічні)
var (
	OBSAddr        string
	OBSPass        string
	YouTubeVideoID string
	AdminSecret    = "lucifer_secret"
	ActivePath     = ".env"

	MaxHeadID int
	MaxBodyID int
)

func Load(path string) {
	ActivePath = path
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("config: файл %s не знайдено\n", path)
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "OBS_ADDR":
			OBSAddr = val
		case "OBS_PASS":
			OBSPass = val
		case "YOUTUBE_VIDEO_ID":
			YouTubeVideoID = val
		case "ADMIN_SECRET":
			AdminSecret = val
		}
	}
}

// ДОДАНО: Динамічний сканер асетів
func ScanAssets(assetsDir string) {
	files, err := os.ReadDir(assetsDir)
	if err != nil {
		log.Printf("config: Не вдалося прочитати папку з асетами: %v\n", err)
		return
	}

	headRegex := regexp.MustCompile(`^head_(\d+)\.png$`)
	bodyRegex := regexp.MustCompile(`^body_(\d+)\.png$`)

	maxH, maxB := 0, 0

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()

		if m := headRegex.FindStringSubmatch(name); m != nil {
			id, _ := strconv.Atoi(m[1])
			if id > maxH {
				maxH = id
			}
		} else if m := bodyRegex.FindStringSubmatch(name); m != nil {
			id, _ := strconv.Atoi(m[1])
			if id > maxB {
				maxB = id
			}
		}
	}

	MaxHeadID = maxH
	MaxBodyID = maxB
	log.Printf("👕 Знайдено асетів: Голів до %d, Тіл до %d\n", MaxHeadID, MaxBodyID)
}
