# Golang CRUD

A small REST API in Go ([Gin](https://github.com/gin-gonic/gin) + MySQL):

- **Auth**: sign up, log in (JWT access + refresh tokens), refresh tokens.
- **Albums**: create, read, update, delete albums (title, artist, price); requires an access token.
- **Weather**: current weather for a coordinate, proxied from [weatherapi.com](https://www.weatherapi.com/); requires an access token.

## Run with Docker (recommended)

```bash
cp .env.example .env    # then fill in the empty secrets in .env
docker compose up --build
```

The API listens on http://localhost:8080. MySQL is published on `127.0.0.1:3307` for local tools. A one-shot `migrate` service applies database migrations before the API starts; if it fails, `docker compose up` stops and `docker compose logs migrate` shows why. Outside Compose, the API applies migrations at startup (`MIGRATE_ON_START=true`), or run `go-crud migrate`, which needs only the `DB_*`/`DATABASE_ADDR` settings.

## Run locally

Requires Go 1.26.10+ (the version in `go.mod`) and a MySQL 8 server.

```bash
docker compose up -d mysql
set -a; . ./.env; set +a
DATABASE_ADDR=127.0.0.1:3307 go run .
```

## Configuration

All configuration comes from environment variables. The app refuses to start if a required value is missing.

| Variable | Required | Default | Notes |
|---|---|---|---|
| `DB_USERNAME`, `DB_PASSWORD` | yes | | MySQL credentials |
| `DATABASE_ADDR` | | `127.0.0.1:3306` | `host:port` |
| `DB_DATABASE` | | `recordings` | |
| `JWT_SECRET` | yes | | ≥ 32 bytes, e.g. `openssl rand -base64 48` |
| `JWT_ISSUER` | | `go-crud` | |
| `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` | | `15m` / `12h` | Go durations |
| `BCRYPT_COST` | | `12` | 10–31 |
| `AUTH_RATE_LIMIT_PER_MINUTE` | | `10` | Requests per client IP per minute to `/api/auth/*`; over the limit answers 429 |
| `TRUSTED_PROXIES` | | | Comma-separated proxy IPs/CIDRs whose `X-Forwarded-For` is trusted. **Set it behind a reverse proxy or load balancer**, or all clients share the proxy's rate limit |
| `REFRESH_REUSE_GRACE` | | `10s` | How long a just-used refresh token may be retried (lost response); the retry gets the same new token. Whole seconds, at most `1m`; `0s` = strictly single-use |
| `HTTP_ADDR` | | `:8080` | |
| `MIGRATE_ON_START` | | `true` | |
| `WEATHER_API_KEY` | | | Without it `/api/weather` answers 503 |
| `WEATHER_API_BASE_URL` | | `https://api.weatherapi.com/v1` | |
| `WEATHER_SIGNING_SECRET`, `WEATHER_SIGNING_CLIENT_ID` | | | Optional HMAC request signing; set both or neither |

## API

Protected routes need `Authorization: Bearer <Token>` using the **access** token from login. The refresh token is only accepted by `/api/auth/refresh`, and **each refresh token works once**: always store the new pair it returns. A retry within `REFRESH_REUSE_GRACE` (default 10 s, whole seconds) of the first use, for example after a lost response, returns the same new refresh token again rather than a different one. Presenting a refresh token later than that is treated as theft and ends all of that user's sessions: the user's session version is incremented, so every refresh and access token issued before is rejected (access tokens within 5 s, the session cache lifetime), and the user must log in again. Tokens are bound to the account ID, so they never carry over to a new account registered with the same email. Expired or already-revoked refresh tokens are simply rejected (401) without revoking anything. A failed refresh (5xx) does not consume the token and can be retried.

`/api/auth/*` is rate-limited per client IP (`AUTH_RATE_LIMIT_PER_MINUTE`, 429 with `Retry-After` when exceeded). The limit is kept in memory per app instance. Behind a reverse proxy set `TRUSTED_PROXIES`; with Docker Desktop port forwarding, all local requests appear to come from the Docker gateway.

| Method | Path | Success | Errors |
|---|---|---|---|
| POST | `/api/auth/signup` `{"name","email","password"}` | 201 `{"Message": ...}` | 400 invalid input (password 8–72 bytes), 409 email taken |
| POST | `/api/auth/login` `{"email","password"}` | 200 `{"Token","RefreshToken"}` | 400, 401 |
| POST | `/api/auth/refresh` `{"RefreshToken"}` | 200 `{"Token","RefreshToken"}` | 400, 401 (invalid, expired or already used) |
| GET | `/api/albums` | 200 `[album...]` (`[]` when empty) | 401 |
| GET | `/api/albums/:id` | 200 `album` | 400 bad id, 404 |
| GET | `/api/albums/artist/:artist` | 200 `[album...]` | 401 |
| POST | `/api/albums` `{"title","artist","price"}` | 201 new id | 400 invalid (price 0–999.99) |
| PUT | `/api/albums/:id` `{"title","artist","price"}` | 200 id | 400, 404 |
| DELETE | `/api/albums/:id` | 200 id | 400, 404 |
| GET | `/api/weather/:lat/:long` | 200 weather | 400 bad coordinates, 502 upstream error, 503 not configured |
| GET | `/healthz` | 200 `{"status":"ok"}` | 503 database unreachable |

Emails are matched case-insensitively but accent-sensitively (`jose@…` and `josé@…` are different accounts). Request bodies must be a single JSON object; a field of the wrong type is reported by name (e.g. `price must be a number`).

Album responses look like `{"ID": 1, "Title": "Blue Train", "Artist": "John Coltrane", "Price": 56.99}`. Errors look like `{"message": "..."}`.

A Postman collection is in [`postman/`](postman/go-crud.postman_collection.json); its login and refresh requests store the tokens in collection variables.

## Tests

```bash
go test -race ./...
```

Integration tests run the repositories and the whole HTTP API against a real MySQL. **They delete every row in the `user`, `album` and `refresh_token` tables**, so they only run when `INTEGRATION_ALLOW_DB_RESET=1` is set; point them at a disposable database:

```bash
docker compose up -d mysql
set -a; . ./.env; set +a
INTEGRATION_ALLOW_DB_RESET=1 DATABASE_ADDR=127.0.0.1:3307 go test -race -tags integration ./integration/...
```

## Migrations

SQL migrations live in [`migrations/`](migrations) ([golang-migrate](https://github.com/golang-migrate/migrate) format) and are embedded in the binary.

`1731542400_user_email_unique` adds a unique index on `user.email`. If an existing database already contains duplicate emails, it fails and the app will not start. Find duplicates with:

```sql
SELECT email, COUNT(*) FROM user GROUP BY email HAVING COUNT(*) > 1;
```

Remove the duplicates, then mark the failed migration as not applied and run `docker compose up` again: `migrate -path migrations -database "mysql://USER:PASS@tcp(HOST:PORT)/recordings" force 1481574547`.
