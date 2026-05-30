#!/usr/bin/env bash
# Blitzball Labs — one-shot production deploy (run on the Hong Kong server).
#
#   sudo bash deploy/deploy.sh [domain]
#
# It will: install Docker (if missing) → generate .env with random secrets
# (first run only) → build & start db + backend + Caddy (auto HTTPS).
# The frontend repo (CompanyWebsite) must already be cloned as a SIBLING dir.
set -euo pipefail

SITE_DOMAIN="${1:-blitzball.lol}"
cd "$(dirname "$0")/.."          # backend repo root
ROOT="$(pwd)"

echo "============================================================"
echo "  Blitzball Labs production deploy"
echo "  domain : $SITE_DOMAIN"
echo "  dir    : $ROOT"
echo "============================================================"

# 1) Docker -------------------------------------------------------------
if ! command -v docker >/dev/null 2>&1; then
  echo ">> Installing Docker..."
  curl -fsSL https://get.docker.com | sh
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "!! 'docker compose' plugin not found. Install Docker Compose v2 and re-run." >&2
  exit 1
fi

# 2) Frontend must be a sibling directory -------------------------------
if [ ! -d "$ROOT/../CompanyWebsite" ]; then
  echo "!! Frontend not found at $ROOT/../CompanyWebsite"
  echo "   Clone it next to this repo first, e.g.:"
  echo "     cd $ROOT/.. && git clone <CompanyWebsite repo URL> CompanyWebsite"
  exit 1
fi

# 3) .env with random secrets (first run only) --------------------------
if [ ! -f "$ROOT/.env" ]; then
  echo ">> Generating .env with random secrets..."
  gen(){ openssl rand -base64 18 2>/dev/null | tr -d '/+=' | cut -c1-20 || date +%s%N; }
  PASS="$(gen)"; SALT="$(gen)$(gen)"; DBP="$(gen)"
  cat > "$ROOT/.env" <<EOF
SITE_DOMAIN=$SITE_DOMAIN
DASHBOARD_USER=admin
DASHBOARD_PASS=$PASS
VISITOR_SALT=$SALT
DB_PASSWORD=$DBP
STORE_IP=true
EOF
  echo "============================================================"
  echo "  DASHBOARD LOGIN (saved in .env — write it down!):"
  echo "    URL : https://$SITE_DOMAIN/dashboard"
  echo "    user: admin"
  echo "    pass: $PASS"
  echo "============================================================"
else
  echo ">> .env already exists, keeping current secrets."
fi

# 4) Build & start ------------------------------------------------------
echo ">> Building and starting containers..."
docker compose -f docker-compose.prod.yml up -d --build

echo ""
echo ">> Done. In ~30s your site should be live at: https://$SITE_DOMAIN"
echo "   (Caddy auto-fetches the HTTPS certificate — the domain must already"
echo "    point to THIS server's IP, and ports 80/443 must be open.)"
echo ""
echo "   Check status : docker compose -f docker-compose.prod.yml ps"
echo "   View logs    : docker compose -f docker-compose.prod.yml logs -f caddy"
