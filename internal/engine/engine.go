package engine

import (
	"strconv"
	"strings"
	"time"

	"Votan/internal/storage"
)

type Pos struct {
	X, Y int
}

type Command struct {
	PlayerID   string
	PlayerName string
	Action     string
}

type Player struct {
	ID             string
	Name           string
	Pos            Pos
	TargetDx       int
	TargetDy       int
	RemainingSteps int
}

type Game struct {
	IsRunning    bool
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
		IsRunning:    true,
		Players:      make(map[string]*Player),
		Grid:         make(map[Pos]*Player),
		BlockedCells: make(map[Pos]bool),
		CommandChan:  make(chan Command, 1000),
		MaxX:         20, // Кількість клітинок по горизонталі
		MaxY:         35, // Кількість клітинок по вертикалі
		DB:           db,
	}

	g.initStaticMap()

	savedUsers, err := db.LoadAllUsers()
	if err == nil {
		for _, u := range savedUsers {
			pos := Pos{X: u.X, Y: u.Y}
			player := &Player{ID: u.ID, Name: u.Name, Pos: pos}
			if g.isInsideMap(pos) && g.Grid[pos] == nil && !g.BlockedCells[pos] {
				g.Players[u.ID] = player
				g.Grid[pos] = player
			}
		}
	}

	return g
}

// initStaticMap — сюди ми додамо твої камені
func (g *Game) initStaticMap() {
	// Приклад: g.BlockedCells[Pos{X: 10, Y: 10}] = true
}

// Допоміжна функція для перевірки меж
func (g *Game) isInsideMap(p Pos) bool {
	return p.X >= 0 && p.X < g.MaxX && p.Y >= 0 && p.Y < g.MaxY
}

func (g *Game) Start() {
	ticker := time.NewTicker(500 * time.Millisecond) // Крок кожні 0.5 сек
	defer ticker.Stop()

	for range ticker.C {
		g.tick()
	}
}

func (g *Game) tick() {
	commandsToProcess := len(g.CommandChan)
	for i := 0; i < commandsToProcess; i++ {
		cmd := <-g.CommandChan
		g.processCommand(cmd)
	}

	for _, p := range g.Players {
		if p.RemainingSteps > 0 {
			nextPos := Pos{X: p.Pos.X + p.TargetDx, Y: p.Pos.Y + p.TargetDy}

			// ПЕРЕВІРКА МЕЖ КАРТИ (Щоб не виходив за межі 20х35)
			if !g.isInsideMap(nextPos) {
				p.RemainingSteps = 0
				continue
			}

			// ПЕРЕВІРКА ПЕРЕШКОД ТА ІНШИХ ГРАВЦІВ
			if g.BlockedCells[nextPos] || g.Grid[nextPos] != nil {
				p.RemainingSteps = 0
				continue
			}

			// Переміщення
			delete(g.Grid, p.Pos)
			p.Pos = nextPos
			g.Grid[p.Pos] = p
			p.RemainingSteps--

			g.DB.UpsertUser(p.ID, p.Name, p.Pos.X, p.Pos.Y)
		}
	}
}

func (g *Game) processCommand(cmd Command) {
	cleanName := strings.TrimPrefix(cmd.PlayerName, "@")

	player, exists := g.Players[cmd.PlayerID]
	if !exists {
		spawnPos := g.findFreeSpawn()
		player = &Player{ID: cmd.PlayerID, Name: cleanName, Pos: spawnPos}
		g.Players[cmd.PlayerID] = player
		g.Grid[spawnPos] = player
		g.DB.UpsertUser(player.ID, player.Name, player.Pos.X, player.Pos.Y)
	}

	if cmd.Action == "" {
		return
	}

	dx, dy, steps := parseAction(cmd.Action)
	if steps > 0 {
		player.TargetDx = dx
		player.TargetDy = dy
		player.RemainingSteps = steps
	}
}

func parseAction(action string) (dx, dy, steps int) {
	if len(action) < 2 || action[0] != '!' {
		return 0, 0, 0
	}
	switch action[1] {
	case 'r': dx = 1
	case 'l': dx = -1
	case 'u': dy = 1
	case 'd': dy = -1
	default: return 0, 0, 0
	}
	steps = 1
	if len(action) > 2 {
		if s, err := strconv.Atoi(action[2:]); err == nil {
			steps = s
		}
	}
	if steps > 5 { steps = 5 } // Обмежуємо за раз
	return dx, dy, steps
}

func (g *Game) findFreeSpawn() Pos {
	for y := 0; y < g.MaxY; y++ {
		for x := 0; x < g.MaxX; x++ {
			p := Pos{X: x, Y: y}
			if g.Grid[p] == nil && !g.BlockedCells[p] {
				return p
			}
		}
	}
	return Pos{X: 0, Y: 0}
}

type GameState struct {
	Players []PlayerState `json:"players"`
}

type PlayerState struct {
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

func (g *Game) GetState() GameState {
	state := GameState{Players: make([]PlayerState, 0, len(g.Players))}
	for _, p := range g.Players {
		state.Players = append(state.Players, PlayerState{Name: p.Name, X: p.Pos.X, Y: p.Pos.Y})
	}
	return state
}