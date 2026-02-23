package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // Імпортуємо драйвер анонімно
)

// DB обгортає стандартне підключення до бази
type DB struct {
	*sql.DB
}

// InitDB створює файл бази і налаштовує таблицю
func InitDB(filepath string) (*DB, error) {
	// Підключаємося до файлу (якщо його немає — він створиться)
	db, err := sql.Open("sqlite", filepath)
	if err != nil {
		return nil, fmt.Errorf("помилка відкриття БД: %w", err)
	}

	// Вмикаємо WAL-режим для високої продуктивності при паралельному записі
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return nil, fmt.Errorf("помилка увімкнення WAL: %w", err)
	}

	// Створюємо таблицю users (згідно з твоїм контрактом даних)
	query := `
	CREATE TABLE IF NOT EXISTS users (
		yt_channel_id TEXT PRIMARY KEY,
		display_name TEXT,
		status INTEGER DEFAULT 0,
		is_irradiated BOOLEAN DEFAULT FALSE,
		pos_x INTEGER DEFAULT 0,
		pos_y INTEGER DEFAULT 0,
		last_active DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("помилка створення таблиці: %w", err)
	}

	fmt.Println("💾 База даних SQLite успішно ініціалізована!")
	return &DB{db}, nil
}

// UserDTO (Data Transfer Object) для передачі даних між БД і рушієм
type UserDTO struct {
	ID   string
	Name string
	X, Y int
}

// LoadAllUsers витягує всіх гравців з бази при старті сервера
func (db *DB) LoadAllUsers() (map[string]UserDTO, error) {
	rows, err := db.Query("SELECT yt_channel_id, display_name, pos_x, pos_y FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make(map[string]UserDTO)
	for rows.Next() {
		var u UserDTO
		if err := rows.Scan(&u.ID, &u.Name, &u.X, &u.Y); err != nil {
			continue // Пропускаємо биті рядки
		}
		users[u.ID] = u
	}
	return users, nil
}

// UpsertUser оновлює координати гравця або створює нового (якщо його ще немає)
func (db *DB) UpsertUser(id, name string, x, y int) {
	query := `
	INSERT INTO users (yt_channel_id, display_name, pos_x, pos_y, last_active)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(yt_channel_id) DO UPDATE SET
		display_name = excluded.display_name,
		pos_x = excluded.pos_x,
		pos_y = excluded.pos_y,
		last_active = CURRENT_TIMESTAMP;`
	
	// Виконуємо запит у фоні, щоб не блокувати гру
	_, err := db.Exec(query, id, name, x, y)
	if err != nil {
		fmt.Printf("Помилка збереження гравця %s в БД: %v\n", name, err)
	}
}