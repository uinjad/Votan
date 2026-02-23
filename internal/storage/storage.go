package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // Анонімний імпорт драйвера
)

// DB обгортає стандартне підключення до бази
type DB struct {
	*sql.DB
}

// InitDB створює файл бази і налаштовує таблицю
func InitDB(filepath string) (*DB, error) {
	db, err := sql.Open("sqlite", filepath)
	if err != nil {
		return nil, fmt.Errorf("помилка відкриття БД: %w", err)
	}

	// Вмикаємо WAL-режим для високої продуктивності
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return nil, fmt.Errorf("помилка увімкнення WAL: %w", err)
	}

	query := `
	CREATE TABLE IF NOT EXISTS users (
		yt_channel_id TEXT PRIMARY KEY,
		tg_id TEXT DEFAULT '',
		display_name TEXT,
		status INTEGER DEFAULT 0,
		is_irradiated BOOLEAN DEFAULT FALSE,
		pos_x INTEGER DEFAULT 0,
		pos_y INTEGER DEFAULT 0,
		head_id INTEGER DEFAULT 0,
		body_id INTEGER DEFAULT 0,
		last_active DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("помилка створення таблиці: %w", err)
	}

	fmt.Println("💾 База даних SQLite успішно ініціалізована!")
	return &DB{db}, nil
}

// UserDTO для передачі даних між БД і рушієм
type UserDTO struct {
	ID           string
	TgID         string
	Name         string
	Status       int
	IsIrradiated bool
	X, Y         int
	HeadID       int
	BodyID       int
}

// LoadAllUsers витягує всіх гравців з бази при старті сервера
func (db *DB) LoadAllUsers() (map[string]UserDTO, error) {
	query := `SELECT yt_channel_id, tg_id, display_name, status, is_irradiated, pos_x, pos_y, head_id, body_id FROM users`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make(map[string]UserDTO)
	for rows.Next() {
		var u UserDTO
		if err := rows.Scan(&u.ID, &u.TgID, &u.Name, &u.Status, &u.IsIrradiated, &u.X, &u.Y, &u.HeadID, &u.BodyID); err != nil {
			continue
		}
		users[u.ID] = u
	}
	return users, nil
}

// UpsertUser оновлює координати та активність у фоні
func (db *DB) UpsertUser(id, name string, x, y int) {
	query := `
	INSERT INTO users (yt_channel_id, display_name, pos_x, pos_y, last_active)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(yt_channel_id) DO UPDATE SET
		display_name = excluded.display_name,
		pos_x = excluded.pos_x,
		pos_y = excluded.pos_y,
		last_active = CURRENT_TIMESTAMP;`

	go func() {
		_, err := db.Exec(query, id, name, x, y)
		if err != nil {
			fmt.Printf("Помилка збереження координат гравця %s: %v\n", name, err)
		}
	}()
}

// UpdateSkin викликається при зміні одягу
func (db *DB) UpdateSkin(id string, headID, bodyID int) {
	query := `UPDATE users SET head_id = ?, body_id = ? WHERE yt_channel_id = ?`
	go db.Exec(query, headID, bodyID, id)
}

// BaptizeUser викликається після лінковки через Telegram
func (db *DB) BaptizeUser(ytID, tgID string) {
	query := `UPDATE users SET status = 1, tg_id = ? WHERE yt_channel_id = ?`
	go db.Exec(query, tgID, ytID)
}

// SetIrradiated зберігає статус опромінення
func (db *DB) SetIrradiated(id string, isIrradiated bool) {
	query := `UPDATE users SET is_irradiated = ? WHERE yt_channel_id = ?`
	go db.Exec(query, isIrradiated, id)
}
