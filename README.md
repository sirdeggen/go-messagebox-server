# Go MessageBox Server

A Go reimplementation of the [BSV MessageBox Server](https://github.com/bsv-blockchain/message-box-server), providing peer-to-peer message storage and delivery with BRC-31 (Authrite) authentication and BRC-29 payment support.

## API Endpoints

All endpoints require BRC-31 authentication via the `go-bsv-middleware` auth middleware.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/sendMessage` | Send a message to one or more recipients' message boxes |
| POST | `/listMessages` | List messages from a specific message box |
| POST | `/acknowledgeMessage` | Acknowledge (delete) received messages |
| POST | `/registerDevice` | Register device for FCM push notifications |
| GET | `/devices` | List registered devices |
| POST | `/permissions/set` | Set message permission (block, allow, or require payment) |
| GET | `/permissions/get` | Get permission for a sender/box combination |
| GET | `/permissions/list` | List all permissions with pagination |
| GET | `/permissions/quote` | Get delivery price quote for recipient(s) |

## Architecture

```
cmd/server/         - Entry point, middleware wiring, CORS handler
internal/
  config/           - Environment variable loading
  db/               - SQLite database, migrations, queries
  handlers/         - HTTP route handlers
  logger/           - Toggleable structured logger
test-client/        - Jest integration tests (TypeScript)
```

### Key Dependencies

| Node.js Original | Go Equivalent |
|---|---|
| `@bsv/sdk` | `github.com/bsv-blockchain/go-sdk` |
| `@bsv/auth-express-middleware` | `github.com/bsv-blockchain/go-bsv-middleware` (auth) |
| `@bsv/payment-express-middleware` | `github.com/bsv-blockchain/go-bsv-middleware` (payment) |
| Express.js | `net/http` (Go 1.22+ routing) |
| Knex + MySQL | `database/sql` + SQLite (default) |
| — | `github.com/bsv-blockchain/go-wallet-toolbox` (wallet) |

## Database

Uses SQLite by default (zero-config). Tables:

- **messageBox** — Named message boxes per identity key
- **messages** — Stored messages with sender, recipient, body
- **message_permissions** — Per-sender or box-wide fee/block settings
- **server_fees** — Server-level delivery fees per box type
- **device_registrations** — FCM tokens for push notifications

## Wallet

Uses `go-wallet-toolbox` with local SQLite storage for production wallet functionality. The wallet provides:

- BRC-31 authentication via `go-bsv-middleware`
- BRC-29 payment processing
- Identity key derivation from `SERVER_PRIVATE_KEY`
- Automatic storage migration on startup

Network is configurable via `BSV_NETWORK` (mainnet/testnet).

## Differences from the Original

1. **Database**: Uses SQLite instead of MySQL by default (configurable via `DB_DRIVER`/`DB_SOURCE`)
2. **WebSockets**: Not yet implemented (HTTP API is fully compatible)
3. **Firebase/FCM**: Push notification sending is stubbed — device registration works, but actual FCM delivery requires Firebase Admin SDK integration

## Quick Start

```bash
cp .env.example .env
# Edit .env with your SERVER_PRIVATE_KEY (64-char hex)

go build -o messagebox-server ./cmd/server
./messagebox-server
```

## Docker

```bash
docker build -t messagebox-server .
docker run -p 8080:8080 -e SERVER_PRIVATE_KEY=your-hex-key messagebox-server
```

## Testing

### Go unit tests

```bash
go test ./...
```

### Jest integration tests

The `test-client/` directory contains 11 integration tests using `@bsv/message-box-client` and `@bsv/sdk` `ProtoWallet`, covering the full message lifecycle against a running server.

```bash
# Terminal 1: Start the server
SERVER_PRIVATE_KEY=$(openssl rand -hex 32) go run ./cmd/server

# Terminal 2: Run tests
cd test-client
npm install
npx jest --verbose
```

**Tests cover:**
- Send message (plaintext, JSON body, send-to-self)
- List messages (populated box, empty box)
- Acknowledge messages (valid, already-acknowledged, nonexistent)
- Input validation (empty recipient, empty body)
- Multiple messages in the same box

All tests use real BRC-31 AuthFetch authentication against the running server.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PRIVATE_KEY` | (required) | Hex-encoded private key for server identity |
| `NODE_ENV` | `development` | Environment (`development`, `production`) |
| `PORT` | `8080` (dev) / `3000` (prod) | HTTP listen port |
| `ROUTING_PREFIX` | `` | URL prefix for all routes |
| `DB_DRIVER` | `sqlite3` | Database driver |
| `DB_SOURCE` | `messagebox.db` | Database connection string |
| `BSV_NETWORK` | `mainnet` | BSV network (`mainnet`, `testnet`) |
| `ENABLE_WEBSOCKETS` | `true` | Enable WebSocket support (not yet implemented) |
