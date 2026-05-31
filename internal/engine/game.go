package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"Votan/internal/config"
)

// Config is the runtime configuration the engine needs. It is injected at
// construction; the engine holds no global state.
type Config struct {
	AdminSecret string
	MaxHeadID   int
	MaxBodyID   int
}

// Game owns all game state. State is mutated only from the Run loop and read
// under the RWMutex, so every field below is guarded by mu.
type Game struct {
	cfg   Config
	store Store
	scene Scene

	mu           sync.RWMutex
	players      map[string]*Player
	grid         map[Pos]*Player
	blockedCells map[Pos]bool

	// profiles is the persisted projection of every known user, loaded once at
	// startup so first-touch restores never hit the database on the hot path.
	profiles map[string]UserRecord

	commands chan Command

	// vote state
	voteActive      bool
	voteTopic       string
	voteOptionA     string
	voteOptionB     string
	voteEndTime     time.Time
	voteResult      string
	voteResultEnd   time.Time
	voteSoundPlayed bool

	// 5G attack state
	attack5GActive  bool
	attack5GZones   map[Pos]bool
	attack5GEndTime time.Time

	// boss state
	bossActive bool
	bossHP     int
}

const commandBuffer = 1024

// NewGame builds a Game. A nil store or scene is normalised to a no-op
// implementation, so callers (and tests) need no nil checks downstream.
func NewGame(store Store, scene Scene, cfg Config) *Game {
	if store == nil {
		store = NopStore{}
	}
	if scene == nil {
		scene = NopScene{}
	}
	g := &Game{
		cfg:          cfg,
		store:        store,
		scene:        scene,
		players:      make(map[string]*Player),
		grid:         make(map[Pos]*Player),
		blockedCells: make(map[Pos]bool),
		profiles:     make(map[string]UserRecord),
		commands:     make(chan Command, commandBuffer),
	}
	g.initStaticMap()
	return g
}

// Commands returns the send-only channel used to feed chat and admin commands
// into the game loop.
func (g *Game) Commands() chan<- Command { return g.commands }

// RestorePlayers loads all persisted users once at startup: every user is
// cached in profiles (for first-touch restore of status/skins), and those
// whose saved position is still valid and free are placed back on the grid, so
// a restart preserves the board.
func (g *Game) RestorePlayers(ctx context.Context) error {
	users, err := g.store.LoadAllUsers(ctx)
	if err != nil {
		return fmt.Errorf("engine: restore players: %w", err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	placed := 0
	for _, u := range users {
		g.profiles[u.ID] = u
		pos := Pos{X: u.X, Y: u.Y}
		if g.cellFree(pos) {
			p := &Player{
				ID: u.ID, Name: u.Name, Pos: pos, LastActive: time.Now(),
				Status: u.Status, IsIrradiated: u.IsIrradiated,
				HeadID: u.HeadID, BodyID: u.BodyID,
			}
			g.players[u.ID] = p
			g.grid[pos] = p
			placed++
		}
	}
	slog.Info("engine: players restored", "cached", len(g.profiles), "placed", placed)
	return nil
}

// Run owns all game state. It blocks until ctx is cancelled, then stops its
// tickers and returns, letting the caller shut down cleanly.
func (g *Game) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	cleanup := time.NewTicker(1 * time.Minute)
	defer cleanup.Stop()

	slog.Info("engine: game loop started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("engine: game loop stopped")
			return
		case <-ticker.C:
			g.tick()
		case <-cleanup.C:
			g.cleanupInactive()
		}
	}
}

// cellFree reports whether pos is in bounds, unblocked and unoccupied.
func (g *Game) cellFree(pos Pos) bool {
	return pos.X >= 0 && pos.X < config.MaxX &&
		pos.Y >= 0 && pos.Y < config.MaxY &&
		g.grid[pos] == nil && !g.blockedCells[pos]
}

func (g *Game) blockArea(x1, y1, x2, y2 int) {
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			g.blockedCells[Pos{X: x, Y: y}] = true
		}
	}
}

func (g *Game) initStaticMap() {
	for x := 0; x < config.MaxX; x++ {
		g.blockedCells[Pos{X: x, Y: 0}] = true
		g.blockedCells[Pos{X: x, Y: config.MaxY - 1}] = true
	}
	for y := 0; y < config.MaxY; y++ {
		g.blockedCells[Pos{X: 0, Y: y}] = true
		g.blockedCells[Pos{X: config.MaxX - 1, Y: y}] = true
	}
	// Static obstacles (decor).
	g.blockArea(9, 25, 10, 30)
	g.blockArea(16, 9, 17, 10)
	g.blockArea(15, 6, 16, 7)
}

func (g *Game) cleanupInactive() {
	g.mu.Lock()
	defer g.mu.Unlock()

	threshold := time.Now().Add(-config.PlayerTimeout)
	for id, p := range g.players {
		if p.LastActive.Before(threshold) {
			delete(g.grid, p.Pos)
			delete(g.players, id)
		}
	}
}

func (g *Game) tick() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Drain a snapshot of the command buffer. len() is a point-in-time reading
	// and only this goroutine receives, so the receives never block.
	for n := len(g.commands); n > 0; n-- {
		g.processCommand(<-g.commands)
	}

	g.processVoteEvent()
	g.process5GEvent()
	g.processDebuffs()
	g.movePlayers()
}

func (g *Game) movePlayers() {
	for _, p := range g.players {
		if p.RemainingSteps <= 0 {
			continue
		}
		next := Pos{X: p.Pos.X + p.TargetDx, Y: p.Pos.Y + p.TargetDy}
		if !g.cellFree(next) {
			p.RemainingSteps = 0
			continue
		}
		delete(g.grid, p.Pos)
		p.Pos = next
		g.grid[next] = p
		p.RemainingSteps--

		// Async, non-blocking: never stalls the tick on disk I/O.
		g.store.UpsertUser(p.ID, p.Name, p.Pos.X, p.Pos.Y)
	}
}

func (g *Game) processVoteEvent() {
	if g.voteActive {
		if time.Until(g.voteEndTime) <= 5*time.Second && !g.voteSoundPlayed {
			g.voteSoundPlayed = true
			g.scene.RestartMedia("viche")
		}
		if time.Now().After(g.voteEndTime) {
			scoreA, scoreB := g.tallyVotes()
			g.voteActive = false
			g.voteSoundPlayed = false

			winner := "НІЧИЯ"
			switch {
			case scoreA > scoreB:
				winner = g.voteOptionA
			case scoreB > scoreA:
				winner = g.voteOptionB
			}
			g.voteResult = fmt.Sprintf("РІШЕННЯ: %s (Рахунок %d:%d)", winner, scoreA, scoreB)
			g.voteResultEnd = time.Now().Add(config.VoteResultTTL)
		}
	}
	if g.voteResult != "" && time.Now().After(g.voteResultEnd) {
		g.voteResult = ""
	}
}

func (g *Game) tallyVotes() (scoreA, scoreB int) {
	if !g.voteActive {
		return 0, 0
	}
	midX := config.MaxX / 2
	for _, p := range g.players {
		if p.Status != 1 || !p.Voted {
			continue
		}
		if p.Pos.X <= midX {
			scoreA++
		} else {
			scoreB++
		}
	}
	return scoreA, scoreB
}

func (g *Game) process5GEvent() {
	if g.attack5GActive && time.Now().After(g.attack5GEndTime) {
		g.attack5GActive = false
		for _, p := range g.players {
			if g.attack5GZones[p.Pos] {
				p.IsIrradiated = true
				p.IrradiatedUntil = time.Now().Add(config.Debuff5GDuration)
				g.store.SetIrradiated(p.ID, true)
			}
		}
		g.attack5GZones = nil
	}
}

func (g *Game) processDebuffs() {
	now := time.Now()
	for _, p := range g.players {
		if p.IsIrradiated && now.After(p.IrradiatedUntil) {
			p.IsIrradiated = false
			g.store.SetIrradiated(p.ID, false)
		}
	}
}

// GetState returns a snapshot of the wire model. Safe for concurrent callers.
func (g *Game) GetState() GameState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	scoreA, scoreB := g.tallyVotes()
	state := GameState{
		Players:        make([]PlayerState, 0, len(g.players)),
		VoteActive:     g.voteActive,
		VoteTopic:      g.voteTopic,
		VoteOptionA:    g.voteOptionA,
		VoteOptionB:    g.voteOptionB,
		VoteScoreA:     scoreA,
		VoteScoreB:     scoreB,
		VoteResult:     g.voteResult,
		Attack5GActive: g.attack5GActive,
		BossActive:     g.bossActive,
		BossHP:         g.bossHP,
		BossMaxHP:      config.BossMaxHP,
	}
	if g.voteActive {
		state.VoteTimeLeft = int(time.Until(g.voteEndTime).Seconds())
	}
	if g.attack5GActive {
		for z := range g.attack5GZones {
			state.Attack5GZones = append(state.Attack5GZones, z)
		}
	}
	for _, p := range g.players {
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
