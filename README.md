# rom-server-poc

A clean-room **Go** implementation of the **ROM: Golden Age** game server — the
raw-TCP world server the retail client connects to on `:17701`, a real,
structured simulation backed by Postgres.

The wire format, cipher, and message catalog are derived from the game client's
protocol metadata; this repo is the production-shaped server built on top of them.

## Stack

| Concern | Choice |
|---|---|
| Language | Go (single static binary, goroutine-per-connection) |
| Wire codec | Generated from the protocol metadata by `cmd/gen` (no hand-written structs) |
| Cipher | Rabbit (RFC 4503) + AES-128-ECB key derivation, ported and golden-tested |
| Persistence | Postgres via `pgx/v5` (`pgxpool`) |
| Migrations | `goose` (plain SQL, `migrations/`) |
| Auth | Delegated to `rom-api` region service — no credentials checked here |

## Architecture

```
cmd/server        TCP listener; handshake per connection; graceful shutdown
cmd/gen           catalog codegen: metadata JSON -> internal/protocol/catalog_gen.go
internal/
  config          environment configuration
  transport       framing, Rabbit cipher, AES key derivation, handshake
  protocol        message catalog + codec (encode/decode); catalog_gen.go
  auth            rom-api region session validation
  world           authoritative in-memory state; single-owner tick loop
  persist         Store interface + models
  persist/postgres  pgx-backed Store
  game            per-connection dispatch (version check, login, ...)
migrations        goose SQL migrations
```

**Concurrency model.** Each connection has its own goroutine for I/O. All mutable
game state is owned by a single `world.World.Run` goroutine; other goroutines
mutate it only by enqueuing commands (`Join`/`Leave`/`Move`), so no locks guard
game state. The DB is off the hot path: hot state (position, HP) lives in memory
and is snapshotted to Postgres on events and periodically — never per packet.

**Codec.** `cmd/gen` reads the protocol metadata (`messages_typed.json`,
`rom_dump.json`, `rom_struct_bases.json`), resolves every message to its full
base-class-first field layout plus the nested structs it reaches, and emits
`internal/protocol/catalog_gen.go`. The runtime codec
(`internal/protocol/codec.go`) is the single source of truth for the wire format
at runtime. **Opcodes and the static AES key rotate on every client update** —
rerun `make gen CATALOG_DIR=<dir>` against the updated metadata and update
`transport.StaticKey`.

## Authentication flow

The game server never checks credentials. The client authenticates against
`rom-api` (auth + region over HTTPS) and receives a `sessionKey` and
`accountCode` (userCode). On the game socket the client sends
`C2S_SessionAuthLogin{m_accountCode, m_sessionKey}`; the server calls the region
service `GET /internal/sessions/:sessionKey` (or `POST .../consume` for
single-use) and accepts only if the returned `userCode` matches `m_accountCode`
and the session's world matches this instance's `ROM_WORLD_ID`.

## Running

```bash
make db-up          # start Postgres (docker compose)
make migrate-up     # apply migrations (needs `goose` on PATH)
make run            # start the game server on :17701
```

Point the client's raw-TCP game socket at this server (bare IP, no TLS) while
letting its HTTPS auth traffic reach `rom-api`. Configuration is via environment
variables — see `.env.example`.

## Development

```bash
make gen     # regenerate the protocol catalog after a client update
make test    # transport (golden Rabbit vectors) + protocol round-trip
make build   # build ./bin/rom-server
make lint    # golangci-lint (if installed)
```

The Rabbit cipher is regression-tested against production-verified golden
keystreams; the codec is tested for encode/decode symmetry.

### Toolchain note

`pgx/v5` requires Go ≥ 1.25. With an older local `go`, keep `GOTOOLCHAIN=auto`
(the default) so Go uses the version pinned in `go.mod`.

## Status

Scaffold. Implemented: transport (handshake + cipher + framing), full message
catalog + codec, region-backed authentication, the world tick-loop skeleton, and
Postgres persistence for characters. Next: character list / creation, enter-world,
movement, and the map/world simulation.
