# CompanyWebsite-Backend

**English** | [中文](#中文)

---

## English

Go backend for **Blitzball Analytics** company website — customer accounts, license management, payment webhooks, and Team Mode leaderboard.

### Features

- **Customer Accounts** — Register, login, session management with bcrypt password hashing
- **License Management** — Auto-issue licenses via payment webhook, email serial-number lookup
- **Payment Integration** — Paddle webhook → automatic license generation + email delivery
- **Email Delivery** — HTML receipts with license serial numbers (via SMTP or Resend API)
- **GeeTest v4 Captcha** — Server-side validation for anti-bot protection
- **Team Mode Backend** — Leaderboard, per-client routing, online member tracking
- **Security** — Cloudflare real-IP trust, security headers (via Caddy), Ed25519 license signing
- **Deployment** — Docker Compose with PostgreSQL + Caddy reverse proxy

### Quick Start

```bash
# 1. Set environment variables (copy from .env.example)
cp .env.example .env
# Edit .env: set DATABASE_URL, LICENSE_PRIVATE_KEY, EMAIL_*, GEETEST_*, etc.

# 2. Run with Docker Compose
docker-compose up -d

# 3. Migrate database
docker-compose exec backend /app/server migrate

# 4. Check status
curl http://localhost:8080/health
```

Server runs on `http://localhost:8080` (proxied via Caddy on `:443` in production).

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-----------|
| `/health` | GET | Health check |
| `/api/register` | POST | Create customer account |
| `/api/login` | POST | Authenticate user |
| `/api/logout` | POST | End session |
| `/api/account` | GET | Get account info (authenticated) |
| `/api/reset-password` | POST | Request password reset |
| `/api/update-password` | POST | Update password with reset token |
| `/api/license/lookup` | POST | Lookup serial number by email |
| `/api/webhook/payment` | POST | Paddle webhook (license auto-issue) |
| `/api/team/leaderboard` | GET | Team Mode leaderboard |
| `/api/team/online` | GET | Online members (Team Mode) |

### Environment Variables

| Variable | Required | Description |
|----------|-------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `LICENSE_PRIVATE_KEY` | Yes | Ed25519 private key (base64) for license signing |
| `LICENSE_PUBLIC_KEY` | No | Ed25519 public key (base64) — auto-derived if omitted |
| `RESEND_API_KEY` | Yes | Email delivery API key (Resend or SMTP) |
| `EMAIL_FROM` | Yes | Sender email address |
| `GEETEST_ID` | Yes | GeeTest Captcha ID |
| `GEETEST_KEY` | Yes | GeeTest Captcha Key |
| `APP_BASE_URL` | Yes | Base URL for customer accounts (e.g., `https://example.com`) |
| `COOKIE_SECURE` | No | Set `true` in production (HTTPS only cookies) |

### Database Migrations

Migrations are in `internal/db/migrations/`:

| Migration | Description |
|-----------|-------------|
| `001_initial.sql` | Initial schema (accounts, sessions) |
| `002_licenses.sql` | License table |
| `003_email_lookups.sql` | Email → serial number mapping |
| `004_payment_webhooks.sql` | Payment webhook log |
| `005_team.sql` | Team Mode (leaderboard, online members) |

Run migrations on startup:

```bash
./server migrate
```

### License Management

#### Workflow

1. **Customer purchases** → Paddle webhook triggers `/api/webhook/payment`
2. **Backend generates** Ed25519-signed license with serial number
3. **Email sent** via Resend API (HTML receipt with serial number)
4. **Customer activates** in Blitz/CloseCrab via serial number + machine fingerprint

#### Serial Number Format

```
AIDAW-XXXX-XXXX-XXXX-XXXX    # Blitz DAW
CRCB-XXXX-XXXX-XXXX-XXXX     # CloseCrab
```

Each serial number is signed with Ed25519. Clients verify with the embedded public key.

### Team Mode Backend

When CloseCrab Team Mode is enabled, the backend tracks:
- **Leaderboard** — Per-client stats (lines written, tools used, bugs fixed)
- **Online Members** — Real-time connected clients
- **Routing** — Per-client session management

Enable via registry credential derivation (see `internal/team/`).

### Deployment

#### Docker Compose (Production)

```yaml
services:
  backend:
    image: ghcr.io/blitzball996/companywebsite-backend:latest
    env_file: .env
    volumes:
      - ./license-key.txt:/app/license-key.txt:ro  # Ed25519 private key
    depends_on:
      - postgres
  caddy:
    image: caddy:2
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: blitzball
      POSTGRES_USER: blitzball
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
```

## Caddy Security Headers

Caddyfile snippet for security:

```
header {
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    X-XSS-Protection "1; mode=block"
    Referrer-Policy "strict-origin-when-cross-origin"
    Permissions-Policy "geolocation=(), microphone=(), camera=()"
}
```

### Building from Source

```bash
# Install dependencies
go mod download

# Build
go build -o server cmd/server/main.go

# Run
./server
```

### License

Proprietary. © 2024-2026 Blitzball Analytics. All rights reserved.

---

## 中文

**Blitzball Analytics** 公司网站 Go 后端 — 客户账户、许可证管理、支付 webhook 和团队模式排行榜。

### 功能

- **客户账户** — 注册、登录、会话管理（bcrypt 密码哈希）
- **许可证管理** — 支付 webhook 自动发码、邮箱查序列号
- **支付集成** — Paddle webhook → 自动生成许可证 + 邮件发送
- **邮件发送** — HTML 收据含序列号（通过 SMTP 或 Resend API）
- **极验 v4 验证码** — 服务端验证防机器人
- **团队模式后端** — 排行榜、客户端路由、在线成员跟踪
- **安全** — Cloudflare 真实 IP 信任、安全头（via Caddy）、Ed25519 许可证签名
- **部署** — Docker Compose + PostgreSQL + Caddy 反向代理

### 快速开始

```bash
# 1. 设置环境变量（从 .env.example 复制）
cp .env.example .env
# 编辑 .env: 设置 DATABASE_URL、LICENSE_PRIVATE_KEY、EMAIL_*、GEETEST_* 等

# 2. Docker Compose 运行
docker-compose up -d

# 3. 迁移数据库
docker-compose exec backend /app/server migrate

# 4. 检查状态
curl http://localhost:8080/health
```

服务器运行在 `http://localhost:8080`（生产环境通过 Caddy 代理到 `:443`）。

### API 端点

| 端点 | 方法 | 描述 |
|------|------|------|
| `/health` | GET | 健康检查 |
| `/api/register` | POST | 创建客户账户 |
| `/api/login` | POST | 用户认证 |
| `/api/logout` | POST | 结束会话 |
| `/api/account` | GET | 获取账户信息（需认证）|
| `/api/reset-password` | POST | 请求重置密码 |
| `/api/update-password` | POST | 用重置令牌更新密码 |
| `/api/license/lookup` | POST | 邮箱查序列号 |
| `/api/webhook/payment` | POST | Paddle webhook（自动发码）|
| `/api/team/leaderboard` | GET | 团队模式排行榜 |
| `/api/team/online` | GET | 在线成员（团队模式）|

### 环境变量

| 变量 | 必需 | 描述 |
|------|------|------|
| `DATABASE_URL` | 是 | PostgreSQL 连接字符串 |
| `LICENSE_PRIVATE_KEY` | 是 | Ed25519 私钥（base64）用于许可证签名 |
| `LICENSE_PUBLIC_KEY` | 否 | Ed25519 公钥（base64）— 省略时自动推导 |
| `RESEND_API_KEY` | 是 | 邮件发送 API 密钥（Resend 或 SMTP）|
| `EMAIL_FROM` | 是 | 发件人邮箱地址 |
| `GEETEST_ID` | 是 | 极验验证码 ID |
| `GEETEST_KEY` | 是 | 极验验证码密钥 |
| `APP_BASE_URL` | 是 | 客户账户基础 URL（如 `https://example.com`）|
| `COOKIE_SECURE` | 否 | 生产环境设 `true`（仅 HTTPS cookie）|

### 数据库迁移

迁移位于 `internal/db/migrations/`：

| 迁移 | 描述 |
|------|------|
| `001_initial.sql` | 初始模式（账户、会话）|
| `002_licenses.sql` | 许可证表 |
| `003_email_lookups.sql` | 邮箱 → 序列号映射 |
| `004_payment_webhooks.sql` | 支付 webhook 日志 |
| `005_team.sql` | 团队模式（排行榜、在线成员）|

启动时运行迁移：

```bash
./server migrate
```

### 许可证管理

#### 工作流

1. **客户购买** → Paddle webhook 触发 `/api/webhook/payment`
2. **后端生成** Ed25519 签名许可证和序列号
3. **发送邮件** 通过 Resend API（HTML 收据含序列号）
4. **客户激活** 在 Blitz/CloseCrab 通过序列号 + 机器指纹

#### 序列号格式

```
AIDAW-XXXX-XXXX-XXXX    # Blitz DAW
CRCB-XXXX-XXXX-XXXX-XXXX     # CloseCrab
```

每个序列号用 Ed25519 签名。客户端用内置公钥验证。

### 团队模式后端

CloseCrab 团队模式启用时，后端跟踪：
- **排行榜** — 每客户端统计（写入行数、工具使用、修复 bug）
- **在线成员** — 实时连接客户端
- **路由** — 每客户端会话管理

通过注册表凭证派生启用（见 `internal/team/`）。

### 部署

#### Docker Compose（生产）

```yaml
services:
  backend:
    image: ghcr.io/blitzball996/companywebsite-backend:latest
    env_file: .env
    volumes:
      - ./license-key.txt:/app/license-key.txt:ro  # Ed25519 私钥
    depends_on:
      - postgres
  caddy:
    image: caddy:2
    ports:
      - "443:443"
    - "80:80"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: blitzball
      POSTGRES_USER: blitzball
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
```

#### Caddy 安全头

Caddyfile 安全片段：

```
header {
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    X-XSS-Protection "1; mode=block"
    Referrer-Policy "strict-origin-when-cross-origin"
    Permissions-Policy "geolocation=(), microphone=(), camera=()"
}
```

### 从源码构建

```bash
# 安装依赖
go mod download

# 构建
go build -o server cmd/server/main.go

# 运行
./server
```

### 许可证

专有。© 2024-2026 Blitzball Analytics。保留所有权利。
