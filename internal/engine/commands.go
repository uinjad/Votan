package engine

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Votan/internal/config"
)

var skinRegex = regexp.MustCompile(`(?i)^!h(\d+)b(\d+)$`)

const OBSSceneName = "Main"
const OBSWebcamSource = "camera"
const OBSSubscribeMovie = "subscribeMovie"
const OBSSubscribeSong = "subscribeSong"

func (g *Game) processCommand(cmd Command) {
	actionStr := strings.TrimSpace(cmd.Action)
	actionLower := strings.ToLower(actionStr)

	if cmd.PlayerID == config.AdminSecret {
		g.handleAdminCommand(actionStr, actionLower)
		return
	}

	player, exists := g.Players[cmd.PlayerID]
	if !exists {
		spawnPos, ok := g.findFreeSpawn()
		if !ok {
			return
		}
		player = &Player{
			ID: cmd.PlayerID, Name: strings.TrimPrefix(cmd.PlayerName, "@"),
			Pos: spawnPos, LastActive: time.Now(),
		}
		g.Players[cmd.PlayerID] = player
		g.Grid[spawnPos] = player
		g.DB.UpsertUser(player.ID, player.Name, player.Pos.X, player.Pos.Y)
	}

	player.LastActive = time.Now()

	if player.IsIrradiated && actionStr != "" {
		phrases := []string{"Хочу ревакцинуватись!", "Піду шукати роботу в офісі!", "Слава Ящерам!", "5G - це здоров'я!"}
		player.LastMessage = phrases[rand.Intn(len(phrases))]
		player.MessageTime = time.Now()
		player.RemainingSteps = 0
		return
	}

	if actionStr != "" {
		isCommand := false

		if actionLower == "!hit" && g.BossActive {
			g.BossHP -= config.BossHitDamage
			if g.BossHP <= 0 {
				g.BossHP = 0
				g.BossActive = false

				if g.OBS != nil {
					go g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeMovie, true)
					go g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeSong, true)
					go g.OBS.RestartMedia(OBSSubscribeMovie)
					go g.OBS.RestartMedia(OBSSubscribeSong)

					// ВИПРАВЛЕНО: Миттєве повернення без анімації
					go g.OBS.SetOpacity(OBSWebcamSource, "Fade", 1.0)
				}
				fmt.Println("⚔️ Ящера повалено силами русичів! Запущено анімацію підписки.")
			}
			isCommand = true

		} else if matches := skinRegex.FindStringSubmatch(actionLower); matches != nil {
			if player.Status == 1 {
				h, _ := strconv.Atoi(matches[1])
				b, _ := strconv.Atoi(matches[2])
				if h >= 0 && h <= config.MaxHeadID && b >= 0 && b <= config.MaxBodyID {
					player.HeadID = h
					player.BodyID = b
					g.DB.UpdateSkin(player.ID, h, b)
				}
			} else {
				player.LastMessage = "Тільки Хрещені можуть змінювати зовнішність!"
				player.MessageTime = time.Now()
			}
			isCommand = true

		} else if strings.HasPrefix(actionLower, "!") {
			dx, dy, steps := parseAction(actionLower)
			if steps > 0 {
				player.TargetDx = dx
				player.TargetDy = dy
				player.RemainingSteps = steps
				isCommand = true
			}
		}

		if !isCommand && !strings.HasPrefix(actionStr, "!") {
			player.LastMessage = actionStr
			player.MessageTime = time.Now()
		}
	}
}

func (g *Game) handleAdminCommand(actionStr, actionLower string) {
	if strings.HasPrefix(actionLower, "!віче") {
		g.VoteActive = true
		g.VoteResult = ""
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(actionStr, "!віче")), "|")
		g.VoteTopic = "ВИБІР ДОЛІ"
		g.VoteOptionA = "ЗА"
		g.VoteOptionB = "ПРОТИ"
		if len(parts) > 0 && parts[0] != "" {
			g.VoteTopic = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			g.VoteOptionA = parts[1]
		}
		if len(parts) > 2 && parts[2] != "" {
			g.VoteOptionB = parts[2]
		}
		g.VoteEndTime = time.Now().Add(config.VoteDuration)

	} else if actionLower == "!stop_vote" {
		if g.VoteActive {
			g.VoteEndTime = time.Now().Add(-1 * time.Second)
		}

	} else if actionLower == "!5g" {
		g.Attack5GActive = true
		g.Attack5GEndTime = time.Now().Add(config.Attack5GDuration)
		g.Attack5GZones = make(map[Pos]bool)
		for i := 0; i < 4; i++ {
			cx, cy := rand.Intn(config.MaxX-4)+2, rand.Intn(config.MaxY-4)+2
			for dx := -2; dx <= 2; dx++ {
				for dy := -2; dy <= 2; dy++ {
					g.Attack5GZones[Pos{X: cx + dx, Y: cy + dy}] = true
				}
			}
		}

	} else if actionLower == "!ящер" {
		if !g.BossActive {
			g.BossActive = true
			g.BossHP = config.BossMaxHP

			if g.OBS != nil {
				go g.OBS.FadeSourceOpacity(OBSWebcamSource, "Fade", 1.0, 0.0, 20*time.Second)
			}
		}

	} else if actionLower == "!kill_boss" {
		if g.BossActive {
			g.BossActive = false
			g.BossHP = 0

			if g.OBS != nil {
				go g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeMovie, true)
				go g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeSong, true)
				go g.OBS.RestartMedia(OBSSubscribeMovie)
				go g.OBS.RestartMedia(OBSSubscribeSong)

				// ВИПРАВЛЕНО: Миттєве повернення без анімації
				go g.OBS.SetOpacity(OBSWebcamSource, "Fade", 1.0)
			}
			fmt.Println("👑 Деміург власноруч знищив Ящера!")
		}

	} else if actionLower == "!fix_obs" {
		g.BossActive = false
		g.BossHP = 0
		if g.OBS != nil {
			go g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeMovie, false)
			go g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeSong, false)

			// ВИПРАВЛЕНО: Миттєве повернення без анімації
			go g.OBS.SetOpacity(OBSWebcamSource, "Fade", 1.0)
		}
		fmt.Println("🔧 Стан OBS та Ящера примусово скинуто до дефолтного!")

	} else if strings.HasPrefix(actionLower, "!kick") {
		targetID := strings.TrimSpace(strings.TrimPrefix(actionStr, "!kick"))
		if p, ok := g.Players[targetID]; ok {
			delete(g.Grid, p.Pos)
			delete(g.Players, targetID)
			fmt.Printf("🚫 Адмін вигнав: %s\n", p.Name)
		}

	} else if strings.HasPrefix(actionLower, "!baptize") {
		targetID := strings.TrimSpace(strings.TrimPrefix(actionStr, "!baptize"))
		if p, ok := g.Players[targetID]; ok {
			p.Status = 1
			p.HeadID = 1
			p.BodyID = 1
			g.DB.BaptizeUser(p.ID, "admin_blessing")
			g.DB.UpdateSkin(p.ID, 1, 1)
			p.LastMessage = "Деміург охрестив мене!"
			p.MessageTime = time.Now()
		}
	}
}

// ... findFreeSpawn та parseAction залишаються без змін ...
func (g *Game) findFreeSpawn() (Pos, bool) {
	for y := 1; y < config.MaxY-1; y++ {
		for x := 1; x < config.MaxX-1; x++ {
			p := Pos{X: x, Y: y}
			if g.Grid[p] == nil && !g.BlockedCells[p] {
				return p, true
			}
		}
	}
	return Pos{}, false
}

func parseAction(action string) (dx, dy, steps int) {
	if len(action) < 2 || action[0] != '!' {
		return 0, 0, 0
	}
	switch action[1] {
	case 'r':
		dx = 1
	case 'l':
		dx = -1
	case 'u':
		dy = 1
	case 'd':
		dy = -1
	default:
		return 0, 0, 0
	}
	steps = 1
	if len(action) > 2 {
		if s, err := strconv.Atoi(action[2:]); err == nil {
			steps = s
		}
	}
	if steps > config.MaxStepsPerTurn {
		steps = config.MaxStepsPerTurn
	}
	return dx, dy, steps
}
