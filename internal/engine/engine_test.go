package engine

import (
	"testing"
)

func TestParseAction(t *testing.T) {
	tests := []struct {
		name          string
		action        string
		expectedDx    int
		expectedDy    int
		expectedSteps int
	}{
		{"Рух вправо на 5", "!r5", 1, 0, 5},
		{"Рух вліво на 10", "!l10", -1, 0, 10},
		{"Рух вверх на 33", "!u33", 0, 1, 33},
		{"Рух вниз без цифри", "!d", 0, -1, 1},
		{"Перевищення ліміту (має зрізати до 33)", "!r100", 1, 0, 33},
		{"Просто текст (не команда)", "Привіт", 0, 0, 0},
		{"Крива команда", "!x5", 0, 0, 0},
		{"Команда з пробілами", "!r 5", 1, 0, 1}, // int парсер не прочитає пробіл, дасть 1 крок
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dx, dy, steps := parseAction(tt.action)
			if dx != tt.expectedDx || dy != tt.expectedDy || steps != tt.expectedSteps {
				t.Errorf("parseAction(%q) = (%d, %d, %d), очікувано (%d, %d, %d)",
					tt.action, dx, dy, steps, tt.expectedDx, tt.expectedDy, tt.expectedSteps)
			}
		})
	}
}

func TestSkinRegex(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		shouldMatch  bool
		expectedHead string
		expectedBody string
	}{
		{"Валідна команда", "!h1b2", true, "1", "2"},
		{"Валідна команда (двозначні)", "!h16b14", true, "16", "14"},
		{"Забагато символів", "!h1b2 text", false, "", ""},
		{"Неправильний формат", "!head1body2", false, "", ""},
		{"Пробіли", "!h 1 b 2", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := skinRegex.FindStringSubmatch(tt.command)
			if tt.shouldMatch {
				if matches == nil {
					t.Errorf("Очікувався збіг для %q, але його немає", tt.command)
				} else if matches[1] != tt.expectedHead || matches[2] != tt.expectedBody {
					t.Errorf("Для %q отримано h:%s b:%s, очікувано h:%s b:%s",
						tt.command, matches[1], matches[2], tt.expectedHead, tt.expectedBody)
				}
			} else if matches != nil {
				t.Errorf("Збіг не очікувався для %q, але він відбувся", tt.command)
			}
		})
	}
}

func TestBlockArea(t *testing.T) {
	// Створюємо гру без БД для тесту (передаємо nil, бо тестуємо лише матрицю)
	g := &Game{
		BlockedCells: make(map[Pos]bool),
		MaxX:         20,
		MaxY:         35,
	}

	// Блокуємо квадрат 2x2
	g.blockArea(5, 5, 6, 6)

	// Перевіряємо, чи заблокувались потрібні клітинки
	expectedBlocked := []Pos{
		{5, 5}, {6, 5},
		{5, 6}, {6, 6},
	}

	for _, p := range expectedBlocked {
		if !g.BlockedCells[p] {
			t.Errorf("Клітинка %v мала бути заблокована, але вона вільна", p)
		}
	}

	// Перевіряємо сусідню вільну клітинку
	if g.BlockedCells[Pos{X: 4, Y: 5}] {
		t.Errorf("Клітинка {4, 5} мала бути вільною, але вона заблокована")
	}
}
