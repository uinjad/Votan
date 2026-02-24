package engine

import (
	"Votan/internal/storage"
	"testing"
)

func TestPlayerMovement(t *testing.T) {
	// Ініціалізуємо тестову БД в пам'яті (щоб не смітити в основну)
	db, _ := storage.InitDB(":memory:")
	defer db.Close()

	// Створюємо гру без OBS для тесту
	game := NewGame(db, nil)

	// Спавнимо тестового гравця
	playerID := "test_user_1"
	game.CommandChan <- Command{PlayerID: playerID, PlayerName: "TestUser", Action: "!r1"}

	// Робимо один "тік" гри
	game.tick()

	player := game.Players[playerID]
	if player == nil {
		t.Fatalf("Гравець не був створений")
	}

	initialX := player.Pos.X

	// Симулюємо ще один тік, щоб гравець зробив крок
	game.tick()

	if player.Pos.X != initialX+1 {
		t.Errorf("Гравець не перемістився праворуч! Очікувалося %d, отримано %d", initialX+1, player.Pos.X)
	}
}
