#!/usr/bin/env bash
# Generate self-signed test certificates for CI.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

openssl ecparam -genkey -name prime256v1 -noout -out "$DIR/server.key"

openssl req -new -x509 \
  -key "$DIR/server.key" \
  -out "$DIR/server.crt" \
  -days 365 \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1"

echo "Generated server.crt and server.key in $DIR"

