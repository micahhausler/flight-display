#!/bin/bash
# fix-readsb-beast-port.sh — One-off: add Beast output port to existing readsb service
#
# This adds --net-bo-port 30005 to the readsb systemd service file
# so that piaware (or other Beast consumers) can connect.
#
# Usage:
#   ssh pi 'sudo bash -s' < fix-readsb-beast-port.sh

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (use sudo)"
fi

SERVICE_FILE="/etc/systemd/system/readsb.service"

if [[ ! -f "$SERVICE_FILE" ]]; then
    error "readsb service file not found at $SERVICE_FILE"
fi

# Check if already configured
if grep -q 'net-bo-port' "$SERVICE_FILE"; then
    info "Beast output port already configured in $SERVICE_FILE"
    exit 0
fi

# Add --net-bo-port 30005 after --net flag
# Handle both multiline (with \) and single-line service files
if grep -q '\-\-net ' "$SERVICE_FILE"; then
    sed -i 's/--net /--net --net-bo-port 30005 /' "$SERVICE_FILE"
    info "Added --net-bo-port 30005 to $SERVICE_FILE"
else
    error "Could not find --net flag in $SERVICE_FILE. Add --net-bo-port 30005 manually."
fi

# Reload and restart
systemctl daemon-reload
systemctl restart readsb

sleep 2

if systemctl is-active --quiet readsb; then
    info "readsb restarted successfully"
else
    error "readsb failed to restart. Check: journalctl -u readsb"
fi

# Verify port is listening
if ss -tlnp | grep -q ':30005'; then
    info "Beast output now available on port 30005"
else
    warn "Port 30005 not detected yet — may take a moment"
fi
