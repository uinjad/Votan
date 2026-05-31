package engine

import "testing"

func TestParseAction(t *testing.T) {
	cases := []struct {
		in            string
		dx, dy, steps int
	}{
		{"!r", 1, 0, 1},
		{"!l", -1, 0, 1},
		{"!u", 0, 1, 1},
		{"!d", 0, -1, 1},
		{"!r5", 1, 0, 5},
		{"!r999", 1, 0, 35}, // clamped to MaxStepsPerTurn
		{"!r-5", 0, 0, 0},   // negative count rejected
		{"!r0", 0, 0, 0},    // zero rejected
		{"!rabc", 0, 0, 0},  // garbage rejected
		{"!x", 0, 0, 0},     // unknown direction
		{"hello", 0, 0, 0},  // not a command
		{"!", 0, 0, 0},      // too short
	}
	for _, c := range cases {
		dx, dy, steps := parseAction(c.in)
		if dx != c.dx || dy != c.dy || steps != c.steps {
			t.Errorf("parseAction(%q) = (%d,%d,%d), want (%d,%d,%d)",
				c.in, dx, dy, steps, c.dx, c.dy, c.steps)
		}
	}
}

func TestAdminAuth(t *testing.T) {
	g := NewGame(nil, nil, Config{AdminSecret: "s3cret"})
	if !g.isAdmin("s3cret") {
		t.Error("correct secret should authenticate")
	}
	if g.isAdmin("wrong") {
		t.Error("wrong secret must not authenticate")
	}

	empty := NewGame(nil, nil, Config{AdminSecret: ""})
	if empty.isAdmin("") {
		t.Error("empty secret must never authenticate")
	}
}

func TestSkinChangeRequiresBaptism(t *testing.T) {
	g := NewGame(nil, nil, Config{MaxHeadID: 5, MaxBodyID: 5})

	// Unbaptized: skin must not change.
	g.processCommand(Command{PlayerID: "p1", PlayerName: "p1", Action: "!h2b3"})
	p := g.players["p1"]
	if p == nil {
		t.Fatal("player should have spawned")
	}
	if p.HeadID != 0 || p.BodyID != 0 {
		t.Errorf("unbaptized skin changed: head=%d body=%d", p.HeadID, p.BodyID)
	}

	// Baptized: skin changes within range.
	p.Status = 1
	g.processCommand(Command{PlayerID: "p1", PlayerName: "p1", Action: "!h2b3"})
	if p.HeadID != 2 || p.BodyID != 3 {
		t.Errorf("baptized skin not applied: head=%d body=%d", p.HeadID, p.BodyID)
	}

	// Out-of-range ids are ignored.
	g.processCommand(Command{PlayerID: "p1", PlayerName: "p1", Action: "!h99b99"})
	if p.HeadID != 2 || p.BodyID != 3 {
		t.Errorf("out-of-range skin applied: head=%d body=%d", p.HeadID, p.BodyID)
	}
}
