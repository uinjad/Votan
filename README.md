# Votan

[![CI](https://github.com/uinjad/Votan/actions/workflows/ci.yml/badge.svg)](https://github.com/uinjad/Votan/actions/workflows/ci.yml)

Votan turns a live YouTube chat into a multiplayer RPG that runs as an overlay
on top of the stream. Viewers play by typing chat commands — nothing to
install, no extensions, no logins. It also drives OBS directly, so boss fights,
votes and visual effects fire on stream in real time.

The theme is a satirical, chat-driven game built around internet conspiracy
memes (5G, lizard people, and friends). The setting is a joke; the engine
isn't.

![overlay on a live stream](exampleStream.png)

## How it works

One Go process runs three things at once:

- a fixed 100 ms tick loop that owns all game state — movement, collisions,
  voting, timed events;
- a chat reader that pulls the YouTube live chat feed and pushes commands into
  a buffered channel;
- a WebSocket server that streams game state to the overlay and the admin
  dashboard ten times a second.

State sits behind a single `sync.RWMutex` and is only mutated from the tick
loop. The chat reader and the WebSocket clients never touch it directly — they
talk to the loop over channels. Each tick drains a length snapshot of the
command channel, which gives natural backpressure when a stream gets loud.

The domain model and the wire model are kept apart: `Player` is the in-memory
entity, `PlayerState` is the JSON the frontend gets. External integrations
(SQLite, OBS) live behind their own packages and are allowed to be nil — the
engine runs headless with no database and no OBS, which is exactly how the
tests drive it.

One detail I like: OBS fades run in their own goroutines, and a global
generation counter lets any new effect cancel a stale fade still in flight, so
two overlapping admin actions can't leave the scene stuck half-faded.

![admin dashboard](exampleAdminPanel.png)

## Stack

- Go 1.25
- `gorilla/websocket` for the realtime layer
- `glebarez/go-sqlite` — pure-Go, CGO-free SQLite, so it cross-compiles cleanly
- `andreykaipov/goobs` for the OBS WebSocket API
- vanilla JS + HTML5 canvas for the overlay and the dashboard

## Running it

Needs Go 1.25+ and, optionally, OBS Studio with its WebSocket server enabled.

```
git clone https://github.com/uinjad/Votan.git
cd Votan
make run
```

Add a Browser Source in OBS pointing at `http://localhost:8080`, and open
`http://localhost:8080/admin.html` for the control panel. Config (stream id,
OBS address/password, admin token) lives in `.env` and can be edited from the
panel.

```
make test      # run the engine tests
make build     # build the binary
make release   # build and package a zip with assets
```

## Chat commands

Viewers move and act by typing in chat:

- `!r5` `!l2` `!u10` `!d` — move right / left / up / down N tiles
- `!hit` — damage the active boss during a boss event
- `!h1b2` — change head / body skin (needs an admin "baptism" first)

## Tests

The engine logic is covered by unit tests — movement and collision rules, the
vote tally, AFK cleanup, and timed debuffs — all running against a headless
game with no DB and no OBS attached:

```
go test ./internal/engine/...
```

## Known limitations

This is built to run locally for a single streamer, and the security model
reflects that: the admin channel is gated by a shared token and the WebSocket
accepts any origin. That's fine on one machine, but it's not something you'd
put on the open internet without adding real auth and proper origin checks.

## Roadmap

- [x] Unit tests for movement, collisions and voting
- [ ] Flipper Zero / Bluetooth HID control of entities from the dashboard
- [ ] Twitch and Kick chat alongside YouTube