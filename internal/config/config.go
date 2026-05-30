package config

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Game constants.
const (
	MaxX             = 20
	MaxY             = 35
	MaxStepsPerTurn  = 35
	BossMaxHP        = 100
	BossHitDamage    = 10
	VoteDuration     = 60 * time.Second
	VoteResultTTL    = 10 * time.Second
	Attack5GDuration = 30 * time.Second
	Debuff5GDuration = 1 * time.Minute
	PlayerTimeout    = 20 * time.Minute
	ChatBubbleTTL    = 10 * time.Second
)

// Runtime config. Skin limits live here because they're discovered dynamically.
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
		log.Printf("config: file %s not found\n", path)
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

// ScanAssets discovers the highest head_N / body_N skin ids in the assets dir,
// so the available range isn't hardcoded and can be updated by dropping files in.
func ScanAssets(assetsDir string) {
	files, err := os.ReadDir(assetsDir)
	if err != nil {
		log.Printf("config: could not read assets dir: %v\n", err)
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
	log.Printf("Assets found: heads up to %d, bodies up to %d\n", MaxHeadID, MaxBodyID)
}
