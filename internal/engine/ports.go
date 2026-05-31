package engine

import (
	"context"
	"time"
)

// UserRecord is the persisted projection of a player. The engine owns this
// type (and the Store interface below), so the persistence layer depends on
// the domain, not the other way around — this is the "consumer-defined
// interface" pattern and is what makes the engine testable in isolation.
type UserRecord struct {
	ID           string
	Name         string
	X, Y         int
	Status       int
	IsIrradiated bool
	HeadID       int
	BodyID       int
}

// Store persists player state.
//
// Writes are intentionally fire-and-forget: implementations apply them
// asynchronously and log their own failures, so the 100 ms game loop never
// blocks on disk I/O. The authoritative state lives in memory; persistence is
// a convenience for surviving restarts.
//
// Reads are synchronous and context-aware and are only used at startup.
type Store interface {
	UpsertUser(id, name string, x, y int)
	UpdateSkin(id string, head, body int)
	Baptize(id string)
	SetIrradiated(id string, irradiated bool)
	DeleteUser(id string)
	LoadAllUsers(ctx context.Context) ([]UserRecord, error)
}

// Scene drives the on-stream presentation (OBS). Every method is a best-effort
// side effect; failures are logged by the implementation.
type Scene interface {
	RestartMedia(name string)
	SetSourceEnabled(scene, source string, enabled bool)
	FadeSourceOpacity(source, filter string, from, to float64, d time.Duration)
	SetOpacity(source, filter string, opacity float64)
}

// NopStore is a Store that does nothing. Used for headless runs and tests, so
// the engine never has to nil-check its dependencies.
type NopStore struct{}

func (NopStore) UpsertUser(id, name string, x, y int) {
}

func (NopStore) UpdateSkin(id string, head, body int) {
}

func (NopStore) Baptize(id string) {
}

func (NopStore) SetIrradiated(id string, irradiated bool) {
}

func (NopStore) DeleteUser(id string) {
}

func (NopStore) LoadAllUsers(ctx context.Context) ([]UserRecord, error) {
	return nil, nil
}

// NopScene is a Scene that does nothing. Used when OBS is not connected.
type NopScene struct{}

func (NopScene) RestartMedia(name string) {
}

func (NopScene) SetSourceEnabled(scene, source string, enabled bool) {
}

func (NopScene) FadeSourceOpacity(source, filter string, from, to float64, d time.Duration) {
}

func (NopScene) SetOpacity(source, filter string, opacity float64) {
}
