# Votan: Real-Time Stream Engagement Engine

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![WebSocket](https://img.shields.io/badge/WebSocket-Gorilla-blue)
![OBS Studio](https://img.shields.io/badge/Integration-OBS_WebSocket-black)
![SQLite](https://img.shields.io/badge/Database-SQLite-003B57?style=flat&logo=sqlite)

Votan is a high-performance, real-time backend engine written in Go that transforms standard live stream chats (e.g., YouTube) into an interactive, multi-user RPG environment overlaid directly onto the video feed.

The system handles real-time player movement, combat mechanics, spatial events, and dynamic OBS Studio scene manipulation without requiring viewers to install any third-party software—everything is driven by chat commands.



## Architecture & Core Principles

This project was built with **Clean Architecture** and **SOLID** principles in mind to ensure scalability and testability:

* **Concurrency & State Management:** Utilizes Go's goroutines and channels to handle continuous tick-rate processing. Shared resources (like OBS animation generations) are protected via `sync.Mutex` to prevent race conditions and goroutine leaks.
* **Dependency Injection:** External integrations (Database, OBS API) are abstracted behind interfaces (`Storage`, `MediaController`). This decouples the game engine from specific implementations and allows for comprehensive unit testing via mocking.
* **Event-Driven WebSocket Server:** Employs full-duplex WebSocket communication to broadcast game state to the frontend renderer and admin dashboard simultaneously with minimal latency.

## Key Features

* **Real-Time Game Loop:** A custom tick-based engine processing grid movement, collision detection, and spatial debuffs (e.g., "5G zones").
* **OBS Studio Automation:** Directly controls the streamer's OBS via the OBS-WebSocket API. Capable of manipulating scene items, triggering media playback, and applying complex visual filters (e.g., fade opacity) programmatically based on game state.
* **Persistent Player Data:** SQLite integration to track player coordinates, progression status ("Baptized"), and cosmetic equipment (Skins).
* **Demiurge Admin Dashboard:** A dedicated web-based control panel for the streamer to trigger boss events, initiate global voting, apply spatial attacks, and manage users.

## Tech Stack

* **Backend:** Go (Golang)
* **Networking:** `gorilla/websocket`
* **Database:** SQLite (`mattn/go-sqlite3`)
* **Integration:** OBS WebSocket API (`andreykaipov/goobs`)
* **Frontend (Overlay & Admin):** Vanilla JavaScript, HTML5 Canvas/DOM

## Project Structure

\`\`\`text
.
├── cmd/
│   └── server/          # Application entry point
├── internal/
│   ├── config/          # Environment and game parameters
│   ├── engine/          # Core game loop, logic, and interfaces
│   ├── obs/             # OBS WebSocket client implementation
│   └── db/              # SQLite database implementation
├── web/
│   ├── public/          # Static assets (HTML, CSS, JS) for overlay and admin
│   └── assets/          # Game sprites and media
└── README.md
\`\`\`

## Getting Started

### Prerequisites
* Go 1.21 or higher
* OBS Studio (with WebSocket Server enabled on port 4455)

### Installation

1.  Clone the repository:
    \`\`\`bash
    git clone https://github.com/yourusername/votan.git
    cd votan
    \`\`\`

2.  Set up the environment variables. Create a `.env` file in the root directory:
    \`\`\`env
    OBS_ADDR=localhost:4455
    OBS_PASS=your_obs_websocket_password
    ADMIN_SECRET=your_secure_admin_token
    \`\`\`

3.  Run the server:
    \`\`\`bash
    go run cmd/server/main.go
    \`\`\`

4.  **Integration Setup:**
    * Add a new "Browser Source" in OBS pointing to `http://localhost:8080`.
    * Open `http://localhost:8080/admin.html` in your browser to access the control panel.

## How it Works (Client Side)

Viewers interact by typing commands into the live chat:
* Movement: `!r5` (Right 5 units), `!l2` (Left 2 units), `!u`, `!d`.
* Combat: `!hit` damages the current boss.
* Customization: `!h1b2` changes the player's head and body skin (requires Admin blessing).

## 🔮 Future Roadmap

* [ ] **Unit Testing:** Implementing Table-Driven Tests with mocked `MediaController` and `Storage`.
* [ ] **Hardware Integration:** Adding Bluetooth HID support for controlling player entities via Flipper Zero directly from the Admin Panel.
* [ ] **Cross-Platform Chat:** Expanding input parsing to support Twitch and Kick concurrently with YouTube.

---
*Designed and developed for robust live stream engagement.*