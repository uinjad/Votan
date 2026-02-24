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

// Константи для OBS
const (
	OBSSceneName      = "Main"
	OBSWebcamSource   = "camera"
	OBSSubscribeMovie = "subscribeMovie"
	OBSSubscribeSong  = "subscribeSong"
)

func (g *Game) processCommand(cmd Command) {
	actionStr := strings.TrimSpace(cmd.Action)
	actionLower := strings.ToLower(actionStr)

	// 👑 АДМІНКА (Люцифер)
	if cmd.PlayerID == config.AdminSecret {
		g.handleAdminCommand(actionStr, actionLower)
		return
	}

	// ОБРОБКА ГРАВЦІВ
	player, exists := g.Players[cmd.PlayerID]
	if !exists {
		spawnPos, ok := g.findFreeSpawn() // ТА САМА ПОМИЛКА БУЛА ТУТ
		if !ok {
			return
		}
		player = &Player{
			ID:         cmd.PlayerID,
			Name:       strings.TrimPrefix(cmd.PlayerName, "@"),
			Pos:        spawnPos,
			LastActive: time.Now(),
		}
		g.Players[cmd.PlayerID] = player
		g.Grid[spawnPos] = player
		g.DB.UpsertUser(player.ID, player.Name, player.Pos.X, player.Pos.Y)
	}

	player.LastActive = time.Now()

	// ☢️ ДЕБАФ ОПРОМІНЕННЯ 5G
	if player.IsIrradiated && actionStr != "" {
		phrases := []string{
			"Хочу ревакцинуватись!",
			"Піду шукати роботу в офісі!",
			"Слава Ящерам!",
			"5G - це здоров'я!",
		}
		player.LastMessage = phrases[rand.Intn(len(phrases))]
		player.MessageTime = time.Now()
		player.RemainingSteps = 0
		return
	}

	if actionStr != "" {
		isCommand := false

		// ⚔️ УДАР ПО БОСУ
		if actionLower == "!hit" && g.BossActive {
			g.handleBossDamage()
			isCommand = true

			// 🧬 ЗМІНА СКІНУ (тільки для R1A1a)
		} else if matches := skinRegex.FindStringSubmatch(actionLower); matches != nil {
			g.handleSkinChange(player, matches)
			isCommand = true

			// 🏃 РУХ (!r5, !l2 і т.д.)
		} else if strings.HasPrefix(actionLower, "!") {
			dx, dy, steps := parseAction(actionLower)
			if steps > 0 {
				player.TargetDx = dx
				player.TargetDy = dy
				player.RemainingSteps = steps
				isCommand = true
			}
		}

		// Звичайне повідомлення в чат
		if !isCommand && !strings.HasPrefix(actionStr, "!") {
			player.LastMessage = actionStr
			player.MessageTime = time.Now()
		}
	}
}

func (g *Game) handleBossDamage() {
	g.BossHP -= config.BossHitDamage
	if g.BossHP <= 0 {
		g.BossHP = 0
		g.BossActive = false
		if g.OBS != nil {
			go g.triggerVictoryMedia()
		}
		fmt.Println("⚔️ Ящера повалено силами русичів!")
	}
}

func (g *Game) triggerVictoryMedia() {
	g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeMovie, true)
	g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeSong, true)
	g.OBS.RestartMedia(OBSSubscribeSong)
	g.OBS.SetOpacity(OBSWebcamSource, "Fade", 1.0)
	time.Sleep(20 * time.Second)
	g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeMovie, false)
}

func (g *Game) handleSkinChange(p *Player, matches []string) {
	if p.Status == 1 {
		h, _ := strconv.Atoi(matches[1])
		b, _ := strconv.Atoi(matches[2])
		if h >= 0 && h <= config.MaxHeadID && b >= 0 && b <= config.MaxBodyID {
			p.HeadID = h
			p.BodyID = b
			g.DB.UpdateSkin(p.ID, h, b)
		}
	} else {
		p.LastMessage = "Потрібні гени R1A1a!"
		p.MessageTime = time.Now()
	}
}

func (g *Game) handleAdminCommand(actionStr, actionLower string) {
	switch {
	case strings.HasPrefix(actionLower, "!віче"):
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(actionStr, "!віче")), "|")
		g.VoteActive = true
		g.VoteTopic = "ВИБІР ДОЛІ"
		if len(parts) > 0 && parts[0] != "" {
			g.VoteTopic = parts[0]
		}
		if len(parts) > 1 {
			g.VoteOptionA = parts[1]
		} else {
			g.VoteOptionA = "ЗА"
		}
		if len(parts) > 2 {
			g.VoteOptionB = parts[2]
		} else {
			g.VoteOptionB = "ПРОТИ"
		}
		g.VoteEndTime = time.Now().Add(config.VoteDuration)

	case actionLower == "!stop_vote":
		if g.VoteActive {
			g.VoteEndTime = time.Now().Add(-1 * time.Second)
		}

	case actionLower == "!5g":
		g.start5GAttack()

	case actionLower == "!ящер":
		g.BossActive = true
		g.BossHP = config.BossMaxHP
		if g.OBS != nil {
			go g.OBS.FadeSourceOpacity(OBSWebcamSource, "Fade", 1.0, 0.0, 20*time.Second)
		}

	case actionLower == "!kill_boss":
		g.BossActive = false
		g.BossHP = 0
		if g.OBS != nil {
			go g.triggerVictoryMedia()
		}

	case actionLower == "!fix_obs":
		if g.OBS != nil {
			g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeMovie, false)
			g.OBS.SetSourceEnabled(OBSSceneName, OBSSubscribeSong, false)
			g.OBS.SetOpacity(OBSWebcamSource, "Fade", 1.0)
		}

	case strings.HasPrefix(actionLower, "!kick"):
		id := strings.TrimSpace(strings.TrimPrefix(actionStr, "!kick"))
		if p, ok := g.Players[id]; ok {
			delete(g.Grid, p.Pos)
			delete(g.Players, id)
		}

	case strings.HasPrefix(actionLower, "!baptize"):
		id := strings.TrimSpace(strings.TrimPrefix(actionStr, "!baptize"))
		if p, ok := g.Players[id]; ok {
			p.Status = 1
			p.HeadID, p.BodyID = 1, 1
			g.DB.BaptizeUser(p.ID, "lucifer_blessing")
			g.DB.UpdateSkin(p.ID, 1, 1)
			p.LastMessage = "в мене гени R1A1a"
			p.MessageTime = time.Now()
		}

	case strings.HasPrefix(actionLower, "!purge"):
		id := strings.TrimSpace(strings.TrimPrefix(actionStr, "!purge"))
		if p, ok := g.Players[id]; ok {
			delete(g.Grid, p.Pos)
			delete(g.Players, id)
		}
		g.DB.DeleteUser(id)
		fmt.Printf("💀 Гравець %s стертий з історії\n", id)
	}
}

func (g *Game) start5GAttack() {
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
}

// ПРИЧИНА ПОМИЛКИ БУЛА В ТОМУ, ЩО ЦЬОГО МЕТОДУ НЕ БУЛО В ФАЙЛІ
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
