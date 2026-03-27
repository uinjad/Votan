package engine

import (
	"fmt"
	"sync"
	"time"

	"Votan/internal/config"
	"Votan/internal/obs"
	"Votan/internal/storage"
)

type Game struct {
	mu           sync.RWMutex
	Players      map[string]*Player
	Grid         map[Pos]*Player
	BlockedCells map[Pos]bool
	CommandChan  chan Command
	DB           *storage.DB
	OBS          *obs.Client

	VoteActive      bool
	VoteTopic       string
	VoteOptionA     string
	VoteOptionB     string
	VoteEndTime     time.Time
	VoteResult      string
	VoteResultEnd   time.Time
	VoteSoundPlayed bool

	Attack5GActive  bool
	Attack5GZones   map[Pos]bool
	Attack5GEndTime time.Time

	BossActive bool
	BossHP     int
}

func NewGame(db *storage.DB, obsClient *obs.Client) *Game {
	g := &Game{
		Players:      make(map[string]*Player),
		Grid:         make(map[Pos]*Player),
		BlockedCells: make(map[Pos]bool),
		CommandChan:  make(chan Command, 1000),
		DB:           db,
		OBS:          obsClient,
	}
	g.initStaticMap()
	return g
}

func (g *Game) RestorePlayersFromDB() {
	users, err := g.DB.LoadAllUsers()
	if err != nil {
		fmt.Println("Помилка відновлення бази гравців:", err)
		return
	}
	for _, u := range users {
		pos := Pos{X: u.X, Y: u.Y}
		if pos.X >= 0 && pos.X < config.MaxX && pos.Y >= 0 && pos.Y < config.MaxY && g.Grid[pos] == nil && !g.BlockedCells[pos] {
			player := &Player{
				ID: u.ID, Name: u.Name, Pos: pos, LastActive: time.Now(),
				Status: u.Status, IsIrradiated: u.IsIrradiated,
				HeadID: u.HeadID, BodyID: u.BodyID,
			}
			g.Players[u.ID] = player
			g.Grid[pos] = player
		}
	}
	fmt.Printf("Відновлено %d слов'ян з бази даних\n", len(g.Players))
}

// Функція-помічник для масового блокування прямокутних зон
func (g *Game) blockArea(x1, y1, x2, y2 int) {
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			g.BlockedCells[Pos{X: x, Y: y}] = true
		}
	}
}

func (g *Game) initStaticMap() {
	// 1. Блокуємо краї екрану
	for x := 0; x < config.MaxX; x++ {
		g.BlockedCells[Pos{X: x, Y: 0}] = true
		g.BlockedCells[Pos{X: x, Y: config.MaxY - 1}] = true
	}
	for y := 0; y < config.MaxY; y++ {
		g.BlockedCells[Pos{X: 0, Y: y}] = true
		g.BlockedCells[Pos{X: config.MaxX - 1, Y: y}] = true
	}

	// 2. Відновлені перешкоди (Декорації)
	g.blockArea(9, 25, 10, 30)
	g.blockArea(16, 9, 17, 10)
	g.blockArea(15, 6, 16, 7)
}

func (g *Game) Start() {
	ticker := time.NewTicker(100 * time.Millisecond)
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

	threshold := time.Now().Add(-1 * config.PlayerTimeout)
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

	commandsToProcess := len(g.CommandChan)
	for i := 0; i < commandsToProcess; i++ {
		cmd := <-g.CommandChan
		g.processCommand(cmd)
	}

	g.processVoteEvent()
	g.process5GEvent()
	g.processDebuffs()

	// ЛОГІКА РУХУ ТА КОЛІЗІЇ
	for _, p := range g.Players {
		if p.RemainingSteps > 0 {
			nextPos := Pos{X: p.Pos.X + p.TargetDx, Y: p.Pos.Y + p.TargetDy}

			// Чиста перевірка: Межі, інші гравці, статичні перешкоди
			if nextPos.X < 0 || nextPos.X >= config.MaxX || nextPos.Y < 0 || nextPos.Y >= config.MaxY ||
				g.Grid[nextPos] != nil ||
				g.BlockedCells[nextPos] {

				p.RemainingSteps = 0
				continue
			}

			// Робимо крок
			delete(g.Grid, p.Pos)
			p.Pos = nextPos
			g.Grid[p.Pos] = p
			p.RemainingSteps--

			if g.DB != nil {
				g.DB.UpsertUser(p.ID, p.Name, p.Pos.X, p.Pos.Y)
			}
		}
	}
}

func (g *Game) processVoteEvent() {
	if g.VoteActive {
		timeLeft := time.Until(g.VoteEndTime)

		if timeLeft <= 5*time.Second && !g.VoteSoundPlayed {
			g.VoteSoundPlayed = true
			if g.OBS != nil {
				g.OBS.RestartMedia("viche")
			}
		}

		if time.Now().After(g.VoteEndTime) {
			scoreA, scoreB := g.calculateCurrentScores()
			g.VoteActive = false
			g.VoteSoundPlayed = false

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
	}

	if g.VoteResult != "" && time.Now().After(g.VoteResultEnd) {
		g.VoteResult = ""
	}
}

func (g *Game) calculateCurrentScores() (int, int) {
	scoreA, scoreB := 0, 0

	if !g.VoteActive {
		return 0, 0
	}

	midX := config.MaxX / 2
	for _, p := range g.Players {
		if p.Status != 1 {
			continue
		}
		if !p.Voted {
			continue
		}

		if p.Pos.X <= midX {
			scoreA += 1
		} else {
			scoreB += 1
		}
	}
	return scoreA, scoreB
}

func (g *Game) process5GEvent() {
	if g.Attack5GActive && time.Now().After(g.Attack5GEndTime) {
		g.Attack5GActive = false
		for _, p := range g.Players {
			if g.Attack5GZones[p.Pos] {
				p.IsIrradiated = true
				p.IrradiatedUntil = time.Now().Add(config.Debuff5GDuration)
				if g.DB != nil {
					g.DB.SetIrradiated(p.ID, true)
				}
			}
		}
		g.Attack5GZones = nil
	}
}

func (g *Game) processDebuffs() {
	now := time.Now()
	for _, p := range g.Players {
		if p.IsIrradiated && now.After(p.IrradiatedUntil) {
			p.IsIrradiated = false
			if g.DB != nil {
				g.DB.SetIrradiated(p.ID, false)
			}
		}
	}
}

func (g *Game) GetState() GameState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	scoreA, scoreB := g.calculateCurrentScores()

	state := GameState{
		Players:        make([]PlayerState, 0, len(g.Players)),
		VoteActive:     g.VoteActive,
		VoteTopic:      g.VoteTopic,
		VoteOptionA:    g.VoteOptionA,
		VoteOptionB:    g.VoteOptionB,
		VoteScoreA:     scoreA,
		VoteScoreB:     scoreB,
		VoteResult:     g.VoteResult,
		Attack5GActive: g.Attack5GActive,
		BossActive:     g.BossActive,
		BossHP:         g.BossHP,
		BossMaxHP:      config.BossMaxHP,
	}

	if g.VoteActive {
		state.VoteTimeLeft = int(time.Until(g.VoteEndTime).Seconds())
	}

	if g.Attack5GActive {
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
			ID: p.ID, Name: p.Name, X: p.Pos.X, Y: p.Pos.Y,
			Status: p.Status, IsIrradiated: p.IsIrradiated,
			HeadID: p.HeadID, BodyID: p.BodyID, Message: msg,
			Voted: p.Voted,
		})
	}
	return state
}
