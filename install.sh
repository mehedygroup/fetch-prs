#!/usr/bin/env bash
set -euo pipefail

REPO="mehedygroup/fetch-prs"
BINARY_NAME="fetch-prs"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
REQUESTED_VERSION="${1:-${FETCH_PRS_VERSION:-${VERSION:-}}}"

OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"

case "$OS_RAW" in
  Darwin) OS="darwin" ;;
  Linux) OS="linux" ;;
  *)
    echo "❌ Unsupported operating system: $OS_RAW"
    exit 1
    ;;
esac

case "$ARCH_RAW" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "❌ Unsupported architecture: $ARCH_RAW"
    exit 1
    ;;
esac

TMP_DIR="$(mktemp -d)"
RELEASE_JSON="$TMP_DIR/release.json"
trap 'rm -rf "$TMP_DIR"' EXIT

CURL_API_ARGS=(--location --silent --show-error --fail)
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  CURL_API_ARGS+=( -H "Authorization: Bearer $GITHUB_TOKEN" )
fi

CURL_DOWNLOAD_ARGS=(--location --silent --show-error --fail)

log() {
  printf '%s\n' "$*"
}

extract_json_string() {
  local key="$1"
  local value
  value=$(tr -d '\n' < "$RELEASE_JSON" | sed -E "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"([^\"]+)\".*/\1/")
  if [[ "$value" == "$(tr -d '\n' < "$RELEASE_JSON")" ]]; then
    return 1
  fi
  printf '%s' "$value"
}

resolve_release_endpoint() {
  if [[ -z "$REQUESTED_VERSION" ]]; then
    printf 'https://api.github.com/repos/%s/releases/latest' "$REPO"
    return 0
  fi

  if [[ "$REQUESTED_VERSION" == v* ]]; then
    printf 'https://api.github.com/repos/%s/releases/tags/%s' "$REPO" "$REQUESTED_VERSION"
    return 0
  fi

  printf 'https://api.github.com/repos/%s/releases/tags/%s' "$REPO" "$REQUESTED_VERSION"
}

fetch_release_json() {
  local url
  url="$(resolve_release_endpoint)"

  log "🔍 Fetching release metadata for ${REQUESTED_VERSION:-latest}..."
  if ! curl "${CURL_API_ARGS[@]}" "$url" -o "$RELEASE_JSON"; then
    if [[ -n "$REQUESTED_VERSION" && "$REQUESTED_VERSION" != v* ]]; then
      REQUESTED_VERSION="v$REQUESTED_VERSION"
      url="$(resolve_release_endpoint)"
      log "🔁 Retrying with tag $REQUESTED_VERSION..."
      curl "${CURL_API_ARGS[@]}" "$url" -o "$RELEASE_JSON"
    else
      return 1
    fi
  fi

  TAG_NAME="$(extract_json_string tag_name)"
  TARBALL_URL="$(extract_json_string tarball_url)"

  if [[ -z "$TAG_NAME" || -z "$TARBALL_URL" ]]; then
    echo "❌ Failed to parse release metadata from GitHub."
    cat "$RELEASE_JSON"
    exit 1
  fi

  log "🔖 Selected release: $TAG_NAME"
}

release_asset_url() {
  local asset_name="$1"
  printf 'https://github.com/%s/releases/download/%s/%s' "$REPO" "$TAG_NAME" "$asset_name"
}

asset_exists() {
  local url="$1"
  local status
  status=$(curl --location --silent --output /dev/null --write-out '%{http_code}' "$url" || true)
  [[ "$status" == "200" ]]
}

find_prebuilt_asset() {
  local version_no_v="${TAG_NAME#v}"
  local candidates=(
    "${BINARY_NAME}_${version_no_v}_${OS}_${ARCH}.tar.gz"
    "${BINARY_NAME}-${version_no_v}-${OS}-${ARCH}.tar.gz"
    "${BINARY_NAME}_${OS}_${ARCH}.tar.gz"
    "${BINARY_NAME}-${OS}-${ARCH}.tar.gz"
    "${BINARY_NAME}_${version_no_v}_${OS}_${ARCH}.tgz"
    "${BINARY_NAME}-${version_no_v}-${OS}-${ARCH}.tgz"
    "${BINARY_NAME}_${OS}_${ARCH}.tgz"
    "${BINARY_NAME}-${OS}-${ARCH}.tgz"
    "${BINARY_NAME}_${version_no_v}_${OS}_${ARCH}.zip"
    "${BINARY_NAME}-${version_no_v}-${OS}-${ARCH}.zip"
    "${BINARY_NAME}_${OS}_${ARCH}.zip"
    "${BINARY_NAME}-${OS}-${ARCH}.zip"
    "${BINARY_NAME}_${version_no_v}_${OS}_${ARCH}"
    "${BINARY_NAME}-${version_no_v}-${OS}-${ARCH}"
    "${BINARY_NAME}_${OS}_${ARCH}"
    "${BINARY_NAME}-${OS}-${ARCH}"
  )

  local candidate url
  for candidate in "${candidates[@]}"; do
    url="$(release_asset_url "$candidate")"
    if asset_exists "$url"; then
      printf '%s' "$candidate"
      return 0
    fi
  done

  return 1
}

download_file() {
  local url="$1"
  local destination="$2"
  log "⬇️  Downloading $url"
  curl "${CURL_DOWNLOAD_ARGS[@]}" "$url" -o "$destination"
}

ensure_install_dir() {
  if [[ -d "$INSTALL_DIR" ]]; then
    return 0
  fi

  if mkdir -p "$INSTALL_DIR" 2>/dev/null; then
    return 0
  fi

  if command -v sudo >/dev/null 2>&1; then
    log "🔐 Creating $INSTALL_DIR with sudo"
    sudo mkdir -p "$INSTALL_DIR"
    return 0
  fi

  echo "❌ Cannot create install directory: $INSTALL_DIR"
  exit 1
}

install_binary() {
  local source_file="$1"
  ensure_install_dir

  if install -m 0755 "$source_file" "$INSTALL_DIR/$BINARY_NAME" 2>/dev/null; then
    :
  elif command -v sudo >/dev/null 2>&1; then
    log "🔐 Installing to $INSTALL_DIR with sudo"
    sudo install -m 0755 "$source_file" "$INSTALL_DIR/$BINARY_NAME"
  else
    echo "❌ Failed to install $BINARY_NAME to $INSTALL_DIR"
    exit 1
  fi

  log "✅ Installed $BINARY_NAME to $INSTALL_DIR/$BINARY_NAME"
}

find_binary_in_dir() {
  local search_dir="$1"
  local match

  match=$(find "$search_dir" -type f -name "$BINARY_NAME" -perm -u+x | head -n 1 || true)
  if [[ -z "$match" ]]; then
    match=$(find "$search_dir" -type f -name "$BINARY_NAME" | head -n 1 || true)
  fi

  if [[ -z "$match" ]]; then
    return 1
  fi

  printf '%s' "$match"
}

install_from_prebuilt_asset() {
  local asset_name="$1"
  local archive_path="$TMP_DIR/$asset_name"
  local extract_dir="$TMP_DIR/extracted"
  mkdir -p "$extract_dir"

  download_file "$(release_asset_url "$asset_name")" "$archive_path"

  case "$asset_name" in
    *.tar.gz|*.tgz)
      tar -xzf "$archive_path" -C "$extract_dir"
      ;;
    *.zip)
      if ! command -v unzip >/dev/null 2>&1; then
        echo "❌ unzip is required to install $asset_name"
        exit 1
      fi
      unzip -q "$archive_path" -d "$extract_dir"
      ;;
    *)
      chmod +x "$archive_path"
      install_binary "$archive_path"
      return 0
      ;;
  esac

  local binary_path
  binary_path="$(find_binary_in_dir "$extract_dir")" || {
    echo "❌ Could not locate $BINARY_NAME inside $asset_name"
    exit 1
  }

  chmod +x "$binary_path"
  install_binary "$binary_path"
}

install_from_source_tarball() {
  local source_archive="$TMP_DIR/source.tar.gz"
  local source_dir="$TMP_DIR/source"
  mkdir -p "$source_dir"

  if ! command -v go >/dev/null 2>&1; then
    echo "❌ No compatible release asset found for ${OS}/${ARCH}, and Go is not installed for source fallback."
    echo "   Install Go or publish a release asset for this platform."
    exit 1
  fi

  log "📦 No matching prebuilt asset found; downloading source release tarball instead."
  download_file "$TARBALL_URL" "$source_archive"
  tar -xzf "$source_archive" -C "$source_dir" --strip-components=1

  log "🛠️  Building $BINARY_NAME from source for ${OS}/${ARCH}..."
  (
    cd "$source_dir"
    GOOS="$OS" GOARCH="$ARCH" go build -o "$TMP_DIR/$BINARY_NAME" .
  )

  install_binary "$TMP_DIR/$BINARY_NAME"
}

print_success_message() {
  log "🎉 ${BINARY_NAME} ${TAG_NAME} installed successfully."
  if command -v "$BINARY_NAME" >/dev/null 2>&1; then
    log "🚀 Try: ${BINARY_NAME} fetch 2025-12-01 2025-12-15 --plain"
  else
    log "ℹ️  Ensure $INSTALL_DIR is on your PATH before running $BINARY_NAME."
  fi
}

main() {
  fetch_release_json

  if asset_name="$(find_prebuilt_asset)"; then
    log "📦 Found prebuilt asset: $asset_name"
    install_from_prebuilt_asset "$asset_name"
  else
    install_from_source_tarball
  fi

  print_success_message
}

main "$@"

