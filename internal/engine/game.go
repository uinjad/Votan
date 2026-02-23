package engine

import (
	"fmt"
	"sync"
	"time"

	"Votan/internal/config"
	"Votan/internal/storage"
)

type Game struct {
	mu           sync.RWMutex
	Players      map[string]*Player
	Grid         map[Pos]*Player
	BlockedCells map[Pos]bool
	CommandChan  chan Command
	DB           *storage.DB

	// Івент "Віче"
	VoteActive    bool
	VoteTopic     string
	VoteOptionA   string
	VoteOptionB   string
	VoteEndTime   time.Time
	VoteResult    string
	VoteResultEnd time.Time

	// Івент "5G Атака"
	Attack5GActive  bool
	Attack5GZones   map[Pos]bool
	Attack5GEndTime time.Time

	// Битва з Ящером
	BossActive bool
	BossHP     int
}

func NewGame(db *storage.DB) *Game {
	g := &Game{
		Players:      make(map[string]*Player),
		Grid:         make(map[Pos]*Player),
		BlockedCells: make(map[Pos]bool),
		CommandChan:  make(chan Command, 1000),
		DB:           db,
	}

	g.initStaticMap()
	g.loadPlayersFromDB()
	return g
}

func (g *Game) initStaticMap() {
	for x := 0; x < config.MaxX; x++ {
		g.BlockedCells[Pos{X: x, Y: 0}] = true
		g.BlockedCells[Pos{X: x, Y: config.MaxY - 1}] = true
	}
	for y := 0; y < config.MaxY; y++ {
		g.BlockedCells[Pos{X: 0, Y: y}] = true
		g.BlockedCells[Pos{X: config.MaxX - 1, Y: y}] = true
	}
	g.blockArea(9, 25, 10, 30)
	g.blockArea(16, 9, 17, 10)
	g.blockArea(15, 6, 16, 7)
}

func (g *Game) blockArea(x1, y1, x2, y2 int) {
	minX, maxX := min(x1, x2), max(x1, x2)
	minY, maxY := min(y1, y2), max(y1, y2)
	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			g.BlockedCells[Pos{X: x, Y: y}] = true
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (g *Game) loadPlayersFromDB() {
	savedUsers, _ := g.DB.LoadAllUsers()
	for _, u := range savedUsers {
		pos := Pos{X: u.X, Y: u.Y}
		if pos.X >= 0 && pos.X < config.MaxX && pos.Y >= 0 && pos.Y < config.MaxY && g.Grid[pos] == nil && !g.BlockedCells[pos] {
			player := &Player{
				ID:           u.ID,
				Name:         u.Name,
				Pos:          pos,
				LastActive:   time.Now(),
				Status:       u.Status,
				IsIrradiated: u.IsIrradiated,
				HeadID:       u.HeadID,
				BodyID:       u.BodyID,
			}
			g.Players[u.ID] = player
			g.Grid[pos] = player
		}
	}
}

func (g *Game) Start() {
	ticker := time.NewTicker(config.TickRate)
	cleanupTicker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-ticker.C:
			g.tick()
		case <-cleanupTicker.C:
			g.cleanupInactive()
		}
	}
}

func (g *Game) cleanupInactive() {
	g.mu.Lock()
	defer g.mu.Unlock()
	threshold := time.Now().Add(config.PlayerTimeout)
	for id, p := range g.Players {
		if p.LastActive.Before(threshold) {
			delete(g.Grid, p.Pos)
			delete(g.Players, id)
		}
	}
}

func (g *Game) tick() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 1. Команди
	commandsToProcess := len(g.CommandChan)
	for i := 0; i < commandsToProcess; i++ {
		cmd := <-g.CommandChan
		g.processCommand(cmd)
	}

	// 2. Івенти
	g.processVoteEvent()
	g.process5GEvent()

	// 3. Стан гравців
	for _, p := range g.Players {
		if p.IsIrradiated && time.Now().After(p.IrradiatedUntil) {
			p.IsIrradiated = false
			g.DB.SetIrradiated(p.ID, false)
		}

		if p.RemainingSteps > 0 {
			nextPos := Pos{X: p.Pos.X + p.TargetDx, Y: p.Pos.Y + p.TargetDy}
			if nextPos.X < 0 || nextPos.X >= config.MaxX || nextPos.Y < 0 || nextPos.Y >= config.MaxY {
				p.RemainingSteps = 0
				continue
			}
			if g.Grid[nextPos] != nil || g.BlockedCells[nextPos] {
				p.RemainingSteps = 0
				continue
			}
			delete(g.Grid, p.Pos)
			p.Pos = nextPos
			g.Grid[p.Pos] = p
			p.RemainingSteps--
			g.DB.UpsertUser(p.ID, p.Name, p.Pos.X, p.Pos.Y)
		}
	}
}

func (g *Game) processVoteEvent() {
	if g.VoteActive && time.Now().After(g.VoteEndTime) {
		g.VoteActive = false
		scoreA, scoreB := 0, 0
		for _, p := range g.Players {
			weight := 1
			if p.Status == 1 {
				weight = 3
			}
			if p.Pos.X <= 9 {
				scoreA += weight
			} else {
				scoreB += weight
			}
		}
		var winner string
		if scoreA > scoreB {
			winner = g.VoteOptionA
		} else if scoreB > scoreA {
			winner = g.VoteOptionB
		} else {
			winner = "НІЧИЯ"
		}
		g.VoteResult = fmt.Sprintf("РІШЕННЯ: %s (Рахунок %d:%d)", winner, scoreA, scoreB)
		g.VoteResultEnd = time.Now().Add(config.VoteResultTTL)
	}
	if g.VoteResult != "" && time.Now().After(g.VoteResultEnd) {
		g.VoteResult = ""
	}
}

func (g *Game) process5GEvent() {
	if g.Attack5GActive && time.Now().After(g.Attack5GEndTime) {
		g.Attack5GActive = false
		for _, p := range g.Players {
			if g.Attack5GZones[p.Pos] {
				p.IsIrradiated = true
				p.IrradiatedUntil = time.Now().Add(config.Debuff5GDuration)
				g.DB.SetIrradiated(p.ID, true)
			}
		}
		g.Attack5GZones = nil
	}
}

func (g *Game) GetState() GameState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	state := GameState{
		Players:        make([]PlayerState, 0, len(g.Players)),
		VoteActive:     g.VoteActive,
		VoteTopic:      g.VoteTopic,
		VoteOptionA:    g.VoteOptionA,
		VoteOptionB:    g.VoteOptionB,
		VoteResult:     g.VoteResult,
		Attack5GActive: g.Attack5GActive,
		BossActive:     g.BossActive,
		BossHP:         g.BossHP,
		BossMaxHP:      config.BossMaxHP,
	}

	if g.VoteActive {
		timeLeft := int(time.Until(g.VoteEndTime).Seconds())
		if timeLeft < 0 {
			timeLeft = 0
		}
		state.VoteTimeLeft = timeLeft
	}

	if g.Attack5GActive {
		timeLeft := int(time.Until(g.Attack5GEndTime).Seconds())
		if timeLeft < 0 {
			timeLeft = 0
		}
		state.Attack5GTimeLeft = timeLeft
		for z := range g.Attack5GZones {
			state.Attack5GZones = append(state.Attack5GZones, z)
		}
	}

	for _, p := range g.Players {
		msg := ""
		if time.Since(p.MessageTime) < config.ChatBubbleTTL {
			msg = p.LastMessage
		}
		state.Players = append(state.Players, PlayerState{
			ID:           p.ID,
			Name:         p.Name,
			X:            p.Pos.X,
			Y:            p.Pos.Y,
			Status:       p.Status,
			IsIrradiated: p.IsIrradiated,
			HeadID:       p.HeadID,
			BodyID:       p.BodyID,
			Message:      msg,
		})
	}
	return state
}
