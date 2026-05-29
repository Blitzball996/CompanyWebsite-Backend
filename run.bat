@echo off
REM Blitzball Labs Analytics - one-click start (requires Docker Desktop)
cd /d G:\CMakePJ\CompanyWebsite-Backend
if not exist .env (
  copy .env.example .env
  echo Created .env from .env.example - EDIT IT to set DASHBOARD_PASS and VISITOR_SALT!
)
echo Starting PostgreSQL + analytics backend via Docker...
docker compose up --build
