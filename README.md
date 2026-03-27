Votan: Real-Time Stream Engagement Engine
Votan is a high-performance, real-time backend engine written in Go that transforms standard live stream chats (e.g., YouTube) into an interactive, multi-user RPG environment overlaid directly onto the video feed.

![example in real stream](exampleStream.png)

The system handles real-time player movement, combat mechanics, spatial events, and dynamic OBS Studio scene manipulation without requiring viewers to install any third-party software—everything is driven by chat commands.

Architecture & Core Principles
This project was built with Clean Architecture and SOLID principles in mind to ensure scalability and testability:

Concurrency & State Management: Utilizes Go's goroutines and channels to handle continuous tick-rate processing. Shared resources (like OBS animation generations) are protected via sync.Mutex to prevent race conditions and goroutine leaks.

Decoupled Logic: External integrations (Database, OBS API) are isolated from the core game loop. This allows the engine to run in a "headless" mode for testing or development without requiring a live OBS connection.

Dynamic Asset Discovery: The engine features an automated asset scanner that detects character skins (Head/Body) at runtime, eliminating the need for hardcoded limits and allowing for "hot" updates to the visual library.

Event-Driven WebSocket Server: Employs full-duplex WebSocket communication to broadcast game state to the frontend renderer and admin dashboard simultaneously with minimal latency.

![example admin panel](exampleAdminPanel.png)

Key Features
Real-Time Game Loop: A custom tick-based engine processing grid movement, collision detection, and spatial debuffs (e.g., "5G radiation zones").

OBS Studio Automation: Directly controls OBS via the OBS-WebSocket API. Capable of manipulating scene items, triggering media playback, and applying complex visual filters (e.g., fade opacity) programmatically.

Persistent Player Data: SQLite integration to track player coordinates, progression status ("Baptized"), and cosmetic equipment (Skins).

Demiurge Admin Dashboard: A dedicated web-based control panel for the streamer to trigger boss events, initiate global voting, apply spatial attacks, and impersonate users for moderation or storytelling.

Tech Stack
Backend: Go (Golang) 1.21+

Networking: gorilla/websocket

Database: github.com/glebarez/go-sqlite (Pure Go, CGO-free for easy cross-compilation)

Integration: OBS WebSocket API (andreykaipov/goobs)

Frontend: Vanilla JavaScript, HTML5 Canvas (Overlay), DOM (Admin)

Getting Started
Prerequisites
Go 1.21 or higher

OBS Studio (with WebSocket Server enabled on port 4455)

Installation
Clone the repository:

Bash
git clone https://github.com/ArthurDovis/Votan.git
cd votan
Run the server (it will prompt for configuration or use .env):

Bash
go run cmd/server/main.go
Integration Setup:

Add a new Browser Source in OBS pointing to http://localhost:8080.

Open http://localhost:8080/admin.html to access the Demiurge Control Panel of press button for open it.

How it Works (Client Side)
Viewers interact by typing commands into the live chat:

Movement: !r5 (Right 5), !l2 (Left 2), !u10 (Up 10), !d.

Combat: !hit damages the current boss during events.

Customization: !h1b2 changes head and body skins (requires Admin blessing/Baptism).

Future Roadmap
[x] Unit Testing: Implemented comprehensive logic testing for movement, collisions, and voting.

[ ] Hardware Integration: Adding Bluetooth HID support for controlling player entities via Flipper Zero directly from the Admin Panel.

[ ] Cross-Platform Chat: Expanding input parsing to support Twitch and Kick concurrently with YouTube.

Designed and developed for live stream engagement.