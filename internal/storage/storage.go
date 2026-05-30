package storage

import (
	"database/sql"

	_ "github.com/glebarez/go-sqlite"
)

type UserRow struct {
	ID           string
	Name         string
	X, Y         int
	Status       int
	IsIrradiated bool
	HeadID       int
	BodyID       int
}

type DB struct {
	sql *sql.DB
}

func InitDB(path string) (*DB, error) {
	// Use "sqlite" (not "sqlite3") for the pure-Go driver.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	query := `
    CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        name TEXT,
        x INTEGER,
        y INTEGER,
        status INTEGER DEFAULT 0,
        is_irradiated BOOLEAN DEFAULT 0,
        head_id INTEGER DEFAULT 0,
        body_id INTEGER DEFAULT 0
    );`

	if _, err := db.Exec(query); err != nil {
		return nil, err
	}

	return &DB{sql: db}, nil
}

func (db *DB) UpsertUser(id, name string, x, y int) {
	query := `
    INSERT INTO users (id, name, x, y) VALUES (?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET name=excluded.name, x=excluded.x, y=excluded.y;`
	db.sql.Exec(query, id, name, x, y)
}

func (db *DB) UpdateSkin(id string, head, body int) {
	db.sql.Exec("UPDATE users SET head_id = ?, body_id = ? WHERE id = ?", head, body, id)
}

func (db *DB) BaptizeUser(id, blessingType string) {
	db.sql.Exec("UPDATE users SET status = 1 WHERE id = ?", id)
}

func (db *DB) SetIrradiated(id string, state bool) {
	db.sql.Exec("UPDATE users SET is_irradiated = ? WHERE id = ?", state, id)
}

func (db *DB) DeleteUser(id string) error {
	_, err := db.sql.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (db *DB) LoadAllUsers() ([]UserRow, error) {
	rows, err := db.sql.Query("SELECT id, name, x, y, status, is_irradiated, head_id, body_id FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Name, &u.X, &u.Y, &u.Status, &u.IsIrradiated, &u.HeadID, &u.BodyID); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

func (db *DB) GetUser(id string) (*UserRow, error) {
	row := db.sql.QueryRow("SELECT name, x, y, status, is_irradiated, head_id, body_id FROM users WHERE id = ?", id)

	var u UserRow
	u.ID = id
	err := row.Scan(&u.Name, &u.X, &u.Y, &u.Status, &u.IsIrradiated, &u.HeadID, &u.BodyID)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) Close() {
	db.sql.Close()
}
