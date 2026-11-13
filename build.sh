#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "1/2 Building React frontend -> ${ROOT_DIR}/web ..."
cd "${ROOT_DIR}/frontend"
npm run build

echo "2/2 Compiling static Go binary -> ${ROOT_DIR}/pg-admin-pro ..."
cd "${ROOT_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags="-s -w" -o pg-admin-pro .

echo "Build complete:"
ls -lh "${ROOT_DIR}/pg-admin-pro"
ls -la "${ROOT_DIR}/web"
