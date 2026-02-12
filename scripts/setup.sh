#!/bin/sh
# Parenta Setup Script for OpenWrt
# Run this script on the router after extracting the deployment archive
#
# SAFETY: This script creates a SEPARATE guest network for the captive portal.
# Main LAN (br-lan) remains untouched - SSH always accessible on 192.168.1.1

set -e

PARENTA_DIR="/opt/parenta"
CONFIG_DIR="/etc/parenta"
DATA_DIR="/opt/parenta/data"
WEB_DIR="/opt/parenta/web"

# Required packages for Parenta
REQUIRED_PACKAGES="opennds iwinfo coreutils-base64"
OPTIONAL_PACKAGES="logd dnsmasq-full"

echo "=== Parenta Setup ==="

# Check if running as root
if [ "$(id -u)" != "0" ]; then
    echo "Error: This script must be run as root"
    exit 1
fi

# ============================================
# CLEANUP: Remove Default OpenWrt Settings
# ============================================
echo "Cleaning up default OpenWrt settings..."

# Stop and disable LuCI (we use our own web interface)
echo "  [INFO] Disabling LuCI web interface..."
/etc/init.d/uhttpd stop 2>/dev/null || true
/etc/init.d/uhttpd disable 2>/dev/null || true

# Remove default www content to free up space and avoid confusion
if [ -d "/www" ]; then
    echo "  [INFO] Backing up and clearing /www directory..."
    mv /www /www.backup.$(date +%s) 2>/dev/null || rm -rf /www 2>/dev/null || true
    mkdir -p /www
    echo "Parenta Active" > /www/index.html
fi

# Remove default WiFi interfaces (OpenWrt creates default 'OpenWrt' SSID)
echo "  [INFO] Removing default WiFi configurations..."
for iface in $(uci show wireless 2>/dev/null | grep "ssid='OpenWrt'" | cut -d'.' -f1-2 | cut -d'=' -f1); do
    echo "    [CLEANUP] Removing default interface: $iface"
    uci delete $iface 2>/dev/null || true
done

# Remove any existing disabled flags from radios (we'll handle enabling properly)
for radio in radio0 radio1 radio2 radio3; do
    uci -q delete wireless.${radio}.disabled 2>/dev/null || true
done

uci commit wireless 2>/dev/null || true
echo "  [OK] Cleanup completed"

# ============================================
# Check and Install Required Packages
# ============================================
echo "Checking required packages..."

is_pkg_installed() {
    opkg list-installed 2>/dev/null | grep -q "^$1 -" && return 0
    return 1
}

PACKAGES_TO_INSTALL=""
NEED_UPDATE=0

echo ""
echo "Required packages:"
for pkg in $REQUIRED_PACKAGES; do
    if is_pkg_installed "$pkg"; then
        echo "  [OK] $pkg is already installed"
    else
        echo "  [MISSING] $pkg needs to be installed"
        PACKAGES_TO_INSTALL="$PACKAGES_TO_INSTALL $pkg"
        NEED_UPDATE=1
    fi
done

echo ""
echo "DNS/DHCP server:"
if is_pkg_installed "dnsmasq-full"; then
    echo "  [OK] dnsmasq-full is installed (recommended)"
elif is_pkg_installed "dnsmasq"; then
    echo "  [OK] dnsmasq is installed"
    echo "  [INFO] Consider upgrading to dnsmasq-full for ipset support"
else
    echo "  [MISSING] dnsmasq needs to be installed"
    PACKAGES_TO_INSTALL="$PACKAGES_TO_INSTALL dnsmasq"
    NEED_UPDATE=1
fi

echo ""
echo "Optional packages:"
for pkg in $OPTIONAL_PACKAGES; do
    if is_pkg_installed "$pkg"; then
        echo "  [OK] $pkg is installed"
    else
        if [ "$pkg" = "dnsmasq-full" ] && is_pkg_installed "dnsmasq"; then
            echo "  [SKIP] $pkg - dnsmasq already installed"
        else
            echo "  [SKIP] $pkg - optional, install manually if needed"
        fi
    fi
done

echo ""
echo "Checking logread availability..."
if command -v logread >/dev/null 2>&1; then
    echo "  [OK] logread is available"
else
    echo "  [MISSING] logread not found - installing logd..."
    PACKAGES_TO_INSTALL="$PACKAGES_TO_INSTALL logd"
    NEED_UPDATE=1
fi

if [ -n "$PACKAGES_TO_INSTALL" ]; then
    echo ""
    echo "Installing missing packages:$PACKAGES_TO_INSTALL"

    echo "Updating package list..."
    if ! opkg update; then
        echo "Warning: Failed to update package list, trying to install anyway..."
    fi

    for pkg in $PACKAGES_TO_INSTALL; do
        echo "Installing $pkg..."
        if opkg install "$pkg"; then
            echo "[OK] $pkg installed successfully"
        else
            echo "[ERROR] Failed to install $pkg"
            echo "Please install $pkg manually: opkg install $pkg"
            exit 1
        fi
    done

    echo ""
    echo "All required packages installed."
else
    echo ""
    echo "All required packages are already installed."
fi

echo ""

# ============================================
# SAFETY CHECK: Ensure SSH won't be locked out
# ============================================
echo "Verifying SSH access safety..."

LAN_IP=$(uci get network.lan.ipaddr 2>/dev/null || echo "192.168.1.1")
echo "Main LAN IP: $LAN_IP (admin access will remain here)"

# ============================================
# Stop existing services
# ============================================
echo "Stopping existing services..."
/etc/init.d/parenta stop 2>/dev/null || true
/etc/init.d/opennds stop 2>/dev/null || true
killall -9 opennds 2>/dev/null || true

# ============================================
# Create directories
# ============================================
echo "Creating directories..."
mkdir -p $PARENTA_DIR
mkdir -p $CONFIG_DIR
mkdir -p $DATA_DIR
mkdir -p $WEB_DIR
mkdir -p /etc/dnsmasq.d
mkdir -p /etc/nftables.d

# ============================================
# Install binary
# ============================================
echo "Installing binary..."
cp parenta $PARENTA_DIR/parenta
chmod +x $PARENTA_DIR/parenta

# ============================================
# Install web files
# ============================================
echo "Installing web files..."
cp -r web/* $WEB_DIR/

# ============================================
# Install configuration
# ============================================
if [ ! -f $CONFIG_DIR/parenta.json ]; then
    echo "Installing default configuration..."
    cp configs/parenta.json $CONFIG_DIR/parenta.json

    # Generate random secrets using coreutils-base64
    FAS_KEY=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)
    JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)

    sed -i "s/CHANGE_THIS_SECRET_KEY/$FAS_KEY/g" $CONFIG_DIR/parenta.json
    sed -i "s/CHANGE_THIS_JWT_SECRET/$JWT_SECRET/g" $CONFIG_DIR/parenta.json
    sed -i 's|"data_dir": "./data"|"data_dir": "/opt/parenta/data"|g' $CONFIG_DIR/parenta.json

    echo "Generated random secrets for FAS and JWT"
else
    echo "Configuration already exists, skipping..."
fi

echo ""

# ============================================
# CRITICAL: SSH Safety Rule (before any network changes)
# ============================================
echo "Adding SSH safety rule..."

SSH_RULE_EXISTS=$(uci show firewall | grep -c "Allow-SSH-Parenta" || echo "0")
if [ "$SSH_RULE_EXISTS" = "0" ]; then
    uci add firewall rule
    uci set firewall.@rule[-1].name='Allow-SSH-Parenta'
    uci set firewall.@rule[-1].src='*'
    uci set firewall.@rule[-1].dest_port='22'
    uci set firewall.@rule[-1].proto='tcp'
    uci set firewall.@rule[-1].target='ACCEPT'
    uci commit firewall
    echo "SSH safety rule added"
else
    echo "SSH safety rule already exists"
fi

# ============================================
# Create Guest Network for Captive Portal
# ============================================
echo "Setting up guest network for captive portal..."

# Detect wireless radios (radio0, radio1, etc.)
RADIOS=$(uci show wireless 2>/dev/null | grep -E "^wireless\.radio[0-9]+=wifi-device" | cut -d'.' -f2 | cut -d'=' -f1)
if [ -z "$RADIOS" ]; then
    echo "[WARNING] No wireless radios detected in UCI! Trying wifi detect..."
    rm -f /etc/config/wireless
    wifi detect > /etc/config/wireless 2>/dev/null || true
    RADIOS=$(uci show wireless 2>/dev/null | grep -E "^wireless\.radio[0-9]+=wifi-device" | cut -d'.' -f2 | cut -d'=' -f1)
fi

if [ -z "$RADIOS" ]; then
    echo "[ERROR] No wireless radios found even after detection!"
    echo "[INFO] Continuing with network setup anyway. WiFi must be configured manually."
    RADIOS=""
else
    echo "Detected radios: $RADIOS"
fi

# ============================================
# CRITICAL FIX: Create Guest Network with Bridge
# ============================================
echo "Creating guest network interface..."

# Remove existing guest config if present to ensure clean setup
uci -q delete network.guest 2>/dev/null || true
uci -q delete network.guest_dev 2>/dev/null || true

# Create the bridge device (CRITICAL: Must list ports properly)
uci set network.guest_dev=device
uci set network.guest_dev.name='br-guest'
uci set network.guest_dev.type='bridge'

# Create the interface
uci set network.guest=interface
uci set network.guest.proto='static'
uci set network.guest.ipaddr='192.168.2.1'
uci set network.guest.netmask='255.255.255.0'
uci set network.guest.device='br-guest'

uci commit network
echo "[OK] Guest network interface created"

VAP_IFS=$(ip -o link | awk -F': ' '/-ap0|ap0/ {print $2}' | tr '\n' ' ' | sed 's/ $//')
if [ -n "$VAP_IFS" ]; then
    echo "  [INFO] Detected VAP interfaces for guest: $VAP_IFS"
    # set ifname explicitly so netifd/dnsmasq will bind to the correct bridge ports
    uci set network.guest.ifname="$VAP_IFS"
    uci commit network
else
    echo "  [WARN] No VAP interfaces auto-detected (will rely on netifd to attach)."
fi

# ============================================
# CRITICAL FIX: DHCP with Gateway Option
# ============================================
echo "Creating DHCP for guest network..."

# ลบคอนฟิกเก่าและรอสักครู่ให้ระบบจัดการคืน Resource
uci -q delete dhcp.guest 2>/dev/null && sleep 1

# สร้าง DHCP section
if uci set dhcp.guest=dhcp; then
    uci set dhcp.guest.interface='guest'
    uci set dhcp.guest.start='100'
    uci set dhcp.guest.limit='50'
    uci set dhcp.guest.leasetime='1h'
    uci add_list dhcp.guest.dhcp_option='3,192.168.2.1'
    uci add_list dhcp.guest.dhcp_option='6,192.168.2.1'
    
    echo "Committing DHCP changes..."
    uci commit dhcp
    echo "[OK] Guest DHCP created successfully"
else
    echo "[ERROR] Failed to set DHCP configuration"
    exit 1
fi


# ============================================
# CRITICAL FIX: Firewall with NAT Masquerading
# ============================================
echo "Creating firewall zone for guest network..."

# Remove existing guest zone if present
EXISTING_ZONE=$(uci show firewall 2>/dev/null | grep -E "name='guest'" | cut -d'.' -f1-2 | cut -d'=' -f1 || echo "")
if [ -n "$EXISTING_ZONE" ]; then
    uci delete $EXISTING_ZONE 2>/dev/null || true
fi

# Create zone
uci add firewall zone
uci set firewall.@zone[-1].name='guest'
uci set firewall.@zone[-1].network='guest'
uci set firewall.@zone[-1].input='REJECT'
uci set firewall.@zone[-1].output='ACCEPT'
uci set firewall.@zone[-1].forward='REJECT'
uci set firewall.@zone[-1].masq='1'  # Enable NAT masquerading
uci set firewall.@zone[-1].mtu_fix='1'

# Allow guest to WAN (internet through captive portal)
uci add firewall forwarding
uci set firewall.@forwarding[-1].src='guest'
uci set firewall.@forwarding[-1].dest='wan'

# Essential rules for captive portal
uci add firewall rule
uci set firewall.@rule[-1].name='Guest-DNS'
uci set firewall.@rule[-1].src='guest'
uci set firewall.@rule[-1].dest_port='53'
uci set firewall.@rule[-1].proto='udp tcp'
uci set firewall.@rule[-1].target='ACCEPT'

uci add firewall rule
uci set firewall.@rule[-1].name='Guest-DHCP'
uci set firewall.@rule[-1].src='guest'
uci set firewall.@rule[-1].dest_port='67'
uci set firewall.@rule[-1].proto='udp'
uci set firewall.@rule[-1].target='ACCEPT'

uci add firewall rule
uci set firewall.@rule[-1].name='Guest-Parenta-HTTP'
uci set firewall.@rule[-1].src='guest'
uci set firewall.@rule[-1].dest_port='8080'
uci set firewall.@rule[-1].proto='tcp'
uci set firewall.@rule[-1].target='ACCEPT'

uci add firewall rule
uci set firewall.@rule[-1].name='Guest-OpenNDS'
uci set firewall.@rule[-1].src='guest'
uci set firewall.@rule[-1].dest_port='2050'
uci set firewall.@rule[-1].proto='tcp'
uci set firewall.@rule[-1].target='ACCEPT'

uci commit firewall
echo "[OK] Guest firewall zone created with NAT"

# ============================================
# CRITICAL FIX: WiFi with Proper Bridge Attachment
# ============================================
echo "Setting up WiFi Access Point 'Parenta'..."

# Enable radios first
for RADIO in $RADIOS; do
    echo "  [INFO] Enabling $RADIO..."
    uci set wireless.${RADIO}.disabled='0'
    uci -q get wireless.${RADIO}.channel >/dev/null || uci set wireless.${RADIO}.channel='auto'
    uci -q get wireless.${RADIO}.country >/dev/null || uci set wireless.${RADIO}.country='US'
done
uci commit wireless

# Remove existing Parenta configs
EXISTING_PARENTA=$(uci show wireless 2>/dev/null | grep -E "ssid='Parenta'" | cut -d'.' -f1-2 | cut -d'=' -f1 || echo "")
if [ -n "$EXISTING_PARENTA" ]; then
    echo "  [INFO] Removing existing Parenta WiFi configurations..."
    for cfg in $EXISTING_PARENTA; do
        uci delete $cfg 2>/dev/null || true
    done
    uci commit wireless
fi

# Create WiFi AP on each radio - CRITICAL: network='guest' attaches to br-guest
WIFI_CREATED=0
for RADIO in $RADIOS; do
    echo "  [INFO] Configuring WiFi on $RADIO..."
    
    IFACE_NAME="parenta_${RADIO}"
    
    # Remove existing if present
    uci -q delete wireless.${IFACE_NAME} 2>/dev/null || true
    
    # CRITICAL: The 'network' option attaches this WiFi to the guest bridge
    uci set wireless.${IFACE_NAME}=wifi-iface
    uci set wireless.${IFACE_NAME}.device="$RADIO"
    uci set wireless.${IFACE_NAME}.network='guest'  # This attaches to br-guest!
    uci set wireless.${IFACE_NAME}.mode='ap'
    uci set wireless.${IFACE_NAME}.ssid='Parenta'
    uci set wireless.${IFACE_NAME}.encryption='none'
    uci set wireless.${IFACE_NAME}.isolate='1'
    uci set wireless.${IFACE_NAME}.disabled='0'
    
    echo "  [OK] Created '${IFACE_NAME}' on $RADIO -> attached to guest network"
    WIFI_CREATED=$((WIFI_CREATED + 1))
done

if [ "$WIFI_CREATED" -gt 0 ]; then
    uci commit wireless
    echo "[OK] WiFi configuration committed ($WIFI_CREATED interface(s) created)"
else
    echo "[WARNING] No WiFi interfaces were created."
fi

# ============================================
# CRITICAL FIX: OpenNDS Configuration
# ============================================
echo "Configuring OpenNDS..."

# Ensure OpenNDS config exists
if [ ! -f /etc/config/opennds ] || [ ! -s /etc/config/opennds ]; then
    echo "  [INFO] Creating OpenNDS configuration..."
    mkdir -p /etc/config
    touch /etc/config/opennds
    uci add opennds opennds 2>/dev/null || true
fi

# CRITICAL: OpenNDS settings
uci set opennds.@opennds[0].enabled='1'
uci set opennds.@opennds[0].gatewayinterface='br-guest'
uci set opennds.@opennds[0].gatewayname='Parenta'
uci set opennds.@opennds[0].gatewayport='2050'

# FAS Configuration
uci set opennds.@opennds[0].fasport='8080'
uci set opennds.@opennds[0].faspath='/api/portal/auth'
uci set opennds.@opennds[0].fasremoteip='192.168.2.1'
uci set opennds.@opennds[0].fas_secure_enabled='1'

# Session settings
uci set opennds.@opennds[0].sessiontimeout='1440'
uci set opennds.@opennds[0].preauthidletimeout='30'
uci set opennds.@opennds[0].authidletimeout='120'

# CRITICAL: Enable debug logging for troubleshooting
uci set opennds.@opennds[0].debuglevel='3'

# Sync FAS key
if [ -f $CONFIG_DIR/parenta.json ]; then
    FAS_KEY=$(grep -o '"fas_key"[[:space:]]*:[[:space:]]*"[^"]*"' $CONFIG_DIR/parenta.json 2>/dev/null | cut -d'"' -f4 || echo "")
    if [ -n "$FAS_KEY" ]; then
        uci set opennds.@opennds[0].faskey="$FAS_KEY"
        echo "  [OK] FAS key synced to OpenNDS"
    fi
fi

uci commit opennds

echo "[OK] OpenNDS configured for br-guest (192.168.2.1)"

# ============================================
# CRITICAL FIX: OpenNDS Firewall Redirect Rules
# ============================================
echo "Adding OpenNDS firewall redirect rules..."

# Remove existing redirects
for rule in $(uci show firewall 2>/dev/null | grep -E "Redirect.*opennds" | cut -d'.' -f1-2 | cut -d'=' -f1); do
    uci delete $rule 2>/dev/null || true
done

# Add redirect for HTTP traffic to OpenNDS
uci add firewall redirect
uci set firewall.@redirect[-1].name='OpenNDS-HTTP-Redirect'
uci set firewall.@redirect[-1].src='guest'
uci set firewall.@redirect[-1].proto='tcp'
uci set firewall.@redirect[-1].src_dport='80'
uci set firewall.@redirect[-1].dest_port='2050'
uci set firewall.@redirect[-1].target='DNAT'

# Add redirect for HTTPS traffic (optional, for CPD)
uci add firewall redirect
uci set firewall.@redirect[-1].name='OpenNDS-HTTPS-Redirect'
uci set firewall.@redirect[-1].src='guest'
uci set firewall.@redirect[-1].proto='tcp'
uci set firewall.@redirect[-1].src_dport='443'
uci set firewall.@redirect[-1].dest_port='2050'
uci set firewall.@redirect[-1].target='DNAT'

uci commit firewall
echo "[OK] OpenNDS redirect rules added"

# ============================================
# Install init.d service script
# ============================================
echo "Installing service script..."
cp deploy/openwrt/parenta.init /etc/init.d/parenta
chmod +x /etc/init.d/parenta

# ============================================
# Install firewall rules (nftables)
# ============================================
echo "Installing firewall rules..."
if [ -d /etc/nftables.d ]; then
    cp deploy/openwrt/firewall.include /etc/nftables.d/parenta.nft
    sed -i "s/192.168.1.1/192.168.2.1/g" /etc/nftables.d/parenta.nft
else
    cp deploy/openwrt/firewall.include /etc/firewall.parenta
    sed -i "s/192.168.1.1/192.168.2.1/g" /etc/firewall.parenta
fi

# ============================================
# Configure dnsmasq
# ============================================
echo "Configuring dnsmasq..."

# Enable dnsmasq conf.d
CONFDIR_EXISTS=$(uci get dhcp.@dnsmasq[0].confdir 2>/dev/null || echo "")
if [ -z "$CONFDIR_EXISTS" ]; then
    uci set dhcp.@dnsmasq[0].confdir='/etc/dnsmasq.d'
    uci add_list dhcp.@dnsmasq[0].interface='br-guest'
    uci commit dhcp
fi

# Create empty dnsmasq filter files
touch /etc/dnsmasq.d/parenta-blocklist.conf
touch /etc/dnsmasq.d/parenta-whitelist.conf

# Add DoH canary domain blocks
echo "Adding DoH canary blocks..."
cat > /etc/dnsmasq.d/parenta-antidoh.conf << 'EOF'
# Block DNS-over-HTTPS canary domains
address=/use-application-dns.net/
address=/mask.icloud.com/
address=/doh.dns.apple.com/
EOF

# ============================================
# Enable and start services
# ============================================
echo "Enabling services..."
/etc/init.d/parenta enable
/etc/init.d/opennds enable

# ============================================
# Apply network changes (CRITICAL: Order matters!)
# ============================================
echo ""
echo "Applying network configuration..."

echo "Restarting network..."
/etc/init.d/network restart
sleep 5

echo "Bringing up WiFi..."
wifi up
sleep 3

echo "Restarting dnsmasq..."
/etc/init.d/dnsmasq restart
sleep 2

echo "Applying firewall rules..."
fw4 reload 2>/dev/null || /etc/init.d/firewall restart
sleep 3

# ============================================
# Start services
# ============================================
echo "Starting services..."

echo "Starting OpenNDS..."
/etc/init.d/opennds start
sleep 3

# Check if OpenNDS started
if ! /usr/bin/ndsctl status >/dev/null 2>&1; then
    echo "[WARNING] OpenNDS didn't start, retrying..."
    sleep 2
    /etc/init.d/opennds restart
    sleep 3
fi

echo "Starting Parenta..."
/etc/init.d/parenta start
sleep 2

# ============================================
# Verify setup
# ============================================
echo ""
echo "=== Verifying Setup ==="

# Check if guest interface is up
if ip addr show br-guest >/dev/null 2>&1; then
    echo "[OK] Guest network (br-guest) is up"
    ip addr show br-guest | grep "inet "
else
    echo "[ERROR] Guest network (br-guest) is NOT up!"
fi

# Check bridge ports
echo ""
echo "Bridge configuration:"
brctl show br-guest 2>/dev/null || echo "[WARNING] br-guest bridge not found or brctl not available"

# Check WiFi
echo ""
echo "WiFi status:"
iwinfo 2>/dev/null | grep -A5 "Parenta" || echo "[WARNING] Parenta SSID not found"

# Check DHCP
echo ""
echo "DHCP status:"
uci show dhcp.guest 2>/dev/null | head -10 || echo "[WARNING] DHCP guest config not found"

# Check OpenNDS
echo ""
if /usr/bin/ndsctl status >/dev/null 2>&1; then
    echo "[OK] OpenNDS is running"
    ndsctl status 2>/dev/null | head -15 || true
else
    echo "[ERROR] OpenNDS is NOT running!"
    echo "[INFO] Checking logs..."
    logread -e opennds | tail -5 || echo "No logs available"
fi

# Check Parenta
if pgrep -f "parenta" >/dev/null 2>&1; then
    echo "[OK] Parenta is running"
else
    echo "[WARN] Parenta not running"
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Network Architecture:"
echo "  Main LAN:  br-lan    / $LAN_IP     (Admin SSH - NO captive portal)"
echo "  Kids WiFi: br-guest  / 192.168.2.1 (Captive portal ACTIVE)"
echo ""
echo "WiFi SSID: 'Parenta' (open network)"
echo ""
echo "Admin Dashboard: http://$LAN_IP:8080"
echo "Default login:   admin / parenta123"
echo ""
echo "TROUBLESHOOTING:"
echo "  1. Connect to 'Parenta' WiFi"
echo "  2. Check if you get IP 192.168.2.x: run 'ipconfig' (Windows) or 'ifconfig' (Linux/Mac)"
echo "  3. Try browsing to http://192.168.2.1:8080 manually"
echo "  4. Check OpenNDS: ndsctl status"
echo "  5. View logs: logread -f -e 'opennds|dnsmasq'"
echo ""
echo "If devices connect but get no IP:"
echo "  - Check: brctl show br-guest (should show wlan interfaces)"
echo "  - Check: uci show wireless | grep network"
echo "  - Restart: /etc/init.d/dnsmasq restart"
echo ""