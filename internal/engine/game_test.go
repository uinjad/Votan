package engine

import (
	"testing"
	"time"

	"Votan/internal/config"
)

// Допоміжна функція для створення чистої гри
func setupTestGame() *Game {
	return NewGame(nil, nil)
}

// ТЕСТ 1: Перевірка руху та колізій
func TestMovementAndCollisions(t *testing.T) {
	g := setupTestGame()

	p1 := &Player{
		ID: "p1", Name: "Ivan",
		Pos: Pos{X: 10, Y: 10},
	}
	g.Players["p1"] = p1
	g.Grid[p1.Pos] = p1

	// Сценарій 1.1: Успішний рух
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos.X != 11 || p1.Pos.Y != 10 {
		t.Errorf("Гравець не зрушив на очікувану клітинку. Поточна позиція: %v", p1.Pos)
	}

	// Сценарій 1.2: Зіткнення з межею карти (стіною)
	p1.Pos = Pos{X: config.MaxX - 1, Y: 10}
	g.Grid[p1.Pos] = p1
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos.X >= config.MaxX {
		t.Errorf("Гравець вийшов за межі карти! Позиція: %v", p1.Pos)
	}
	if p1.RemainingSteps != 0 {
		t.Errorf("Кроки не обнулилися після удару об стіну")
	}

	// Сценарій 1.3: Зіткнення з іншим гравцем
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
		t.Errorf("Гравець p1 наступив на гравця p2!")
	}

	// Сценарій 1.4: Зіткнення зі статичною перешкодою
	obstaclePos := Pos{X: 15, Y: 15}
	g.BlockedCells[obstaclePos] = true
	p1.Pos = Pos{X: 14, Y: 15}
	g.Grid[p1.Pos] = p1
	p1.TargetDx = 1
	p1.TargetDy = 0
	p1.RemainingSteps = 1
	g.tick()

	if p1.Pos == obstaclePos {
		t.Errorf("Гравець пройшов крізь дерево/камінь!")
	}
}

// ТЕСТ 2: Перевірка логіки Віче
func TestVotingSystem(t *testing.T) {
	g := setupTestGame()
	g.VoteActive = true

	// Сірий (нехрещений)
	p1 := &Player{ID: "p1", Status: 0, Voted: true, Pos: Pos{X: 5, Y: 10}}
	g.Players["p1"] = p1

	// Хрещений, не рухався
	p2 := &Player{ID: "p2", Status: 1, Voted: false, Pos: Pos{X: 5, Y: 11}}
	g.Players["p2"] = p2

	// Хрещений, рухався ЗА
	p3 := &Player{ID: "p3", Status: 1, Voted: true, Pos: Pos{X: 5, Y: 12}}
	g.Players["p3"] = p3

	// Хрещений, рухався ПРОТИ
	p4 := &Player{ID: "p4", Status: 1, Voted: true, Pos: Pos{X: 15, Y: 12}}
	g.Players["p4"] = p4

	scoreA, scoreB := g.calculateCurrentScores()

	if scoreA != 1 {
		t.Errorf("Очікувався 1 голос ЗА, отримано: %d", scoreA)
	}
	if scoreB != 1 {
		t.Errorf("Очікувався 1 голос ПРОТИ, отримано: %d", scoreB)
	}
}

// ТЕСТ 3: Очищення AFK гравців
func TestCleanupInactivePlayers(t *testing.T) {
	g := setupTestGame()

	p1 := &Player{ID: "p1", Pos: Pos{X: 5, Y: 5}, LastActive: time.Now().Add(-1 * time.Second)}
	g.Players["p1"] = p1
	g.Grid[p1.Pos] = p1

	p2 := &Player{ID: "p2", Pos: Pos{X: 6, Y: 6}, LastActive: time.Now().Add(-20 * time.Minute)}
	g.Players["p2"] = p2
	g.Grid[p2.Pos] = p2

	g.cleanupInactive()

	if _, exists := g.Players["p1"]; !exists {
		t.Errorf("Активного гравця було помилково видалено!")
	}
	if _, exists := g.Players["p2"]; exists {
		t.Errorf("AFK гравця НЕ було видалено!")
	}
}

// ТЕСТ 4: Спадіння радіації
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
		t.Errorf("Дебаф радіації не спав після закінчення терміну дії!")
	}
}
