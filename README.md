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
entity, `PlayerState` is the JSON the frontend gets.

## Design notes

A few decisions worth calling out:

- **Single self-contained binary.** The overlay and admin UI are embedded into
  the executable with `//go:embed`, so a release is one file — download it, run
  it, done. No assets to copy, no install step.

- **Dependency inversion.** The engine depends on two small interfaces it owns
  itself — `Store` (persistence) and `Scene` (OBS) — not on concrete types. The
  SQLite and OBS packages implement those interfaces; the engine never imports
  them. A `nil` dependency is normalised to a no-op implementation, so the game
  runs fully headless (which is exactly how the tests drive it) without a single
  nil check on the hot path.

- **Persistence never blocks the loop.** Writes are fire-and-forget: the store
  hands them to a single background goroutine over a buffered channel and
  returns immediately, so a slow disk can never stall a tick. That one goroutine
  also serialises every write, which keeps SQLite happy. The in-memory state is
  authoritative; the database is a convenience for surviving restarts. Under
  extreme load the queue sheds writes (and logs it) rather than blocking.

- **Restart-safe board.** On startup every persisted user is loaded once into an
  in-memory cache; players whose saved tile is still free are placed back on the
  board, so a restart preserves positions, skins and "baptism" status. First
  contact from a returning viewer is served from that cache — no database read
  in the request path.

- **Graceful shutdown.** `main` builds a `signal.NotifyContext`; Ctrl+C or
  SIGTERM cancels it, which unwinds everything in order — the HTTP server drains
  via `Shutdown`, the game loop and chat reader return on the cancelled context,
  live WebSocket connections close, and finally the queued DB writes are flushed
  before the database is closed.

- **OBS fade cancellation.** OBS fades run in their own goroutines, and a global
  generation counter lets any new effect cancel a stale fade still in flight, so
  two overlapping admin actions can't leave the scene stuck half-faded.

![admin dashboard](exampleAdminPanel.png)

## Stack

- Go 1.25
- `gorilla/websocket` for the realtime layer
- `glebarez/go-sqlite` — pure-Go, CGO-free SQLite, so it cross-compiles cleanly
- `andreykaipov/goobs` for the OBS WebSocket API
- `log/slog` for structured logging
- `//go:embed` for a single-file build; vanilla JS + HTML5 canvas for the UI

## Running it

### Option A — download a release (no Go needed)

Grab the binary for your OS from [Releases](https://github.com/uinjad/Votan/releases):

- **Windows:** download `Votan_*_windows_amd64.exe` and double-click it.
- **macOS:** download `Votan_*_darwin_arm64` (Apple Silicon) or
  `Votan_*_darwin_amd64` (Intel), then in a terminal:
  ```
  chmod +x Votan_*_darwin_*
  xattr -d com.apple.quarantine Votan_*_darwin_*   # unsigned indie binary
  ./Votan_*_darwin_*
  ```

The program opens the admin panel automatically. Add a Browser Source in OBS
pointing at `http://127.0.0.1:8080`.

### Option B — from source

Needs Go 1.25+.

```
git clone https://github.com/uinjad/Votan.git
cd Votan
make run
```

Configuration (stream id, OBS address/password, admin token) is entered in the
admin panel at `http://127.0.0.1:8080/admin.html`; the panel writes a local
`.env` for you (mode `0600`, gitignored). See `.env.example` for the full list.
The server binds to loopback by default; override with `LISTEN_ADDR` in `.env`.

## Make targets

```
make build   # self-contained binary for this machine
make run      # run without building
make test     # unit tests
make race     # tests under the race detector
make check    # gofmt + vet + race (what CI enforces)
make lint     # golangci-lint
make dist     # cross-compile Windows + macOS binaries into ./dist
make help     # full list
```

## Chat commands

Viewers move and act by typing in chat:

- `!r5` `!l2` `!u10` `!d` — move right / left / up / down N tiles
- `!hit` — damage the active boss during a boss event
- `!h1b2` — change head / body skin (needs an admin "baptism" first)

## Tests

The engine logic is covered by unit tests — movement and collision rules, the
vote tally, AFK cleanup, timed debuffs, command parsing, admin auth — plus a
race-detector test that hammers the command channel and state reads while the
loop runs. Everything runs against a headless game with no DB and no OBS:

```
go test -race ./...
```

## Releasing

Releases are automated. Push a SemVer tag and a GitHub Action cross-compiles the
single-file binaries for Windows and macOS, generates checksums, and publishes a
release with auto-generated notes:

```
git tag -a v1.0.0 -m "Votan v1.0.0"
git push origin v1.0.0
```

The version is baked into the binary (`-X main.version`) and logged at startup.

## Known limitations

This is built to run locally for a single streamer, and the security model
reflects that. The server **binds to loopback (`127.0.0.1`) by default**, so the
admin panel and the `/api/config` endpoint — which expose the local OBS and
admin secrets to the operator's own browser — are not reachable from the
network. The admin channel is gated by a shared token compared in constant time,
and the WebSocket accepts any origin (safe behind the loopback bind). Putting
this on the open internet would need real auth, proper origin checks, and a
config endpoint that never ships secrets to the client.

## Roadmap

- [x] Unit tests for movement, collisions and voting
- [x] Graceful shutdown, dependency injection, async persistence
- [x] Single-file build with embedded UI; automated cross-platform releases
- [ ] Localisation
- [ ] Twitch and Kick chat alongside YouTube

## License

[MIT](LICENSE)