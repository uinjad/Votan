package engine

import (
	"crypto/subtle"
	"log/slog"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Votan/internal/config"
)

var skinRegex = regexp.MustCompile(`(?i)^!h(\d+)b(\d+)$`)

const (
	obsSceneName      = "Main"
	obsWebcamSource   = "camera"
	obsSubscribeMovie = "subscribeMovie"
	obsSubscribeSong  = "subscribeSong"
)

// isAdmin compares in constant time so the secret can't be recovered via
// timing, and never authenticates against an empty secret.
func (g *Game) isAdmin(playerID string) bool {
	if g.cfg.AdminSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(playerID), []byte(g.cfg.AdminSecret)) == 1
}

// processCommand must be called with g.mu held (it runs from tick).
func (g *Game) processCommand(cmd Command) {
	action := strings.TrimSpace(cmd.Action)
	lower := strings.ToLower(action)

	if g.isAdmin(cmd.PlayerID) {
		g.handleAdminCommand(action, lower)
		return
	}

	player, ok := g.players[cmd.PlayerID]
	if !ok {
		player = g.spawnPlayer(cmd)
		if player == nil {
			return // board full, no free cell
		}
	}
	player.LastActive = time.Now()

	// Irradiated players can't move; they babble instead.
	if player.IsIrradiated && action != "" {
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

	if action == "" {
		return
	}

	skinMatch := skinRegex.FindStringSubmatch(lower)
	switch {
	case lower == "!hit" && g.bossActive:
		g.handleBossDamage()
	case skinMatch != nil:
		g.handleSkinChange(player, skinMatch)
	case strings.HasPrefix(lower, "!"):
		if dx, dy, steps := parseAction(lower); steps > 0 {
			player.TargetDx, player.TargetDy, player.RemainingSteps = dx, dy, steps
			if g.voteActive {
				player.Voted = true
			}
		}
	default:
		player.LastMessage = action
		player.MessageTime = time.Now()
	}
}

func (g *Game) spawnPlayer(cmd Command) *Player {
	pos, ok := g.findFreeSpawn()
	if !ok {
		return nil
	}
	p := &Player{
		ID:         cmd.PlayerID,
		Name:       strings.TrimPrefix(cmd.PlayerName, "@"),
		Pos:        pos,
		LastActive: time.Now(),
	}
	// Restore persisted status/skins from the in-memory profile cache (no DB
	// access on the hot path).
	if prof, ok := g.profiles[cmd.PlayerID]; ok {
		p.Status = prof.Status
		p.IsIrradiated = prof.IsIrradiated
		p.HeadID = prof.HeadID
		p.BodyID = prof.BodyID
	}
	g.players[cmd.PlayerID] = p
	g.grid[pos] = p
	g.store.UpsertUser(p.ID, p.Name, p.Pos.X, p.Pos.Y)
	return p
}

func (g *Game) handleBossDamage() {
	g.bossHP -= config.BossHitDamage
	if g.bossHP <= 0 {
		g.bossHP = 0
		g.bossActive = false
		go g.triggerVictoryMedia()
		slog.Info("engine: boss defeated by chat")
	}
}

// triggerVictoryMedia runs in its own goroutine and only touches g.scene
// (immutable after construction), so it needs no lock.
func (g *Game) triggerVictoryMedia() {
	g.scene.SetSourceEnabled(obsSceneName, obsSubscribeMovie, true)
	g.scene.SetSourceEnabled(obsSceneName, obsSubscribeSong, true)
	g.scene.RestartMedia(obsSubscribeSong)
	g.scene.SetOpacity(obsWebcamSource, "Fade", 1.0)
	time.Sleep(30 * time.Second)
	g.scene.SetSourceEnabled(obsSceneName, obsSubscribeMovie, false)
}

func (g *Game) handleSkinChange(p *Player, m []string) {
	if p.Status != 1 {
		p.LastMessage = "Потрібні гени R1A1a!"
		p.MessageTime = time.Now()
		return
	}
	h, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	if h >= 0 && h <= g.cfg.MaxHeadID && b >= 0 && b <= g.cfg.MaxBodyID {
		p.HeadID, p.BodyID = h, b
		g.store.UpdateSkin(p.ID, h, b)
	}
}

// handleAdminCommand dispatches admin verbs. Prefix order matters: the longer,
// more specific prefixes (!kick_unbaptized, !purge_unbaptized) MUST be matched
// before their shorter forms (!kick, !purge).
func (g *Game) handleAdminCommand(action, lower string) {
	switch {
	case strings.HasPrefix(lower, "!віче"):
		g.startVote(action)

	case lower == "!stop_vote":
		if g.voteActive {
			g.voteEndTime = time.Now().Add(-time.Second)
		}

	case lower == "!5g":
		g.start5GAttack()

	case lower == "!ящер":
		g.bossActive = true
		g.bossHP = config.BossMaxHP
		go g.scene.FadeSourceOpacity(obsWebcamSource, "Fade", 1.0, 0.0, 20*time.Second)

	case lower == "!kill_boss":
		g.bossActive = false
		g.bossHP = 0
		go g.triggerVictoryMedia()

	case lower == "!fix_obs":
		g.scene.SetSourceEnabled(obsSceneName, obsSubscribeMovie, false)
		g.scene.SetSourceEnabled(obsSceneName, obsSubscribeSong, false)
		g.scene.SetOpacity(obsWebcamSource, "Fade", 1.0)

	case strings.HasPrefix(lower, "!rename"):
		g.renamePlayer(action)

	case strings.HasPrefix(lower, "!kick_unbaptized"):
		n := 0
		for id, p := range g.players {
			if p.Status != 1 {
				delete(g.grid, p.Pos)
				delete(g.players, id)
				n++
			}
		}
		slog.Info("engine: kicked unbaptized", "count", n)

	case strings.HasPrefix(lower, "!kick"):
		id := strings.TrimSpace(strings.TrimPrefix(action, "!kick"))
		g.removePlayer(id)

	case strings.HasPrefix(lower, "!baptize"):
		id := strings.TrimSpace(strings.TrimPrefix(action, "!baptize"))
		if p, ok := g.players[id]; ok {
			p.Status = 1
			p.HeadID, p.BodyID = 1, 1
			g.store.Baptize(p.ID)
			g.store.UpdateSkin(p.ID, 1, 1)
			p.LastMessage = "в мене гени R1A1a"
			p.MessageTime = time.Now()
		}

	case strings.HasPrefix(lower, "!purge_unbaptized"):
		fromMap, fromStore := 0, 0
		for id, p := range g.players {
			if p.Status != 1 {
				delete(g.grid, p.Pos)
				delete(g.players, id)
				fromMap++
			}
		}
		for id, prof := range g.profiles {
			if prof.Status != 1 {
				g.store.DeleteUser(id)
				delete(g.profiles, id)
				fromStore++
			}
		}
		slog.Info("engine: purged unbaptized", "from_map", fromMap, "from_store", fromStore)

	case strings.HasPrefix(lower, "!purge"):
		id := strings.TrimSpace(strings.TrimPrefix(action, "!purge"))
		g.removePlayer(id)
		g.store.DeleteUser(id)
		delete(g.profiles, id)
		slog.Info("engine: player purged", "id", id)
	}
}

func (g *Game) startVote(action string) {
	rest := strings.TrimSpace(strings.TrimPrefix(action, "!віче"))
	parts := strings.Split(rest, "|")

	g.voteActive = true
	for _, p := range g.players {
		p.Voted = false
	}

	g.voteTopic = "ВИБІР ДОЛІ"
	if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
		g.voteTopic = strings.TrimSpace(parts[0])
	}
	g.voteOptionA = "ЗА"
	if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
		g.voteOptionA = strings.TrimSpace(parts[1])
	}
	g.voteOptionB = "ПРОТИ"
	if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
		g.voteOptionB = strings.TrimSpace(parts[2])
	}
	g.voteEndTime = time.Now().Add(config.VoteDuration)
	slog.Info("engine: vote started", "topic", g.voteTopic)
}

func (g *Game) start5GAttack() {
	g.attack5GActive = true
	g.attack5GEndTime = time.Now().Add(config.Attack5GDuration)
	g.attack5GZones = make(map[Pos]bool)
	for i := 0; i < 4; i++ {
		cx := rand.Intn(config.MaxX-4) + 2
		cy := rand.Intn(config.MaxY-4) + 2
		for dx := -2; dx <= 2; dx++ {
			for dy := -2; dy <= 2; dy++ {
				g.attack5GZones[Pos{X: cx + dx, Y: cy + dy}] = true
			}
		}
	}
	slog.Info("engine: 5G attack started")
}

func (g *Game) renamePlayer(action string) {
	rest := strings.TrimSpace(strings.TrimPrefix(action, "!rename"))
	id, name, ok := strings.Cut(rest, "|")
	if !ok {
		return
	}
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if p, ok := g.players[id]; ok {
		p.Name = name
		g.store.UpsertUser(p.ID, p.Name, p.Pos.X, p.Pos.Y)
		slog.Info("engine: player renamed", "id", id, "name", name)
	}
}

func (g *Game) removePlayer(id string) {
	if p, ok := g.players[id]; ok {
		delete(g.grid, p.Pos)
		delete(g.players, id)
	}
}

func (g *Game) findFreeSpawn() (Pos, bool) {
	for y := 1; y < config.MaxY-1; y++ {
		for x := 1; x < config.MaxX-1; x++ {
			p := Pos{X: x, Y: y}
			if g.grid[p] == nil && !g.blockedCells[p] {
				return p, true
			}
		}
	}
	return Pos{}, false
}

// parseAction parses a movement command like "!r5". A non-positive or
// unparseable step count yields steps == 0, which the caller ignores. This is
// the fix for the old "!r-5" trap that produced negative deltas.
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
		n, err := strconv.Atoi(action[2:])
		if err != nil || n <= 0 {
			return 0, 0, 0 // reject garbage and non-positive counts
		}
		steps = n
	}
	if steps > config.MaxStepsPerTurn {
		steps = config.MaxStepsPerTurn
	}
	return dx, dy, steps
}
