package engine

import (
	"testing"
	"time"

	"Votan/internal/config"
)

// setupTestGame returns a clean, headless game (no DB, no OBS).
func setupTestGame() *Game {
	return NewGame(nil, nil)
}

// TEST 1: movement and collisions.
func TestMovementAndCollisions(t *testing.T) {
	g := setupTestGame()

	p1 := &Player{
		ID: "p1", Name: "Ivan",
		Pos: Pos{X: 10, Y: 10},
	}
	g.Players["p1"] = p1
	g.Grid[p1.Pos] = p1

	// 1.1: successful move.
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos.X != 11 || p1.Pos.Y != 10 {
		t.Errorf("player did not move to the expected cell. current pos: %v", p1.Pos)
	}

	// 1.2: collision with the map edge (wall).
	p1.Pos = Pos{X: config.MaxX - 1, Y: 10}
	g.Grid[p1.Pos] = p1
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos.X >= config.MaxX {
		t.Errorf("player went out of bounds. pos: %v", p1.Pos)
	}
	if p1.RemainingSteps != 0 {
		t.Errorf("steps were not reset after hitting a wall")
	}

	// 1.3: collision with another player.
	p2 := &Player{ID: "p2", Pos: Pos{X: 10, Y: 10}}
	g.Players["p2"] = p2
	g.Grid[p2.Pos] = p2

	p1.Pos = Pos{X: 9, Y: 10}
	g.Grid[p1.Pos] = p1
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos.X == 10 {
		t.Errorf("player p1 stepped onto player p2")
	}

	// 1.4: collision with a static obstacle.
	obstaclePos := Pos{X: 15, Y: 15}
	g.BlockedCells[obstaclePos] = true
	p1.Pos = Pos{X: 14, Y: 15}
	g.Grid[p1.Pos] = p1
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos == obstaclePos {
		t.Errorf("player walked through a static obstacle")
	}
}

// TEST 2: voting logic.
func TestVotingSystem(t *testing.T) {
	g := setupTestGame()
	g.VoteActive = true

	// Unbaptized (gray): should not count.
	p1 := &Player{ID: "p1", Status: 0, Voted: true, Pos: Pos{X: 5, Y: 10}}
	g.Players["p1"] = p1

	// Baptized, did not move: should not count.
	p2 := &Player{ID: "p2", Status: 1, Voted: false, Pos: Pos{X: 5, Y: 11}}
	g.Players["p2"] = p2

	// Baptized, moved to the "for" side.
	p3 := &Player{ID: "p3", Status: 1, Voted: true, Pos: Pos{X: 5, Y: 12}}
	g.Players["p3"] = p3

	// Baptized, moved to the "against" side.
	p4 := &Player{ID: "p4", Status: 1, Voted: true, Pos: Pos{X: 15, Y: 12}}
	g.Players["p4"] = p4

	scoreA, scoreB := g.calculateCurrentScores()

	if scoreA != 1 {
		t.Errorf("expected 1 vote for side A, got: %d", scoreA)
	}
	if scoreB != 1 {
		t.Errorf("expected 1 vote for side B, got: %d", scoreB)
	}
}

// TEST 3: AFK player cleanup.
func TestCleanupInactivePlayers(t *testing.T) {
	g := setupTestGame()

	p1 := &Player{ID: "p1", Pos: Pos{X: 5, Y: 5}, LastActive: time.Now().Add(-1 * time.Second)}
	g.Players["p1"] = p1
	g.Grid[p1.Pos] = p1

	// p2 has been AFK for 24h, which guarantees removal.
	p2 := &Player{ID: "p2", Pos: Pos{X: 6, Y: 6}, LastActive: time.Now().Add(-24 * time.Hour)}
	g.Players["p2"] = p2
	g.Grid[p2.Pos] = p2

	g.cleanupInactive()

	if _, exists := g.Players["p1"]; !exists {
		t.Errorf("an active player was removed by mistake")
	}
	if _, exists := g.Players["p2"]; exists {
		t.Errorf("the AFK player was not removed")
	}
}

// TEST 4: radiation debuff expiry.
func TestDebuffs(t *testing.T) {
	g := setupTestGame()

	p1 := &Player{
		ID:              "p1",
		IsIrradiated:    true,
		IrradiatedUntil: time.Now().Add(-1 * time.Second),
	}
	g.Players["p1"] = p1

	g.processDebuffs()

	if p1.IsIrradiated {
		t.Errorf("the radiation debuff did not expire after its duration")
	}
}
