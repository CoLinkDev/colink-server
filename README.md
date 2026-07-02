# CoLink Server

Backend API server, WebSocket relay, and update service for CoLink.

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
# Get a Github token at https://github.com/settings/personal-access-tokens, remember to enable the read:packages permission for this token
# `docker login ghcr.io` via this token

mkdir -p colink-server/deploy/nginx
cd colink-server

curl -fsSLO https://raw.githubusercontent.com/CoLinkDev/colink-server/master/.env.example
curl -fsSLO https://raw.githubusercontent.com/CoLinkDev/colink-server/master/docker-compose.yml
curl -fsSLo deploy/nginx/default.conf https://raw.githubusercontent.com/CoLinkDev/colink-server/master/deploy/nginx/default.conf

cp .env.example .env

# Edit .env and fill in required variables

docker compose pull
docker compose up -d
```

## Environment Variables

See `.env.example`. CoLink binaries read `COLINK_*` variables directly, and Docker Compose passes the same names into containers.

## Services

| Service | Responsibility |
|---|---|
| `nginx` | Public entrypoint and path routing |
| `server` | Auth, account, device API, WebSocket tickets, WebSocket relay, frontend fallback |
| `update` | GitHub release checks, cached update metadata, update asset downloads |
| `postgres` | PostgreSQL storage |

The server does not persist messages, files, or clipboard content — it only relays WebSocket frames between authenticated devices belonging to the same user.
