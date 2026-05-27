#!/usr/bin/env bash

# Detect if the script is being sourced instead of run as a subprocess.
# source/. runs in the current shell: set -e and exit would kill the terminal.
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
  echo "[error] Do not source this script. Run it directly:"
  echo "        bash scripts/setup-local.sh"
  return 1
fi

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ── PATH augmentation ─────────────────────────────────────────────────────────
# Add common tool install locations that may be missing in non-login shells
export PATH="/usr/local/go/bin:$PATH"                      # Go (macOS pkg installer)
export PATH="$HOME/go/bin:$PATH"                           # golang-migrate, etc.
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"       # Homebrew (Apple Silicon / Intel)

# ── helpers ──────────────────────────────────────────────────────────────────

ok()   { echo "[ok]  $*"; }
info() { echo "[..] $*"; }
warn() { echo "[!!] $*"; }
die()  { echo "[xx] $*" >&2; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not found. Install it and re-run."
}

# ── prerequisites ─────────────────────────────────────────────────────────────

info "Checking prerequisites..."
require docker
require go
require python3
require node
require npm

# golang-migrate (install if missing — go install is user-global to ~/go/bin)
if ! command -v migrate >/dev/null 2>&1; then
  info "Installing golang-migrate..."
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  if command -v migrate >/dev/null 2>&1; then
    ok "golang-migrate installed."
    SKIP_MIGRATE=0
  else
    warn "golang-migrate install succeeded but binary not found in PATH. Skipping migrations."
    SKIP_MIGRATE=1
  fi
else
  SKIP_MIGRATE=0
fi

# MinIO client (install via Homebrew if missing)
MC_AVAILABLE=0
if command -v mc >/dev/null 2>&1; then
  MC_AVAILABLE=1
elif command -v brew >/dev/null 2>&1; then
  info "Installing MinIO client (mc) via Homebrew..."
  brew install minio/stable/mc --quiet && MC_AVAILABLE=1 || warn "mc install failed — bucket must be created manually."
else
  warn "mc not found and Homebrew not available — create bucket manually at http://localhost:9001"
fi

ok "Prerequisites satisfied."

# ── JWT keys ──────────────────────────────────────────────────────────────────

if [[ ! -f go-app/jwt_private.pem ]] || [[ ! -f go-app/jwt_public.pem ]]; then
  info "Generating RS256 JWT key pair..."
  openssl genrsa -out go-app/jwt_private.pem 2048 2>/dev/null
  openssl rsa -in go-app/jwt_private.pem -pubout -out go-app/jwt_public.pem 2>/dev/null
  ok "JWT keys written to go-app/jwt_private.pem and go-app/jwt_public.pem"
else
  ok "JWT keys already exist, skipping."
fi

# ── go-app .env ───────────────────────────────────────────────────────────────

if [[ ! -f go-app/.env ]]; then
  info "Creating go-app/.env from template..."
  cat > go-app/.env <<'EOF'
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
REDIS_URL=redis://localhost:6379/0
KAFKA_BOOTSTRAP_SERVERS=localhost:9092
JWT_PRIVATE_KEY_PATH=./jwt_private.pem
JWT_PUBLIC_KEY_PATH=./jwt_public.pem
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=atp-strategies
S3_REGION=us-east-1
CORS_ORIGINS=http://localhost:5173
PORT=8080
EOF
  ok "go-app/.env created."
else
  ok "go-app/.env already exists, skipping."
fi

# ── go-data .env ─────────────────────────────────────────────────────────────

if [[ ! -f go-data/.env ]]; then
  info "Creating go-data/.env from template..."
  cat > go-data/.env <<'EOF'
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
KAFKA_BOOTSTRAP_SERVERS=localhost:9092
HTTP_PORT=8081
EOF
  ok "go-data/.env created (add ALPACA_API_KEY and ALPACA_API_SECRET manually)."
else
  ok "go-data/.env already exists, skipping."
fi

# ── python .env ───────────────────────────────────────────────────────────────

if [[ ! -f python/.env ]]; then
  info "Creating python/.env from template..."
  cat > python/.env <<'EOF'
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
REDIS_URL=redis://localhost:6379/0
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=atp-strategies
S3_REGION=us-east-1
LEAN_IMAGE=lean-atp:latest
LEAN_JOB_TMP_DIR=/tmp/atp-jobs
GO_DATA_URL=http://localhost:8081
LEAN_KAFKA_BOOTSTRAP_SERVERS=host.docker.internal:9092
EOF
  ok "python/.env created."
else
  ok "python/.env already exists, skipping."
fi

# ── infrastructure ────────────────────────────────────────────────────────────

info "Starting infrastructure (Kafka, Redis, PostgreSQL, MinIO)..."
docker compose -f local-docker-compose.yml up -d
ok "Infrastructure containers started."

# Wait for PostgreSQL to be ready
info "Waiting for PostgreSQL to be ready..."
for i in $(seq 1 20); do
  if docker exec "$(docker compose -f local-docker-compose.yml ps -q postgres)" pg_isready -U postgres -q 2>/dev/null; then
    ok "PostgreSQL is ready."
    break
  fi
  if [[ $i -eq 20 ]]; then
    die "PostgreSQL did not become ready in time."
  fi
  sleep 2
done

# ── migrations ────────────────────────────────────────────────────────────────

if [[ $SKIP_MIGRATE -eq 0 ]]; then
  info "Running database migrations..."
  migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" \
          -path migrations up
  ok "Migrations applied."
else
  warn "Skipping migrations (migrate not installed)."
fi

# ── MinIO bucket ──────────────────────────────────────────────────────────────

if [[ $MC_AVAILABLE -eq 1 ]]; then
  info "Creating MinIO bucket 'atp-strategies'..."
  mc alias set atp-local http://localhost:9000 minioadmin minioadmin --quiet 2>/dev/null || true
  mc mb --ignore-existing atp-local/atp-strategies 2>/dev/null || true
  ok "MinIO bucket ready."
else
  warn "mc (MinIO client) not found — create bucket manually:"
  warn "  Open http://localhost:9001 (user: minioadmin / pass: minioadmin)"
  warn "  Create a bucket named 'atp-strategies'"
fi

# ── Go dependencies ───────────────────────────────────────────────────────────

info "Downloading Go module dependencies..."
(cd go-app  && go mod tidy 2>&1 | tail -3) || warn "go-app: go mod tidy had warnings."
(cd go-data && go mod tidy 2>&1 | tail -3) || warn "go-data: go mod tidy had warnings."
ok "Go modules ready."

# ── Python dependencies ───────────────────────────────────────────────────────

info "Installing Python dependencies..."
pip3 install -r python/requirements.txt -q
ok "Python packages installed."

# ── Node dependencies ─────────────────────────────────────────────────────────

info "Installing frontend dependencies..."
(cd web && npm install --silent)
ok "npm packages installed."

# ── lean-atp image ────────────────────────────────────────────────────────────

if docker image inspect lean-atp:latest >/dev/null 2>&1; then
  ok "lean-atp:latest image already exists, skipping build."
else
  info "Building lean-atp Docker image (this takes a few minutes)..."
  docker build -t lean-atp:latest lean-plugin/
  ok "lean-atp:latest built."
fi

# ── logs directory ────────────────────────────────────────────────────────────

mkdir -p logs
ok "logs/ directory ready."

# ── done ─────────────────────────────────────────────────────────────────────

echo ""
echo "Setup complete. To start services:"
echo ""
echo "  Terminal 1 (API):      cd go-app  && go run ."
echo "  Terminal 2 (Data):     cd go-data && go run main.go"
echo "  Terminal 3 (Workers):  cd python  && celery -A celery_worker worker --loglevel=info"
echo "  Terminal 4 (Frontend): cd web     && npm run dev"
echo ""
echo "  Frontend:  http://localhost:5173"
echo "  API:       http://localhost:8080"
echo "  MinIO:     http://localhost:9001  (minioadmin / minioadmin)"
