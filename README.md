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

## 指标含义 / Metrics explained

| 指标 | 英文 | 含义 |
|---|---|---|
| **PV** | Page View | 页面被打开的总次数，刷新一次算一次。 |
| **UV** | Unique Visitor | 独立访客数。同一人当天多次访问只算 1 个（按 `IP+UA` 的每日加盐哈希去重）。 |
| **会话 Sessions** | Sessions | 一次连续浏览算一个会话；关闭标签页或长时间无操作后重新访问算新会话。 |
| **平均停留** | Avg Duration | 每个页面被停留的平均时长。 |
| **跳出率** | Bounce Rate | 只看了一个页面就离开的会话占比，越低越好。 |
| **当前在线** | Live | 最近 5 分钟内仍在访问的人数。 |

## 博客 / Blog

后端内置双语博客。面板「博客管理」标签可写/改/删文章（中英文双语字段），前端 `blog.html` 通过公开接口 `GET /api/posts` 展示「最新动态」。


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
