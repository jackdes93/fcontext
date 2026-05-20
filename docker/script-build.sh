#!/bin/bash
# =============================================================================
# build.sh — Docker image builder with auto-versioning
#
# ── Semantic Versioning (SemVer) ─────────────────────────────────────────────
#
#   Format chuẩn: MAJOR.MINOR.PATCH  (ví dụ: 2.1.4)
#
#   MAJOR  — Tăng khi có breaking change (không tương thích ngược).
#             Ví dụ: đổi toàn bộ API, refactor kiến trúc, migration DB lớn.
#             → Khi tăng MAJOR, reset MINOR và PATCH về 0.
#
#   MINOR  — Tăng khi thêm tính năng mới nhưng vẫn tương thích ngược.
#             Ví dụ: thêm endpoint mới, thêm màn hình, bổ sung config option.
#             → Khi tăng MINOR, reset PATCH về 0.
#
#   PATCH  — Tăng khi sửa bug, hotfix, thay đổi nhỏ không ảnh hưởng API.
#             Ví dụ: fix lỗi logic, sửa typo, cập nhật dependency nhỏ.
#
#   Quy tắc vàng:
#     - MAJOR=0 (0.x.y) → giai đoạn phát triển ban đầu, API chưa ổn định.
#     - Một khi release MAJOR=1 → cam kết với người dùng về tính ổn định.
#     - Không bao giờ sửa/xóa version đã release (tag bất biến).
#
# ── Áp dụng trong script này ─────────────────────────────────────────────────
#
#   Script dùng format tùy chỉnh: v<MAJOR>.<MINOR>.<PATCH>-build<N>
#   thay vì chỉ dùng PATCH, bổ sung BUILD NUMBER tự động tăng mỗi lần chạy.
#
#   Lý do: trong môi trường CI/CD deploy hàng ngày, "build number"
#   phản ánh đúng hơn số lần deploy thực tế, không phải số lần fix bug.
#
#   v1.0.0-build1   ← khởi tạo
#   v1.0.0-build42  ← sau 41 lần build/deploy
#   v1.1.0-build1   ← sau khi thêm feature mới  (--minor → reset build về 1)
#   v2.0.0-build1   ← sau breaking change        (--major → reset về 2.0.0-build1)
#
# ── Version format: v<MAJOR>.<MINOR>.<PATCH>-build<N> ────────────────────────
#   ví dụ: v1.0.0-build1, v1.0.0-build42, v2.1.0-build7
#
# Cách dùng:
#   ./script-build.sh                        # auto bump build number (v1.0.0-build42 → v1.0.0-build43)
#   ./script-build.sh --major                # bump major (v1.0.0-build42 → v2.0.0-build1)
#   ./script-build.sh --minor                # bump minor (v1.0.0-build42 → v1.1.0-build1)
#   ./script-build.sh --patch                # chỉ bump build number (giống mặc định)
#   ./script-build.sh --dry-run              # xem kết quả mà không thực sự build
#   ./script-build.sh --no-tag               # build nhưng không tạo git tag
#
# Biến môi trường:
#   IMAGE_NAME    tên Docker image   (default: tên thư mục hiện tại)
#   PLATFORM      target platform    (default: linux/amd64)
#   DOCKERFILE    đường dẫn         (default: Dockerfile)
# =============================================================================

set -euo pipefail

# ── Màu sắc output ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

log_info()    { echo -e "${BLUE}==>${NC} ${BOLD}$*${NC}"; }
log_success() { echo -e "${GREEN}✔${NC}  $*"; }
log_warn()    { echo -e "${YELLOW}⚠${NC}   $*"; }
log_error()   { echo -e "${RED}✖${NC}  $*" >&2; }
log_step()    { echo -e "${CYAN}   →${NC} $*"; }

# ── Defaults ──────────────────────────────────────────────────────────────────
IMAGE_NAME="${IMAGE_NAME:-$(basename "$PWD")}"
PLATFORM="${PLATFORM:-linux/amd64}"
DOCKERFILE="${DOCKERFILE:-Dockerfile}"

BUMP_MODE="patch"   # patch | minor | major
DRY_RUN=false
NO_TAG=false

# ── Parse arguments ───────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --major)    BUMP_MODE="major"; shift ;;
    --minor)    BUMP_MODE="minor"; shift ;;
    --patch)    BUMP_MODE="patch"; shift ;;
    --dry-run)  DRY_RUN=true;      shift ;;
    --no-tag)   NO_TAG=true;       shift ;;
    -h|--help)
      sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      log_error "Unknown argument: $1"
      log_error "Dùng --help để xem hướng dẫn."
      exit 1
      ;;
  esac
done

# ── Kiểm tra dependencies ─────────────────────────────────────────────────────
check_deps() {
  local missing=()
  for cmd in git docker; do
    command -v "$cmd" &>/dev/null || missing+=("$cmd")
  done

  if ! docker buildx version &>/dev/null; then
    missing+=("docker buildx")
  fi

  if [[ ! -f "$DOCKERFILE" ]]; then
    log_error "Không tìm thấy Dockerfile: $DOCKERFILE"
    exit 1
  fi

  if [[ ${#missing[@]} -gt 0 ]]; then
    log_error "Thiếu các lệnh sau: ${missing[*]}"
    exit 1
  fi
}

# ── Đọc version hiện tại từ git tag ──────────────────────────────────────────
# Format tag: v<MAJOR>.<MINOR>.<PATCH>-build<N>
parse_current_version() {
  local latest_tag

  latest_tag=$(git tag --list 'v*-build*' --sort=-version:refname 2>/dev/null | head -1)

  if [[ -z "$latest_tag" ]]; then
    log_warn "Chưa tìm thấy tag nào theo format v<X>.<Y>.<Z>-build<N>. Khởi tạo từ v1.0.0-build0."
    CUR_MAJOR=1
    CUR_MINOR=0
    CUR_PATCH=0
    CUR_BUILD=0
  else
    if [[ "$latest_tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)-build([0-9]+)$ ]]; then
      CUR_MAJOR="${BASH_REMATCH[1]}"
      CUR_MINOR="${BASH_REMATCH[2]}"
      CUR_PATCH="${BASH_REMATCH[3]}"
      CUR_BUILD="${BASH_REMATCH[4]}"
    else
      log_error "Tag '$latest_tag' không đúng format v<MAJOR>.<MINOR>.<PATCH>-build<N>"
      exit 1
    fi
  fi

  CURRENT_VERSION="v${CUR_MAJOR}.${CUR_MINOR}.${CUR_PATCH}-build${CUR_BUILD}"
}

# ── Tính version mới ──────────────────────────────────────────────────────────
compute_next_version() {
  case "$BUMP_MODE" in
    major)
      NEW_MAJOR=$((CUR_MAJOR + 1))
      NEW_MINOR=0
      NEW_PATCH=0
      NEW_BUILD=1
      ;;
    minor)
      NEW_MAJOR=$CUR_MAJOR
      NEW_MINOR=$((CUR_MINOR + 1))
      NEW_PATCH=0
      NEW_BUILD=1
      ;;
    patch)
      NEW_MAJOR=$CUR_MAJOR
      NEW_MINOR=$CUR_MINOR
      NEW_PATCH=$CUR_PATCH
      NEW_BUILD=$((CUR_BUILD + 1))
      ;;
  esac

  NEXT_VERSION="v${NEW_MAJOR}.${NEW_MINOR}.${NEW_PATCH}-build${NEW_BUILD}"
}

# ── Git info ──────────────────────────────────────────────────────────────────
get_git_info() {
  GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
  GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
  BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
}

# ── Git tag + push ────────────────────────────────────────────────────────────
create_git_tag() {
  local tag="$1"
  local message="Release $tag [skip ci]"

  if git rev-parse "$tag" &>/dev/null; then
    log_warn "Tag '$tag' đã tồn tại, bỏ qua."
    return
  fi

  log_step "Tạo git tag: $tag"
  git tag -a "$tag" -m "$message"

  if git remote get-url origin &>/dev/null; then
    log_step "Push tag lên remote..."
    git push origin "$tag"
    log_success "Đã push tag $tag lên origin."
  else
    log_warn "Không có remote 'origin', tag chỉ được tạo local."
  fi
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
  echo ""
  echo -e "${BOLD}╔══════════════════════════════════════╗${NC}"
  echo -e "${BOLD}║       Docker Auto-Build Script       ║${NC}"
  echo -e "${BOLD}╚══════════════════════════════════════╝${NC}"
  echo ""

  check_deps
  parse_current_version
  compute_next_version
  get_git_info

  log_info "Thông tin build"
  log_step "Image name   : ${IMAGE_NAME}"
  log_step "Platform     : ${PLATFORM}"
  log_step "Bump mode    : ${BUMP_MODE}"
  log_step "Version cũ   : ${CURRENT_VERSION}"
  log_step "Version mới  : ${NEXT_VERSION}  ← sẽ dùng"
  log_step "Git branch   : ${GIT_BRANCH}"
  log_step "Git commit   : ${GIT_COMMIT}"
  log_step "Build date   : ${BUILD_DATE}"
  echo ""

  if [[ "$DRY_RUN" == true ]]; then
    log_warn "DRY-RUN mode: Không thực sự build hay tạo tag."
    log_success "Nếu build thật, image sẽ được tag: ${IMAGE_NAME}:${NEXT_VERSION}"
    exit 0
  fi

  log_info "Building Docker image..."

  docker buildx build \
    --platform="${PLATFORM}" \
    --build-arg VERSION="${NEXT_VERSION}" \
    --build-arg COMMIT="${GIT_COMMIT}" \
    --build-arg BUILD_DATE="${BUILD_DATE}" \
    -t "${IMAGE_NAME}:${NEXT_VERSION}" \
    -t "${IMAGE_NAME}:latest" \
    -f "${DOCKERFILE}" \
    --load \
    .

  log_success "Build thành công: ${IMAGE_NAME}:${NEXT_VERSION}"

  log_info "Xuất file image ${IMAGE_NAME}-${NEXT_VERSION}.tar"

  docker save -o "${IMAGE_NAME}-${NEXT_VERSION}.tar" "${IMAGE_NAME}:${NEXT_VERSION}"

  log_success "Xuất file image ${IMAGE_NAME}-${NEXT_VERSION}.tar thành công"

  if [[ "$NO_TAG" == false ]]; then
    echo ""
    log_info "Tạo Git tag..."
    create_git_tag "${NEXT_VERSION}"
    log_success "Git tag: ${NEXT_VERSION}"
  else
    log_warn "--no-tag: bỏ qua bước tạo git tag."
  fi

  echo ""
  log_info "Dọn dangling images..."
  docker image prune -f --filter "dangling=true" >/dev/null
  log_success "Dọn dẹp xong."

  echo ""
  echo -e "${GREEN}${BOLD}✔ Done!${NC}"
  echo -e "   Image : ${BOLD}${IMAGE_NAME}:${NEXT_VERSION}${NC}"
  [[ "$NO_TAG" == false ]] && echo -e "   Tag   : ${BOLD}${NEXT_VERSION}${NC}"
  echo ""
}

main
