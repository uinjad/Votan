package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"Votan/internal/config"
)

func newTestGame() *Game {
	return NewGame(nil, nil, Config{MaxHeadID: 10, MaxBodyID: 10})
}

func TestMovementAndCollisions(t *testing.T) {
	g := newTestGame()

	// 1.1: a free move succeeds.
	p1 := &Player{ID: "p1", Pos: Pos{X: 10, Y: 10}, TargetDx: 1, RemainingSteps: 1}
	g.players["p1"] = p1
	g.grid[p1.Pos] = p1
	g.tick()
	if p1.Pos.X != 11 || p1.Pos.Y != 10 {
		t.Errorf("expected move to (11,10), got (%d,%d)", p1.Pos.X, p1.Pos.Y)
	}

	// 1.2: a wall stops movement.
	p1.Pos = Pos{X: config.MaxX - 1, Y: 10}
	g.grid[p1.Pos] = p1
	p1.TargetDx, p1.RemainingSteps = 1, 1
	g.tick()
	if p1.Pos.X >= config.MaxX || p1.RemainingSteps != 0 {
		t.Errorf("wall not respected: pos.X=%d steps=%d", p1.Pos.X, p1.RemainingSteps)
	}

	// 1.3: collision with another player blocks the move.
	p2 := &Player{ID: "p2", Pos: Pos{X: 10, Y: 10}}
	g.players["p2"] = p2
	g.grid[p2.Pos] = p2
	p1.Pos = Pos{X: 9, Y: 10}
	g.grid[p1.Pos] = p1
	p1.TargetDx, p1.RemainingSteps = 1, 1
	g.tick()
	if p1.Pos.X == 10 {
		t.Error("player walked through another player")
	}

	// 1.4: a static obstacle blocks the move.
	obstacle := Pos{X: 15, Y: 15}
	g.blockedCells[obstacle] = true
	p1.Pos = Pos{X: 14, Y: 15}
	g.grid[p1.Pos] = p1
	p1.TargetDx, p1.RemainingSteps = 1, 1
	g.tick()
	if p1.Pos == obstacle {
		t.Error("player walked onto a static obstacle")
	}
}

func TestVotingSystem(t *testing.T) {
	g := newTestGame()
	g.voteActive = true

	add := func(id string, x int, status int, voted bool) {
		g.players[id] = &Player{ID: id, Pos: Pos{X: x, Y: 10}, Status: status, Voted: voted}
	}
	add("a", 5, 1, true)  // baptized, voted, left  -> A
	add("b", 15, 1, true) // baptized, voted, right -> B
	add("c", 3, 0, true)  // unbaptized -> ignored
	add("d", 4, 1, false) // didn't vote -> ignored

	scoreA, scoreB := g.tallyVotes()
	if scoreA != 1 || scoreB != 1 {
		t.Errorf("expected 1:1, got %d:%d", scoreA, scoreB)
	}
}

func TestCleanupInactivePlayers(t *testing.T) {
	g := newTestGame()
	g.players["active"] = &Player{ID: "active", Pos: Pos{X: 5, Y: 5}, LastActive: time.Now()}
	g.players["afk"] = &Player{ID: "afk", Pos: Pos{X: 6, Y: 6}, LastActive: time.Now().Add(-24 * time.Hour)}

	g.cleanupInactive()

	if _, ok := g.players["active"]; !ok {
		t.Error("active player was wrongly removed")
	}
	if _, ok := g.players["afk"]; ok {
		t.Error("AFK player was not cleaned up")
	}
}

func TestDebuffs(t *testing.T) {
	g := newTestGame()
	p := &Player{ID: "p", IsIrradiated: true, IrradiatedUntil: time.Now().Add(-time.Second)}
	g.players["p"] = p

	g.processDebuffs()

	if p.IsIrradiated {
		t.Error("expired debuff was not cleared")
	}
}

// TestConcurrentAccessIsRaceFree exercises the locking by hammering the command
// channel and GetState while the loop runs. Run with: go test -race ./...
func TestConcurrentAccessIsRaceFree(t *testing.T) {
	g := newTestGame()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { defer close(done); g.Run(ctx) }()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("u%d", n)
			for j := 0; j < 200; j++ {
				g.Commands() <- Command{PlayerID: id, PlayerName: id, Action: "!r1"}
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = g.GetState()
			}
		}()
	}

	wg.Wait()
	cancel()
	<-done
}
