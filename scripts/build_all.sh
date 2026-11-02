#!/usr/bin/env bash
set -e

VERSION="v1.0.0"
APP_NAME="git-webui"
DIST_DIR="dist"

echo "=========================================="
echo " Building GitWebUI $VERSION for All OS "
echo "=========================================="

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Targets: GOOS/GOARCH
TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

for TARGET in "${TARGETS[@]}"; do
  OS="${TARGET%%/*}"
  ARCH="${TARGET##*/}"
  
  OUTPUT_NAME="${APP_NAME}-${OS}-${ARCH}"
  if [ "$OS" = "windows" ]; then
    OUTPUT_NAME="${OUTPUT_NAME}.exe"
  fi

  echo "🔨 Compiling for $OS ($ARCH)..."
  CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" go build -ldflags="-s -w" -o "${DIST_DIR}/${OUTPUT_NAME}" main.go

  # Package into tar.gz or zip
  cd "$DIST_DIR"
  if [ "$OS" = "windows" ]; then
    zip -q "${APP_NAME}-${OS}-${ARCH}.zip" "${OUTPUT_NAME}"
  else
    tar -czf "${APP_NAME}-${OS}-${ARCH}.tar.gz" "${OUTPUT_NAME}"
  fi
  cd ..
done

echo "✅ All builds completed in ${DIST_DIR}/:"
ls -lh "$DIST_DIR"
