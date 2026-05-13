#!/bin/bash
# setup-adsb.sh — Install and configure readsb + RTL-SDR on Raspberry Pi
#
# This script:
# 1. Installs rtl-sdr drivers and blacklists conflicting kernel modules
# 2. Builds and installs readsb from source (the actively maintained ADS-B decoder)
# 3. Creates a systemd service for readsb
# 4. Enables the JSON output that flight-display reads from
#
# Prerequisites:
#   - Raspberry Pi running Debian Bullseye (arm64)
#   - Nooelec NESDR Mini (or any RTL2832U-based dongle) plugged into USB
#   - Internet access for package downloads
#
# Usage:
#   ssh pi 'sudo bash -s -- --lat 47.6115 --lon -122.3470' < setup-adsb.sh
#
# After setup, verify with:
#   curl http://localhost:8080/data/aircraft.json

set -euo pipefail

# -------------------------------------------------------------------
# Parse arguments
# -------------------------------------------------------------------
LAT=""
LON=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --lat) LAT="$2"; shift 2 ;;
        --lon) LON="$2"; shift 2 ;;
        *) echo "Unknown argument: $1"; echo "Usage: $0 [--lat LATITUDE --lon LONGITUDE]"; exit 1 ;;
    esac
done

if [[ -n "$LAT" && -z "$LON" ]] || [[ -z "$LAT" && -n "$LON" ]]; then
    echo "Error: --lat and --lon must both be provided"
    exit 1
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# Must run as root
if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root (use sudo)"
fi

info "=== ADS-B Receiver Setup for flight-display ==="
echo

# -------------------------------------------------------------------
# Step 1: Install RTL-SDR drivers
# -------------------------------------------------------------------
info "Step 1: Installing RTL-SDR drivers and build dependencies..."

apt-get update -qq
apt-get install -y --no-install-recommends \
    rtl-sdr \
    librtlsdr-dev \
    librtlsdr0 \
    build-essential \
    debhelper \
    pkg-config \
    libncurses-dev \
    zlib1g-dev \
    libzstd-dev \
    git \
    curl

# -------------------------------------------------------------------
# Step 2: Blacklist DVB kernel modules that conflict with RTL-SDR
# -------------------------------------------------------------------
info "Step 2: Blacklisting DVB kernel modules..."

BLACKLIST_FILE="/etc/modprobe.d/blacklist-rtlsdr.conf"
if [[ ! -f "$BLACKLIST_FILE" ]]; then
    cat > "$BLACKLIST_FILE" << 'EOF'
# Blacklist DVB drivers that claim the RTL2832U device before rtl-sdr can use it
blacklist dvb_usb_rtl28xxu
blacklist dvb_usb_rtl2832u
blacklist rtl2832
blacklist rtl2830
blacklist dvb_usb_v2
blacklist dvb_core
EOF
    info "Created $BLACKLIST_FILE"

    # Unload the modules if currently loaded
    for mod in dvb_usb_rtl28xxu dvb_usb_rtl2832u rtl2832 rtl2830; do
        rmmod "$mod" 2>/dev/null || true
    done
else
    info "Blacklist already exists at $BLACKLIST_FILE, skipping"
fi

# -------------------------------------------------------------------
# Step 3: Build readsb from source
# -------------------------------------------------------------------
info "Step 3: Building readsb from source..."

BUILD_DIR="/tmp/readsb-build"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

cd "$BUILD_DIR"
git clone --depth 1 https://github.com/wiedehopf/readsb.git
cd readsb

# Build with RTL-SDR support and network output
make -j"$(nproc)" \
    RTLSDR=yes \
    OPTIMIZE="-O2"

# Install binary
install -m 755 readsb /usr/local/bin/readsb
install -m 755 viewadsb /usr/local/bin/viewadsb

info "readsb installed to /usr/local/bin/readsb"

# -------------------------------------------------------------------
# Step 4: Create readsb user and directories
# -------------------------------------------------------------------
info "Step 4: Creating readsb user and directories..."

# Create system user if it doesn't exist
if ! id -u readsb &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin readsb
fi

# Create run directory for JSON output
mkdir -p /run/readsb
chown readsb:readsb /run/readsb

# Create persistent data directory
mkdir -p /var/lib/readsb
chown readsb:readsb /var/lib/readsb

# Ensure readsb user can access the RTL-SDR USB device
usermod -aG plugdev readsb 2>/dev/null || true

# -------------------------------------------------------------------
# Step 5: Create systemd service
# -------------------------------------------------------------------
info "Step 5: Creating systemd service..."

READSB_LAT="${LAT:-0}"
READSB_LON="${LON:-0}"

cat > /etc/systemd/system/readsb.service << EOF
[Unit]
Description=readsb ADS-B receiver
Documentation=https://github.com/wiedehopf/readsb
After=network.target
Wants=network.target

[Service]
Type=simple
User=readsb
RuntimeDirectory=readsb
RuntimeDirectoryPreserve=yes
ExecStart=/usr/local/bin/readsb \\
    --device-type rtlsdr \\
    --gain -10 \\
    --ppm 0 \\
    --net \\
    --net-connector localhost,30003,beast_out \\
    --net-bo-port 30005 \\
    --net-json-port 30047 \\
    --net-api-port 8080 \\
    --json-location-accuracy 2 \\
    --lat ${READSB_LAT} \\
    --lon ${READSB_LON} \\
    --write-json /run/readsb \\
    --write-json-every 1 \\
    --quiet
Restart=always
RestartSec=5
Nice=-5

[Install]
WantedBy=multi-user.target
EOF

# -------------------------------------------------------------------
# Step 6: Create tmpfiles.d entry so /run/readsb survives reboot
# -------------------------------------------------------------------
cat > /etc/tmpfiles.d/readsb.conf << 'EOF'
d /run/readsb 0755 readsb readsb -
EOF

# -------------------------------------------------------------------
# Step 7: Enable and start (but don't fail if no dongle is plugged in)
# -------------------------------------------------------------------
info "Step 7: Enabling readsb service..."

systemctl daemon-reload
systemctl enable readsb.service

# Check if the RTL-SDR dongle is present
if rtl_test -t 2>&1 | grep -q "Found 1 device"; then
    info "RTL-SDR dongle detected! Starting readsb..."
    systemctl start readsb.service
    sleep 3

    if systemctl is-active --quiet readsb.service; then
        info "readsb is running!"
    else
        warn "readsb failed to start. Check: journalctl -u readsb"
    fi
else
    warn "No RTL-SDR dongle detected. readsb is enabled but not started."
    warn "Plug in the Nooelec NESDR Mini and run: sudo systemctl start readsb"
fi

# -------------------------------------------------------------------
# Cleanup
# -------------------------------------------------------------------
rm -rf "$BUILD_DIR"

# -------------------------------------------------------------------
# Summary
# -------------------------------------------------------------------
echo
info "=== Setup Complete ==="
echo
echo "  Service:     readsb.service (enabled, starts on boot)"
echo "  JSON output: http://localhost:8080/data/aircraft.json"
echo "  Also at:     /run/readsb/aircraft.json"
echo "  Logs:        journalctl -u readsb -f"
echo "  Live view:   viewadsb"
echo
if [[ "$READSB_LAT" == "0" && "$READSB_LON" == "0" ]]; then
    echo "  Location not configured. To set it later:"
    echo "    Edit /etc/systemd/system/readsb.service"
    echo "    Set --lat and --lon to your coordinates"
    echo "    sudo systemctl daemon-reload && sudo systemctl restart readsb"
else
    echo "  Location:    lat=${READSB_LAT}, lon=${READSB_LON}"
fi
echo
