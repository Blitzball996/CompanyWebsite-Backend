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

## 隐私与地理定位 / Privacy & Geolocation

- 后端用离线库 **ip2region** 把 IP 解析到 **国家 / 省 / 市 + 运营商**。这是 IP 定位**可靠的最高精度（市/区县级）**，
  无法精确到街道门牌——真正到街道只有访客在浏览器手动授权 GPS 才行。
- `STORE_IP` 环境变量控制是否**存储明文 IP**：
  - `STORE_IP=true`（默认）：记录真实 IP，面板可见。属于收集个人信息，请确保符合《个人信息保护法》/GDPR 并在隐私政策告知。
  - `STORE_IP=false`：不存明文 IP（仅地区统计），隐私更安全。
- 访客 ID 始终是每日轮换的加盐哈希，不可逆推回 IP。

## 博客 / Blog

后端内置双语博客。面板「博客管理」标签可写/改/删文章（中英文双语字段），前端 `blog.html` 通过公开接口 `GET /api/posts` 展示「最新动态」。

## API

> 新增接口：`/api/stats/regions`（地区）、`/api/stats/visitors`（访客列表+地理）、`/api/stats/journey?id=`（单访客轨迹）、
> `/api/posts`（公开读博客）、`/api/admin/posts`（Basic Auth 增删改）。


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
