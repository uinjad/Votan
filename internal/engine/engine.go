package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Votan/internal/storage"
)

// Регулярний вираз для переодягання: шукає формат !h1b2
var skinRegex = regexp.MustCompile(`^!h(\d+)b(\d+)$`)

// Pos визначає координати на матриці
type Pos struct {
	X, Y int
}

// Command описує вхідну команду з чату
type Command struct {
	PlayerID   string
	PlayerName string
	Action     string
}

// Player описує стан гравця
type Player struct {
	ID             string
	Name           string
	Pos            Pos
	TargetDx       int
	TargetDy       int
	RemainingSteps int
	LastActive     time.Time

	// Дані з БД
	Status       int
	IsIrradiated bool
	HeadID       int
	BodyID       int

	// Дані для чату
	LastMessage string
	MessageTime time.Time
}

// Game керує станом ігрового світу
type Game struct {
	Players      map[string]*Player
	Grid         map[Pos]*Player
	BlockedCells map[Pos]bool
	CommandChan  chan Command
	MaxX         int
	MaxY         int
	DB           *storage.DB
}

func NewGame(db *storage.DB) *Game {
	g := &Game{
		Players:      make(map[string]*Player),
		Grid:         make(map[Pos]*Player),
		BlockedCells: make(map[Pos]bool),
		CommandChan:  make(chan Command, 1000),
		MaxX:         20,
		MaxY:         35,
		DB:           db,
	}

	g.initStaticMap()

	// Завантаження гравців з бази при старті
	savedUsers, _ := db.LoadAllUsers()
	for _, u := range savedUsers {
		pos := Pos{X: u.X, Y: u.Y}
		if pos.X >= 0 && pos.X < g.MaxX && pos.Y >= 0 && pos.Y < g.MaxY && g.Grid[pos] == nil && !g.BlockedCells[pos] {
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

	return g
}

func (g *Game) initStaticMap() {
	// 1. Блокуємо зовнішній контур карти
	for x := 0; x < g.MaxX; x++ {
		g.BlockedCells[Pos{X: x, Y: 0}] = true
		g.BlockedCells[Pos{X: x, Y: g.MaxY - 1}] = true
	}
	for y := 0; y < g.MaxY; y++ {
		g.BlockedCells[Pos{X: 0, Y: y}] = true
		g.BlockedCells[Pos{X: g.MaxX - 1, Y: y}] = true
	}

	// 2. Блокуємо твої об'єкти (координати знизу зліва)
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

func (g *Game) Start() {
	ticker := time.NewTicker(500 * time.Millisecond)
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
	threshold := time.Now().Add(-3 * time.Hour)
	for id, p := range g.Players {
		if p.LastActive.Before(threshold) {
			delete(g.Grid, p.Pos)
			delete(g.Players, id)
			fmt.Printf("🧹 Видалено неактивного гравця: %s\n", p.Name)
		}
	}
}

func (g *Game) tick() {
	commandsToProcess := len(g.CommandChan)
	for i := 0; i < commandsToProcess; i++ {
		cmd := <-g.CommandChan
		g.processCommand(cmd)
	}

	// Рух гравців
	for _, p := range g.Players {
		if p.RemainingSteps > 0 {
			nextPos := Pos{X: p.Pos.X + p.TargetDx, Y: p.Pos.Y + p.TargetDy}

			// Межі карти
			if nextPos.X < 0 || nextPos.X >= g.MaxX || nextPos.Y < 0 || nextPos.Y >= g.MaxY {
				p.RemainingSteps = 0
				continue
			}

			// Колізії
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

func (g *Game) processCommand(cmd Command) {
	player, exists := g.Players[cmd.PlayerID]

	// Спавн нового гравця
	if !exists {
		spawnPos, ok := g.findFreeSpawn()
		if !ok {
			return // Карта переповнена
		}
		player = &Player{
			ID:         cmd.PlayerID,
			Name:       strings.TrimPrefix(cmd.PlayerName, "@"),
			Pos:        spawnPos,
			LastActive: time.Now(),
			Status:     0,
			HeadID:     0,
			BodyID:     0,
		}
		g.Players[cmd.PlayerID] = player
		g.Grid[spawnPos] = player
		g.DB.UpsertUser(player.ID, player.Name, player.Pos.X, player.Pos.Y)
	}

	player.LastActive = time.Now()
	actionStr := strings.TrimSpace(cmd.Action)
	actionLower := strings.ToLower(actionStr)

	if actionStr != "" {
		isCommand := false

		// 1. Секретна команда для отримання статусу "Хрещений"
		if actionLower == "!tk" {
			player.Status = 1
			player.HeadID = 1 // Автоматично одягаємо голову 1
			player.BodyID = 1 // Автоматично одягаємо тіло 1

			// Зберігаємо в БД
			g.DB.BaptizeUser(player.ID, "mock_telegram_id")
			g.DB.UpdateSkin(player.ID, player.HeadID, player.BodyID)

			// Канонічна фраза в чат
			player.LastMessage = "Всяке даяніє благо єсмь"
			player.MessageTime = time.Now()
			isCommand = true

			// 2. Зміна скінів (команда типу !h1b2)
		} else if matches := skinRegex.FindStringSubmatch(actionLower); matches != nil {
			if player.Status != 1 {
				player.LastMessage = "Тільки Хрещені можуть змінювати зовнішність!"
				player.MessageTime = time.Now()
			} else {
				headVal, _ := strconv.Atoi(matches[1])
				bodyVal, _ := strconv.Atoi(matches[2])

				// Ліміт голів: 0-16, Тіла: 0-14
				if headVal >= 0 && headVal <= 16 && bodyVal >= 0 && bodyVal <= 14 {
					player.HeadID = headVal
					player.BodyID = bodyVal
					g.DB.UpdateSkin(player.ID, player.HeadID, player.BodyID)
				} else {
					player.LastMessage = "Такого одягу не існує!"
					player.MessageTime = time.Now()
				}
			}
			isCommand = true

			// 3. Рух (!r5)
		} else if strings.HasPrefix(actionLower, "!") {
			dx, dy, steps := parseAction(actionLower)
			if steps > 0 {
				player.TargetDx = dx
				player.TargetDy = dy
				player.RemainingSteps = steps
				isCommand = true
			}
		}

		// 4. Якщо це звичайне повідомлення (не команда) - виводимо в чат
		if !isCommand && !strings.HasPrefix(actionStr, "!") {
			player.LastMessage = actionStr
			player.MessageTime = time.Now()
		}
	}
}

func (g *Game) findFreeSpawn() (Pos, bool) {
	// Шукаємо клітинку (відступаємо від країв)
	for y := 1; y < g.MaxY-1; y++ {
		for x := 1; x < g.MaxX-1; x++ {
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
	// Максимальний ліміт на дистанцію ходіння - 33
	if steps > 33 {
		steps = 33
	}
	return dx, dy, steps
}

// Передаємо стан на фронтенд
type GameState struct {
	Players []PlayerState `json:"players"`
}

type PlayerState struct {
	Name         string `json:"name"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Status       int    `json:"status"`
	IsIrradiated bool   `json:"isIrradiated"`
	HeadID       int    `json:"headId"`
	BodyID       int    `json:"bodyId"`
	Message      string `json:"message"`
}

func (g *Game) GetState() GameState {
	state := GameState{Players: make([]PlayerState, 0, len(g.Players))}
	for _, p := range g.Players {

		msg := ""
		// Хмаринка живе 7 секунд
		if time.Since(p.MessageTime) < 7*time.Second {
			msg = p.LastMessage
		}

		state.Players = append(state.Players, PlayerState{
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
