package engine

import (
	"time"
)

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

	Status       int
	IsIrradiated bool
	HeadID       int
	BodyID       int

	IrradiatedUntil time.Time
	LastMessage     string
	MessageTime     time.Time

	// Whether the player has acted (moved) during the current vote.
	Voted bool
}

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

type PlayerState struct {
	ID           string `json:"id"` // exposed so the admin panel can target players
	Name         string `json:"name"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Status       int    `json:"status"`
	IsIrradiated bool   `json:"isIrradiated"`
	HeadID       int    `json:"headId"`
	BodyID       int    `json:"bodyId"`
	Message      string `json:"message"`

	// Vote status, forwarded to the frontend.
	Voted bool `json:"voted"`
}
