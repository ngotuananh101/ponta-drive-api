#!/usr/bin/env bash
# ==============================================================================
# Ponta Drive Backend - Deployment Script
# Framework: Goravel (Go)
# ==============================================================================

set -euo pipefail

# --- Màu sắc hiển thị ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] [INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] [SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] [WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] [ERROR]${NC} $1"
}

# --- Cấu hình mặc định ---
BRANCH="${1:-main}"
SERVICE_NAME="${SERVICE_NAME:-ponta-drive-backend}"
PORT="${APP_PORT:-3000}"
HEALTHCHECK_URL="http://127.0.0.1:${PORT}/ping"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$PROJECT_DIR"

log_info "========================================================"
log_info "Bắt đầu triển khai Ponta Drive Backend (Nhánh: $BRANCH)"
log_info "Thư mục dự án: $PROJECT_DIR"
log_info "========================================================"

# 1. Kiểm tra file cấu hình .env
if [ ! -f .env ]; then
    log_error "File .env không tồn tại tại $PROJECT_DIR/.env!"
    log_warn "Hãy sao chép .env.example sang .env và cấu hình trước khi deploy: cp .env.example .env"
    exit 1
fi

# 2. Cập nhật mã nguồn từ Git
log_info "1/5. Kéo mã nguồn mới nhất từ Git origin/$BRANCH..."
git fetch origin "$BRANCH"
git reset --hard "origin/$BRANCH"
log_success "Đã cập nhật mã nguồn thành công."

# 3. Kiểm tra và tải các thư viện dependencies
log_info "2/5. Kiểm tra môi trường Go và tải các thư viện dependencies..."
if ! command -v go >/dev/null 2>&1; then
    log_error "Không tìm thấy lệnh 'go'. Vui lòng cài đặt Go trên server."
    exit 1
fi

go mod tidy
go mod download

# 4. Biên dịch binary mới (Atomic Swap để tránh làm gián đoạn file binary đang chạy)
log_info "3/5. Biên dịch Go binary (main.new)..."
go build -ldflags "-s -w -extldflags '-static'" -o main.new .
chmod +x main.new
log_success "Biên dịch binary thành công."

# 5. Chạy Migration Database
log_info "4/5. Đang chạy Database Migrations..."
./main.new artisan migrate
log_success "Database migrations hoàn tất."

# 6. Hoán đổi file binary chính
mv main.new main

# 7. Set lại quyền thực thi cho deploy.sh
chmod +x deploy.sh

# 8. Triển khai với docker compose
docker-compose up -d --build

log_success "========================================================"
log_success "🎉 DEPLOY THÀNH CÔNG! Backend đã sẵn sàng phục vụ."
log_success "========================================================"
exit 0