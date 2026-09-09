#!/usr/bin/env bash
# Quick setup on a fresh Linux host.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created .env — fill TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID"
fi

if [[ ! -f config.yaml ]]; then
  cp config.example.yaml config.yaml
  echo "Created config.yaml — edit watch.services / watch.containers"
fi

echo
echo "Next:"
echo "  1. nano .env"
echo "  2. nano config.yaml   # containers & services for THIS host"
echo "  3. make test && make build"
echo "  4. make install       # or copy binary + files to /opt/hostpulse"
