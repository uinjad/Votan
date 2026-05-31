package engine

import "time"

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Command is a single chat or admin instruction fed into the game loop.
type Command struct {
	PlayerID   string
	PlayerName string
	Action     string
}

// Player is the in-memory domain entity. It never leaves the engine.
type Player struct {
	ID             string
	Name           string
	Pos            Pos
	TargetDx       int
	TargetDy       int
	RemainingSteps int
	LastActive     time.Time

	Status       int
	IsIrradiated bool
	HeadID       int
	BodyID       int

	IrradiatedUntil time.Time
	LastMessage     string
	MessageTime     time.Time

	// Voted reports whether the player acted (moved) during the current vote.
	Voted bool
}

// GameState is the wire model sent to the overlay and dashboard.
type GameState struct {
	Players []PlayerState `json:"players"`

	// Vote.
	VoteActive   bool   `json:"voteActive"`
	VoteTopic    string `json:"voteTopic"`
	VoteOptionA  string `json:"voteOptionA"`
	VoteOptionB  string `json:"voteOptionB"`
	VoteTimeLeft int    `json:"voteTimeLeft"`
	VoteScoreA   int    `json:"voteScoreA"`
	VoteScoreB   int    `json:"voteScoreB"`
	VoteResult   string `json:"voteResult"`

	// 5G attack.
	Attack5GActive   bool  `json:"attack5gActive"`
	Attack5GTimeLeft int   `json:"attack5gTimeLeft"`
	Attack5GZones    []Pos `json:"attack5gZones"`

	// Boss fight.
	BossActive bool `json:"bossActive"`
	BossHP     int  `json:"bossHP"`
	BossMaxHP  int  `json:"bossMaxHP"`
}

// PlayerState is the wire projection of a Player.
type PlayerState struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Status       int    `json:"status"`
	IsIrradiated bool   `json:"isIrradiated"`
	HeadID       int    `json:"headId"`
	BodyID       int    `json:"bodyId"`
	Message      string `json:"message"`
	Voted        bool   `json:"voted"`
}
