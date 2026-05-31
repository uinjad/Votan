package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite" // CGO-free SQLite driver

	"Votan/internal/engine"
)

// Compile-time guarantee that DB satisfies the engine's persistence port.
var _ engine.Store = (*DB)(nil)

// writeOp is a single asynchronous persistence operation.
type writeOp struct {
	desc string
	run  func(context.Context) error
}

// DB is a SQLite-backed engine.Store. Writes are queued and applied by a
// single background goroutine, so the game loop never blocks on disk I/O and
// SQLite never sees concurrent writers. Reads run synchronously.
type DB struct {
	sql    *sql.DB
	writes chan writeOp
	done   chan struct{}
	wg     sync.WaitGroup
}

const (
	writeQueueSize = 4096
	writeTimeout   = 5 * time.Second
)

// InitDB opens the database, ensures the schema and starts the writer.
func InitDB(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open %q: %w", path, err)
	}
	// SQLite allows one writer; a single open connection keeps writes serial
	// and avoids "database is locked" under the async writer.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("storage: ping: %w", err)
	}

	const schema = `
		CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			name          TEXT,
			x             INTEGER,
			y             INTEGER,
			status        INTEGER DEFAULT 0,
			is_irradiated BOOLEAN DEFAULT 0,
			head_id       INTEGER DEFAULT 0,
			body_id       INTEGER DEFAULT 0
		);`
	if _, err := sqlDB.Exec(schema); err != nil {
		return nil, fmt.Errorf("storage: create schema: %w", err)
	}

	db := &DB{
		sql:    sqlDB,
		writes: make(chan writeOp, writeQueueSize),
		done:   make(chan struct{}),
	}
	db.wg.Add(1)
	go db.writer()
	return db, nil
}

func (db *DB) writer() {
	defer db.wg.Done()
	for {
		select {
		case op := <-db.writes:
			db.exec(op)
		case <-db.done:
			// Drain whatever is queued, then stop.
			for {
				select {
				case op := <-db.writes:
					db.exec(op)
				default:
					return
				}
			}
		}
	}
}

func (db *DB) exec(op writeOp) {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	if err := op.run(ctx); err != nil {
		// Best-effort persistence: log, don't crash the stream.
		slog.Error("storage: write failed", "op", op.desc, "err", err)
	}
}

// enqueue submits a best-effort write. If the queue is full or the store is
// shutting down, the write is dropped (and logged) and the caller never blocks.
func (db *DB) enqueue(op writeOp) {
	select {
	case <-db.done:
		return
	default:
	}
	select {
	case db.writes <- op:
	default:
		slog.Warn("storage: write queue full, dropping op", "op", op.desc)
	}
}

func (db *DB) UpsertUser(id, name string, x, y int) {
	db.enqueue(writeOp{desc: "upsert_user", run: func(ctx context.Context) error {
		_, err := db.sql.ExecContext(ctx, `
			INSERT INTO users (id, name, x, y) VALUES (?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name, x = excluded.x, y = excluded.y;`,
			id, name, x, y)
		return err
	}})
}

func (db *DB) UpdateSkin(id string, head, body int) {
	db.enqueue(writeOp{desc: "update_skin", run: func(ctx context.Context) error {
		_, err := db.sql.ExecContext(ctx,
			"UPDATE users SET head_id = ?, body_id = ? WHERE id = ?", head, body, id)
		return err
	}})
}

func (db *DB) Baptize(id string) {
	db.enqueue(writeOp{desc: "baptize", run: func(ctx context.Context) error {
		_, err := db.sql.ExecContext(ctx,
			"UPDATE users SET status = 1 WHERE id = ?", id)
		return err
	}})
}

func (db *DB) SetIrradiated(id string, irradiated bool) {
	db.enqueue(writeOp{desc: "set_irradiated", run: func(ctx context.Context) error {
		_, err := db.sql.ExecContext(ctx,
			"UPDATE users SET is_irradiated = ? WHERE id = ?", irradiated, id)
		return err
	}})
}

func (db *DB) DeleteUser(id string) {
	db.enqueue(writeOp{desc: "delete_user", run: func(ctx context.Context) error {
		_, err := db.sql.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
		return err
	}})
}

// LoadAllUsers reads every persisted user. Used once at startup.
func (db *DB) LoadAllUsers(ctx context.Context) ([]engine.UserRecord, error) {
	rows, err := db.sql.QueryContext(ctx,
		"SELECT id, name, x, y, status, is_irradiated, head_id, body_id FROM users")
	if err != nil {
		return nil, fmt.Errorf("storage: query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []engine.UserRecord
	for rows.Next() {
		var u engine.UserRecord
		if err := rows.Scan(&u.ID, &u.Name, &u.X, &u.Y, &u.Status,
			&u.IsIrradiated, &u.HeadID, &u.BodyID); err != nil {
			return nil, fmt.Errorf("storage: scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate users: %w", err)
	}
	return users, nil
}

// Close stops the background writer (after draining queued writes) and closes
// the database. It must be called only after all writers have stopped.
func (db *DB) Close() error {
	close(db.done)
	db.wg.Wait()
	if err := db.sql.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	return nil
}
