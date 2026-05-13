#!/bin/bash
# setup-piaware.sh — Install PiAware feeder connecting to an existing readsb instance
#
# This script:
# 1. Adds the FlightAware APT repository and GPG keyring
# 2. Installs piaware (feeder only, no dump1090-fa)
# 3. Configures piaware to connect to the local readsb Beast output (port 30005)
# 4. Enables and starts the piaware service
#
# Prerequisites:
#   - Raspberry Pi running Debian Bullseye or Bookworm (armhf or arm64)
#   - readsb already running with --net-bo-port 30005 (Beast output)
#   - Internet access
#
# Usage:
#   ssh pi 'sudo bash -s' < setup-piaware.sh
#   ssh pi 'sudo bash -s -- --feeder-id XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX' < setup-piaware.sh
#
# After setup:
#   If no --feeder-id was provided, visit https://flightaware.com/adsb/piaware/claim
#   to link the receiver to your FlightAware account.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# -------------------------------------------------------------------
# Parse arguments
# -------------------------------------------------------------------
FEEDER_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --feeder-id) FEEDER_ID="$2"; shift 2 ;;
        *) echo "Unknown argument: $1"; echo "Usage: $0 [--feeder-id UUID]"; exit 1 ;;
    esac
done

# Must run as root
if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (use sudo)"
fi

info "=== PiAware Feeder Setup ==="
echo

# -------------------------------------------------------------------
# Step 1: Detect OS codename
# -------------------------------------------------------------------
info "Step 1: Detecting OS..."

if [ -x "$(command -v lsb_release)" ]; then
    CODENAME="$(lsb_release -c -s)"
fi

if [ -z "${CODENAME:-}" ]; then
    . /etc/os-release
    case "$VERSION_ID" in
        "10") CODENAME="buster"   ;;
        "11") CODENAME="bullseye" ;;
        "12") CODENAME="bookworm" ;;
        *)    error "Unsupported Debian version: $VERSION_ID" ;;
    esac
fi

ARCH="$(dpkg --print-architecture)"
info "Detected: Debian ${CODENAME} (${ARCH})"

# Verify supported
case "${CODENAME}:${ARCH}" in
    bullseye:armhf|bullseye:arm64|bookworm:armhf|bookworm:arm64|buster:armhf)
        ;;
    *)
        warn "FlightAware may not provide packages for ${CODENAME}:${ARCH}"
        ;;
esac

# -------------------------------------------------------------------
# Step 2: Install FlightAware APT repository
# -------------------------------------------------------------------
info "Step 2: Adding FlightAware APT repository..."

KEYRING_URL="https://github.com/flightaware/flightaware-apt-repository/raw/master/usr/share/keyrings/flightaware-archive-keyring.gpg"
KEYRING_PATH="/usr/share/keyrings/flightaware-archive-keyring.gpg"
SOURCES_LIST="/etc/apt/sources.list.d/flightaware-apt-repository.list"

# Install keyring
curl -fsSL "${KEYRING_URL}" -o "${KEYRING_PATH}"
info "Installed GPG keyring to ${KEYRING_PATH}"

# Write sources list
cat > "${SOURCES_LIST}" << EOF
deb [ signed-by=${KEYRING_PATH} ] http://flightaware.com/adsb/piaware/files/packages ${CODENAME} piaware
EOF
info "Added APT source: ${SOURCES_LIST}"

# -------------------------------------------------------------------
# Step 3: Install piaware
# -------------------------------------------------------------------
info "Step 3: Installing piaware..."

apt-get update -qq
apt-get install -y --no-install-recommends piaware

# -------------------------------------------------------------------
# Step 4: Configure piaware
# -------------------------------------------------------------------
info "Step 4: Configuring piaware..."

# Tell piaware to use the local Beast output from readsb
piaware-config receiver-type other
piaware-config receiver-host localhost
piaware-config receiver-port 30005

# Set feeder ID if provided
if [[ -n "$FEEDER_ID" ]]; then
    piaware-config feeder-id "$FEEDER_ID"
    info "Feeder ID set to: $FEEDER_ID"
fi

# Allow automatic updates for piaware
piaware-config allow-auto-updates yes
piaware-config allow-manual-updates yes

# -------------------------------------------------------------------
# Step 5: Verify readsb Beast output is available
# -------------------------------------------------------------------
info "Step 5: Checking readsb Beast output on port 30005..."

if ss -tlnp | grep -q ':30005'; then
    info "Beast output detected on port 30005"
else
    warn "Nothing listening on port 30005!"
    warn "Ensure readsb is configured with --net-bo-port 30005"
    warn "PiAware will retry connecting, but won't feed data until Beast output is available"
fi

# -------------------------------------------------------------------
# Step 6: Enable and start piaware
# -------------------------------------------------------------------
info "Step 6: Enabling and starting piaware..."

systemctl enable piaware
systemctl restart piaware

sleep 3

if systemctl is-active --quiet piaware; then
    info "piaware is running!"
else
    warn "piaware may not have started cleanly. Check: journalctl -u piaware"
fi

# -------------------------------------------------------------------
# Summary
# -------------------------------------------------------------------
echo
info "=== Setup Complete ==="
echo
echo "  Service:   piaware.service (enabled, starts on boot)"
echo "  Status:    sudo piaware-status"
echo "  Logs:      journalctl -u piaware -f"
echo
if [[ -n "$FEEDER_ID" ]]; then
    echo "  Feeder ID: $FEEDER_ID"
    echo "  Your receiver should appear at: https://flightaware.com/adsb/stats"
else
    echo "  To link this receiver to your FlightAware account:"
    echo "    Visit: https://flightaware.com/adsb/piaware/claim"
    echo "    Or run: sudo piaware-config feeder-id <YOUR-FEEDER-ID>"
    echo "    Then:   sudo systemctl restart piaware"
fi
echo
