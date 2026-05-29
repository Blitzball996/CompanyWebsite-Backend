# Blitzball Labs — Analytics Backend

Self-hosted, privacy-friendly web analytics for the [CompanyWebsite](../CompanyWebsite).
No cookies, no third party, no PII stored (visitor IDs are salted daily-rotating hashes).

**Stack:** Go (chi + pgx) · PostgreSQL · self-built Chart.js dashboard · Docker.

## Quick start (Docker — recommended)

Requires **Docker Desktop**.

```bash
copy .env.example .env        # then edit DASHBOARD_PASS + VISITOR_SALT
run.bat                       # or: docker compose up --build
```

- Backend / collect API: <http://localhost:8090>
- Dashboard: <http://localhost:8090/dashboard> (HTTP Basic Auth — user/pass from `.env`)

Postgres data persists in `./pgdata/`.

## Quick start (without Docker)

Requires **Go 1.22+** and a running **PostgreSQL**.

```bash
set DATABASE_URL=postgres://analytics:analytics@localhost:5432/analytics?sslmode=disable
set DASHBOARD_USER=admin
set DASHBOARD_PASS=your-pass
set VISITOR_SALT=some-random-string
go mod tidy
go run ./cmd/server
```

## Connect your website

Add this before `</body>` in each page (`index.html`, `blitz.html`, `closecrab.html`):

```html
<script defer src="http://localhost:8090/analytics.js"
        data-endpoint="http://localhost:8090/api/collect"></script>
```

In production, replace `localhost:8090` with your deployed backend URL, and add that
origin to `ALLOWED_ORIGINS` in `.env`.

### Custom events

```js
window.bzTrack('signup_click', { plan: 'pro' });
```

Auto-tracked already: pageviews, scroll depth, time-on-page, GitHub/download/outbound
clicks, and language-switcher clicks.

## API

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/api/collect` | CORS | Ingest a pageview/event (called by `analytics.js`) |
| GET | `/dashboard` | Basic | Visualization UI |
| GET | `/api/stats/summary?days=7` | Basic | PV/UV/sessions/bounce/live cards |
| GET | `/api/stats/trend` | Basic | PV/UV by day |
| GET | `/api/stats/pages` | Basic | Top pages |
| GET | `/api/stats/referrers` | Basic | Traffic sources |
| GET | `/api/stats/devices` `/browsers` `/os` `/langs` | Basic | Distributions |
| GET | `/api/stats/events` | Basic | Custom event counts |
| GET | `/health` | — | Liveness |

## Privacy

`visitor_id = sha256(salt + day + ip + user_agent)[:16]`. Raw IPs are never stored and
the hash rotates daily, so visitors cannot be tracked across days or de-anonymized.

## Layout

```
cmd/server         entrypoint + routing
internal/collect   ingest endpoint + UA parsing
internal/stats     aggregation SQL → JSON
internal/dashboard dashboard page + Basic Auth
internal/db        pool + migrations
internal/config    env config
web/analytics.js   client script served at /analytics.js
```
