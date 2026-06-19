# CoLink Server

Backend API server and WebSocket relay for CoLink.

**Tech stack:** Go 1.24 · Gin · GORM · PostgreSQL 16 · Gorilla WebSocket · golang-jwt · Docker

## Development

```sh
docker compose -f docker-compose.dev.yml up -d --build
```

To use a specific `.env` file:

```sh
docker compose --env-file .env -f docker-compose.dev.yml up -d --build
```

Note: when the same variable exists in both the terminal environment and the `.env` file, the terminal value takes precedence. Use `--env-file` to force file values.

## Production

```sh
cp .env.example .env   # fill in required variables
docker compose up -d --build
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `JWT_SECRET` | **required** | HS256 signing secret |
| `SERVER_MODE` | `debug` | Gin mode (`debug` / `release`) |
| `SERVER_PORT` | `8080` | HTTP listen port |
| `DATABASE_HOST` | `localhost` | PostgreSQL host |
| `DATABASE_PORT` | `5432` | PostgreSQL port |
| `DATABASE_USER` | `colink` | PostgreSQL user |
| `DATABASE_PASSWORD` | *(empty)* | PostgreSQL password |
| `DATABASE_DBNAME` | `colink` | PostgreSQL database name |
| `DATABASE_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `JWT_ACCESS_TTL` | `15m` | Access token TTL |
| `JWT_REFRESH_TTL` | `720h` | Refresh token TTL |
| `WS_TICKET_TTL` | `30s` | WebSocket ticket TTL |
| `UPDATE_CHECK_INTERVAL` | `30m` | GitHub release check interval |
| `UPDATE_STORAGE_PATH` | `./data/updates` | Update asset storage path |

The server does not persist messages, files, or clipboard content — it only relays WebSocket frames between authenticated devices belonging to the same user.
