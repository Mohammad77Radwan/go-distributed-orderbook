# Quantum Distributed Order Book

A realtime distributed order book simulation platform built with Go, Redis Pub/Sub, WebSocket fan-out, and a live trading-style dashboard.

## At A Glance

| Capability | Implementation |
|---|---|
| Matching + book maintenance | In-memory concurrent order book (`sync.RWMutex`) |
| Priority model | Price-time priority for bids/asks |
| Event transport | Redis Pub/Sub channel `market_updates` |
| Client delivery | WebSocket hub with safe concurrent writes |
| UI | Svelte realtime dashboard with derived market metrics |
| Ops basics | Health endpoint, graceful shutdown, reconnect behavior |

## What The System Does

The system continuously simulates market order flow and streams snapshots of top-of-book state to connected browser clients. The frontend renders that stream as a trading cockpit: spread, mid price, liquidity by side, depth map, ladder view, and flow/tape-style reactions.

## Architecture

### Component Topology

```mermaid
flowchart LR
		SIM[Order Simulator\nmain.go] --> ENG[Matching Engine\nengine/matcher.go]
		ENG --> PUB[Snapshot Publisher\nengine/pubsub.go]
		PUB --> REDIS[(Redis Pub/Sub\nmarket_updates)]
		REDIS --> GATE[WebSocket Gateway\ngateway/ws.go]
		GATE --> UI[Realtime Dashboard\nfrontend/src/routes/+page.svelte]
```

### Runtime Interaction Sequence

```mermaid
sequenceDiagram
		participant S as Simulator
		participant E as Engine
		participant R as Redis
		participant G as WS Gateway
		participant C as Browser Client

		loop every 10ms
				S->>E: AddOrder(order)
				E->>E: Update + sort order book
				E->>R: PublishSnapshot(snapshot)
				R-->>G: market_updates message
				G-->>C: WebSocket snapshot payload
				C->>C: Recompute derived metrics + render
		end
```

## Core Technical Highlights

### Backend
- Concurrency-safe order book operations with lock granularity (`RWMutex`).
- Stable sorting for deterministic tie-breaking:
	- Bids sorted by descending price, then oldest first.
	- Asks sorted by ascending price, then oldest first.
- Snapshot capping (top 10 levels per side) to keep payloads bounded.
- Redis-backed event decoupling between engine and gateway.
- WebSocket hub model with per-client write mutexes for safe fan-out.
- Process lifecycle handling with cancellation and graceful server shutdown.

### Frontend
- Reactive WebSocket store for realtime state updates.
- Socket reconnect loop for resilience.
- Derived analytics from stream snapshots:
	- best bid / best ask
	- spread + mid price
	- side liquidity totals
	- imbalance
- Visual modules for depth, ladder, and tape semantics.

## Repository Layout

```text
.
├── engine/
│   ├── matcher.go      # Book mutation, sorting, snapshot generation
│   ├── pubsub.go       # Engine wrapper + Redis snapshot publisher
│   └── types.go        # Domain types
├── gateway/
│   └── ws.go           # Redis bridge + websocket client hub
├── frontend/
│   ├── src/lib/websocket.ts      # Realtime socket store
│   └── src/routes/+page.svelte   # Dashboard screen
├── main.go             # Wiring, simulator, HTTP server
└── docker-compose.yml  # Redis runtime
```

## Quick Start

### Prerequisites
- Go 1.21+
- Redis 7+
- Node.js 20+ (for frontend build/dev)

### 1) Start Redis

```bash
docker compose up -d redis
```

or:

```bash
redis-server --port 6379
```

### 2) Run backend

```bash
go run .
```

Backend listens on `:8080`.

### 3) Frontend development mode

```bash
cd frontend
npm install
npm run dev
```

### 4) Build frontend for static serving via backend

```bash
cd frontend
npm run build
cd ..
go run .
```

`main.go` serves static assets from `frontend/build` when present.

## API Surface

- `GET /healthz` -> readiness probe (`ok`)
- `GET /ws` -> websocket stream of `OrderBookSnapshot`

### Snapshot Payload

```json
{
	"bids": [
		{
			"id": "1712234567890",
			"type": "buy",
			"price": 100.12,
			"quantity": 540,
			"createdAt": "2026-04-03T10:20:30Z"
		}
	],
	"asks": [],
	"timestamp": "2026-04-03T10:20:30Z"
}
```

## Design Notes

### Why snapshots over diffs?
- Stateless client rendering model.
- Lower cognitive overhead for correctness.
- Easier recovery after reconnect.

### Why Redis in the middle?
- Clean separation between compute and delivery planes.
- Future-ready for multi-gateway fan-out.
- Operationally simple and familiar.

### Why in-memory book?
- Lowest-latency path for simulation workloads.
- Straightforward deterministic behavior.
- Easy extension to persisted event pipelines later.

## Reliability And Operations

- Graceful shutdown path using `signal.NotifyContext`.
- Read deadlines + pong handling in websocket read pump.
- Client cleanup on write/read failure.
- Frontend reconnect loop on socket close.

## Validation Commands

```bash
go test ./...
```

```bash
cd frontend && npm run check
```

## Roadmap

- Add external order ingestion API (`REST`/`gRPC`).
- Add benchmark suite with p95/p99 publish-to-render latency.
- Add multi-symbol channels and partitioned routing.
- Add deterministic replay mode from persisted snapshots/events.
- Add observability metrics and tracing for end-to-end flow.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
