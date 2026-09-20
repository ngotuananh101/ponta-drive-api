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

# 3. Phân loại hình thức triển khai: Docker Compose hoặc Native Systemd
if [ -f "docker-compose.yml" ] && command -v docker >/dev/null 2>&1 && [ "${USE_DOCKER:-false}" = "true" ]; then
    # ----------------------------------------------------
    # TRIỂN KHAI BẰNG DOCKER COMPOSE
    # ----------------------------------------------------
    log_info "2/5. Phát hiện chế độ Docker Compose..."

    log_info "3/5. Build lại image Docker..."
    docker compose build

    log_info "4/5. Chạy Database Migrations..."
    # Chạy migration trong container tạm thời
    docker compose run --rm goravel ./main artisan migrate --force

    log_info "5/5. Khởi động lại service bằng Docker Compose..."
    docker compose up -d

    # Dọn dẹp images cũ không dùng
    docker image prune -f >/dev/null 2>&1 || true

else
    # ----------------------------------------------------
    # TRIỂN KHAI NATIVE (GO BINARY + SYSTEMD)
    # ----------------------------------------------------
    log_info "2/5. Kiểm tra môi trường Go và tải các thư viện dependencies..."
    if ! command -v go >/dev/null 2>&1; then
        log_error "Không tìm thấy lệnh 'go'. Vui lòng cài đặt Go trên server hoặc chuyển USE_DOCKER=true."
        exit 1
    fi

    go mod tidy
    go mod download

    # 3. Biên dịch binary mới (Atomic Swap để tránh làm gián đoạn file binary đang chạy)
    log_info "3/5. Biên dịch Go binary (main.new)..."
    go build -ldflags "-s -w -extldflags '-static'" -o main.new .
    chmod +x main.new
    log_success "Biên dịch binary thành công."

    # 4. Chạy Migration Database
    log_info "4/5. Đang chạy Database Migrations..."
    ./main.new artisan migrate --force
    log_success "Database migrations hoàn tất."

    # Hoán đổi file binary chính
    mv main.new main

    # 5. Khởi động lại service
    log_info "5/5. Khởi động lại dịch vụ..."
    if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files | grep -q "${SERVICE_NAME}.service"; then
        log_info "Khởi động lại Systemd service: $SERVICE_NAME..."
        sudo systemctl restart "$SERVICE_NAME"
    elif command -v pm2 >/dev/null 2>&1 && pm2 list | grep -q "$SERVICE_NAME"; then
        log_info "Khởi động lại PM2 process: $SERVICE_NAME..."
        pm2 restart "$SERVICE_NAME"
    else
        log_warn "Không tìm thấy service $SERVICE_NAME trên systemctl hoặc pm2."
        log_warn "Hãy tự khởi động lại tiến trình chạy ./main nếu bạn đang chạy thủ công."
    fi
fi

# 4. Kiểm tra Healthcheck
log_info "Đang kiểm tra trạng thái hoạt động (Healthcheck: $HEALTHCHECK_URL)..."
HEALTHY=false
for i in {1..15}; do
    if curl -s -f "$HEALTHCHECK_URL" > /dev/null 2>&1; then
        HEALTHY=true
        break
    fi
    sleep 2
done

# 5. Set lại quyền thực thi cho deploy.sh
chmod +x deploy.sh

if [ "$HEALTHY" = true ]; then
    log_success "========================================================"
    log_success "🎉 DEPLOY THÀNH CÔNG! Backend đã sẵn sàng phục vụ."
    log_success "Endpoint /ping phản hồi bình thường."
    log_success "========================================================"
else
    log_error "========================================================"
    log_error "⚠️ CẢNH BÁO: Healthcheck thất bại hoặc chưa sẵn sàng sau 30s!"
    log_error "Vui lòng kiểm tra log: sudo journalctl -u $SERVICE_NAME -n 50 -f"
    log_error "========================================================"
    exit 1
fi
