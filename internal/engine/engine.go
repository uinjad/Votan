package engine

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Votan/internal/storage"
)

var skinRegex = regexp.MustCompile(`(?i)^!h(\d+)b(\d+)$`)

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
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
	LastActive     time.Time

	Status          int
	IsIrradiated    bool
	IrradiatedUntil time.Time // Час дії дебафу 5G
	HeadID          int
	BodyID          int

	LastMessage string
	MessageTime time.Time
}

type Game struct {
	Players      map[string]*Player
	Grid         map[Pos]*Player
	BlockedCells map[Pos]bool
	CommandChan  chan Command
	MaxX, MaxY   int
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
	Attack5GZones   map[Pos]bool // Зони ураження
	Attack5GEndTime time.Time
}

func NewGame(db *storage.DB) *Game {
	g := &Game{
		Players:      make(map[string]*Player),
		Grid:         make(map[Pos]*Player),
		BlockedCells: make(map[Pos]bool),
		CommandChan:  make(chan Command, 1000),
		MaxX:         20, MaxY: 35,
		DB: db,
	}

	g.initStaticMap()
	savedUsers, _ := db.LoadAllUsers()
	for _, u := range savedUsers {
		pos := Pos{X: u.X, Y: u.Y}
		if pos.X >= 0 && pos.X < g.MaxX && pos.Y >= 0 && pos.Y < g.MaxY && g.Grid[pos] == nil && !g.BlockedCells[pos] {
			player := &Player{
				ID: u.ID, Name: u.Name, Pos: pos, LastActive: time.Now(),
				Status: u.Status, IsIrradiated: u.IsIrradiated, HeadID: u.HeadID, BodyID: u.BodyID,
			}
			g.Players[u.ID] = player
			g.Grid[pos] = player
		}
	}
	return g
}

func (g *Game) initStaticMap() {
	for x := 0; x < g.MaxX; x++ {
		g.BlockedCells[Pos{X: x, Y: 0}] = true
		g.BlockedCells[Pos{X: x, Y: g.MaxY - 1}] = true
	}
	for y := 0; y < g.MaxY; y++ {
		g.BlockedCells[Pos{X: 0, Y: y}] = true
		g.BlockedCells[Pos{X: g.MaxX - 1, Y: y}] = true
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

	// 🛑 ТАЙМЕР ВІЧЕ
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
		g.VoteResult = fmt.Sprintf("РІШЕННЯ: %s (Голосів %d:%d)", winner, scoreA, scoreB)
		g.VoteResultEnd = time.Now().Add(10 * time.Second)
	}
	if g.VoteResult != "" && time.Now().After(g.VoteResultEnd) {
		g.VoteResult = ""
	}

	// 📡 ТАЙМЕР 5G АТАКИ
	if g.Attack5GActive && time.Now().After(g.Attack5GEndTime) {
		g.Attack5GActive = false
		for _, p := range g.Players {
			if g.Attack5GZones[p.Pos] {
				p.IsIrradiated = true
				p.IrradiatedUntil = time.Now().Add(30 * time.Second) // Дебаф на 30 сек
				g.DB.SetIrradiated(p.ID, true)
			}
		}
		g.Attack5GZones = nil
		fmt.Println("📡 5G Атака завершена! Ті, хто не встиг втекти, опромінені.")
	}

	// Зняття опромінення після закінчення часу
	for _, p := range g.Players {
		if p.IsIrradiated && time.Now().After(p.IrradiatedUntil) {
			p.IsIrradiated = false
			g.DB.SetIrradiated(p.ID, false)
		}
	}

	// Рух
	for _, p := range g.Players {
		if p.RemainingSteps > 0 {
			nextPos := Pos{X: p.Pos.X + p.TargetDx, Y: p.Pos.Y + p.TargetDy}
			if nextPos.X < 0 || nextPos.X >= g.MaxX || nextPos.Y < 0 || nextPos.Y >= g.MaxY {
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

func (g *Game) processCommand(cmd Command) {
	actionStr := strings.TrimSpace(cmd.Action)
	actionLower := strings.ToLower(actionStr)

	// 👑 АДМІНКА
	if cmd.PlayerID == "GOD_MODE_ADMIN_SECRET" {
		if strings.HasPrefix(actionLower, "!віче") {
			g.VoteActive = true
			g.VoteResult = ""
			parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(actionStr, "!віче")), "|")
			g.VoteTopic = "ВИБІР ДОЛІ"
			g.VoteOptionA = "ЗА"
			g.VoteOptionB = "ПРОТИ"
			if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
				g.VoteTopic = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
				g.VoteOptionA = strings.TrimSpace(parts[1])
			}
			if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
				g.VoteOptionB = strings.TrimSpace(parts[2])
			}
			g.VoteEndTime = time.Now().Add(60 * time.Second)
			return
		}
		if actionLower == "!stop_vote" {
			g.VoteActive = false
			return
		}

		// ЗАПУСК 5G АТАКИ
		if actionLower == "!5g" {
			g.Attack5GActive = true
			g.Attack5GEndTime = time.Now().Add(20 * time.Second)
			g.Attack5GZones = make(map[Pos]bool)

			// Генеруємо 4 випадкових квадратних зони розміром 5х5
			for i := 0; i < 4; i++ {
				cx, cy := rand.Intn(g.MaxX-4)+2, rand.Intn(g.MaxY-4)+2
				for dx := -2; dx <= 2; dx++ {
					for dy := -2; dy <= 2; dy++ {
						g.Attack5GZones[Pos{X: cx + dx, Y: cy + dy}] = true
					}
				}
			}
			fmt.Println("📡 5G Атака почалася!")
			return
		}
		return
	}

	player, exists := g.Players[cmd.PlayerID]
	if !exists {
		spawnPos, ok := g.findFreeSpawn()
		if !ok {
			return
		}
		player = &Player{ID: cmd.PlayerID, Name: strings.TrimPrefix(cmd.PlayerName, "@"), Pos: spawnPos, LastActive: time.Now()}
		g.Players[cmd.PlayerID] = player
		g.Grid[spawnPos] = player
		g.DB.UpsertUser(player.ID, player.Name, player.Pos.X, player.Pos.Y)
	}

	player.LastActive = time.Now()

	// ☢️ ДЕБАФ ОПРОМІНЕННЯ: Перехоплення управління
	if player.IsIrradiated && actionStr != "" {
		phrases := []string{"Хочу ревакцинуватись!", "Слава Ящерам!", "5G - це здоров'я!", "Завантажую оновлення..."}
		player.LastMessage = phrases[rand.Intn(len(phrases))]
		player.MessageTime = time.Now()
		player.RemainingSteps = 0 // Блокуємо рух
		return                    // Не обробляємо реальні команди
	}

	// Звичайні команди
	if actionStr != "" {
		isCommand := false
		if actionLower == "!tk" {
			player.Status = 1
			player.HeadID = 1
			player.BodyID = 1
			g.DB.BaptizeUser(player.ID, "mock_telegram_id")
			g.DB.UpdateSkin(player.ID, player.HeadID, player.BodyID)
			player.LastMessage = "Всяке даяніє благо єсмь"
			player.MessageTime = time.Now()
			isCommand = true
		} else if matches := skinRegex.FindStringSubmatch(actionLower); matches != nil {
			if player.Status != 1 {
				player.LastMessage = "Тільки Хрещені можуть змінювати зовнішність!"
				player.MessageTime = time.Now()
			} else {
				h, _ := strconv.Atoi(matches[1])
				b, _ := strconv.Atoi(matches[2])
				if h >= 0 && h <= 16 && b >= 0 && b <= 14 {
					player.HeadID = h
					player.BodyID = b
					g.DB.UpdateSkin(player.ID, h, b)
				}
			}
			isCommand = true
		} else if strings.HasPrefix(actionLower, "!") {
			dx, dy, steps := parseAction(actionLower)
			if steps > 0 {
				player.TargetDx = dx
				player.TargetDy = dy
				player.RemainingSteps = steps
				isCommand = true
			}
		}

		if !isCommand && !strings.HasPrefix(actionStr, "!") {
			player.LastMessage = actionStr
			player.MessageTime = time.Now()
		}
	}
}

func (g *Game) findFreeSpawn() (Pos, bool) {
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
	if steps > 33 {
		steps = 33
	}
	return dx, dy, steps
}

// JSON ДЛЯ ФРОНТЕНДУ
type GameState struct {
	Players []PlayerState `json:"players"`
	// Віче
	VoteActive   bool   `json:"voteActive"`
	VoteTopic    string `json:"voteTopic"`
	VoteOptionA  string `json:"voteOptionA"`
	VoteOptionB  string `json:"voteOptionB"`
	VoteTimeLeft int    `json:"voteTimeLeft"`
	VoteScoreA   int    `json:"voteScoreA"`
	VoteScoreB   int    `json:"voteScoreB"`
	VoteResult   string `json:"voteResult"`
	// 5G
	Attack5GActive   bool  `json:"attack5gActive"`
	Attack5GTimeLeft int   `json:"attack5gTimeLeft"`
	Attack5GZones    []Pos `json:"attack5gZones"` // Масив координат для відмальовки
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
	state := GameState{Players: make([]PlayerState, 0, len(g.Players)), VoteActive: g.VoteActive, VoteTopic: g.VoteTopic, VoteOptionA: g.VoteOptionA, VoteOptionB: g.VoteOptionB, VoteResult: g.VoteResult, Attack5GActive: g.Attack5GActive}

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
		// Передаємо масив зон
		for z := range g.Attack5GZones {
			state.Attack5GZones = append(state.Attack5GZones, z)
		}
	}

	scoreA, scoreB := 0, 0
	for _, p := range g.Players {
		msg := ""
		if time.Since(p.MessageTime) < 7*time.Second {
			msg = p.LastMessage
		}
		if g.VoteActive {
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
		state.Players = append(state.Players, PlayerState{
			Name: p.Name, X: p.Pos.X, Y: p.Pos.Y, Status: p.Status,
			IsIrradiated: p.IsIrradiated, HeadID: p.HeadID, BodyID: p.BodyID, Message: msg,
		})
	}

	state.VoteScoreA = scoreA
	state.VoteScoreB = scoreB
	return state
}
