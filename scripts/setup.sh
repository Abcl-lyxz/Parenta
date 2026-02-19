#!/bin/sh
# =============================================================================
# Parenta Setup Script for OpenWrt (fw4/nftables)
# ONE-TIME factory setup for Xiaomi AX3000T
#
# This script configures:
# - Guest network (br-guest / 192.168.2.1/24)
# - WiFi AP "Parenta" (open, client isolation)
# - OpenNDS captive portal with FAS
# - dnsmasq DNS filtering
# - Firewall rules
# - Parenta Go backend
#
# Features:
# - Phase 0: Full cleanup of previous installs before configuring
# - Every command is error-checked with descriptive messages
# - Safe to re-run multiple times (idempotent)
# - Full log saved to /tmp/parenta-setup.log
#
# Run this ONCE from factory-reset OpenWrt installation.
# =============================================================================

# =============================================================================
# STRICT ERROR HANDLING
# =============================================================================
# We do NOT use `set -e` globally because we need granular error handling.
# Instead, every command is wrapped with run_cmd() which tracks failures.

TOTAL_ERRORS=0
TOTAL_WARNINGS=0
CURRENT_PHASE=""
LOG_FILE="/tmp/parenta-setup.log"

# =============================================================================
# CONSTANTS
# =============================================================================
GATEWAY_IP="192.168.2.1"
GATEWAY_NETMASK="255.255.255.0"
PARENTA_PORT="8080"
OPENNDS_PORT="2050"
WIFI_SSID="Parenta"

PARENTA_DIR="/opt/parenta"
CONFIG_DIR="/etc/parenta"
DATA_DIR="/opt/parenta/data"
WEB_DIR="/opt/parenta/web"
DNSMASQ_CONFDIR="/etc/dnsmasq.d"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# =============================================================================
# LOGGING & ERROR HANDLING FUNCTIONS
# =============================================================================
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
    echo "[INFO] $1" >> "$LOG_FILE"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    echo "[WARN] $1" >> "$LOG_FILE"
    TOTAL_WARNINGS=$((TOTAL_WARNINGS + 1))
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    echo "[ERROR] $1" >> "$LOG_FILE"
    TOTAL_ERRORS=$((TOTAL_ERRORS + 1))
}

log_step() {
    echo -e "${CYAN}  -> ${NC}$1"
    echo "  -> $1" >> "$LOG_FILE"
}

# run_cmd "description" command [args...]
# Runs a command, logs success/failure, tracks errors.
# Returns 0 on success, 1 on failure (does NOT exit).
run_cmd() {
    local DESC="$1"
    shift

    log_step "$DESC"

    OUTPUT=$( eval "$@" 2>&1 )
    local RC=$?

    if [ $RC -ne 0 ]; then
        log_error "$DESC — FAILED (exit code: $RC)"
        if [ -n "$OUTPUT" ]; then
            echo -e "${RED}         Output: ${NC}$OUTPUT"
            echo "         Output: $OUTPUT" >> "$LOG_FILE"
        fi
        return 1
    else
        if [ -n "$OUTPUT" ]; then
            echo "         Output: $OUTPUT" >> "$LOG_FILE"
        fi
        return 0
    fi
}

# run_cmd_critical "description" command [args...]
# Same as run_cmd but EXITS the script on failure.
run_cmd_critical() {
    if ! run_cmd "$@"; then
        log_error "CRITICAL FAILURE in $CURRENT_PHASE — cannot continue."
        log_error "Check log: $LOG_FILE"
        exit 1
    fi
}

# run_cmd_optional "description" command [args...]
# Same as run_cmd but downgrades failure to warning (no error count).
run_cmd_optional() {
    local DESC="$1"
    shift
    log_step "$DESC"
    OUTPUT=$( eval "$@" 2>&1 )
    local RC=$?
    if [ $RC -ne 0 ]; then
        echo -e "${YELLOW}  -> ${NC}$DESC — skipped"
        echo "  -> $DESC — skipped (rc=$RC)" >> "$LOG_FILE"
        return 1
    fi
    return 0
}

# verify_file "path" "description"
# Checks that a file exists after creation.
verify_file() {
    if [ ! -f "$1" ]; then
        log_error "File not created: $1 ($2)"
        return 1
    else
        log_step "Created: $1"
        return 0
    fi
}

# =============================================================================
# PHASE 0: CLEANUP (Remove ALL previous Parenta configuration)
# =============================================================================
phase0_cleanup() {
    CURRENT_PHASE="Phase 0: Cleanup"
    log_info "=== Phase 0: Cleanup Previous Installation ==="
    log_info "Removing any existing Parenta config to start fresh..."

    # --- Stop services (don't error-check, they may not exist) ---
    log_step "Stopping existing services..."
    /etc/init.d/parenta stop 2>/dev/null
    /etc/init.d/opennds stop 2>/dev/null

    # --- Clean UCI: network ---
    log_step "Cleaning network config..."
    uci -q delete network.guest_dev 2>/dev/null
    uci -q delete network.guest 2>/dev/null
    uci commit network 2>/dev/null

    # --- Clean UCI: dhcp ---
    log_step "Cleaning DHCP config..."
    uci -q delete dhcp.guest 2>/dev/null
    uci -q delete dhcp.@dnsmasq[0].confdir 2>/dev/null
    uci -q delete dhcp.@dnsmasq[0].interface 2>/dev/null
    uci commit dhcp 2>/dev/null

    # --- Clean UCI: wireless (remove all Parenta and default SSIDs) ---
    log_step "Cleaning wireless config..."
    # Remove by SSID name
    for IFACE in $(uci show wireless 2>/dev/null | grep "ssid='$WIFI_SSID'" | cut -d'.' -f1-2 | cut -d'=' -f1); do
        log_step "  Removing WiFi interface: $IFACE"
        uci delete "$IFACE" 2>/dev/null
    done
    # Remove by naming convention parenta_radioX
    for IFACE in $(uci show wireless 2>/dev/null | grep -E "^wireless\.parenta_" | cut -d'.' -f2 | cut -d'=' -f1 | sort -u); do
        log_step "  Removing WiFi interface: wireless.$IFACE"
        uci delete "wireless.$IFACE" 2>/dev/null
    done
    # Remove default OpenWrt SSIDs
    for IFACE in $(uci show wireless 2>/dev/null | grep "ssid='OpenWrt'" | cut -d'.' -f1-2 | cut -d'=' -f1); do
        log_step "  Removing default SSID: $IFACE"
        uci delete "$IFACE" 2>/dev/null
    done
    uci commit wireless 2>/dev/null

    # --- Clean UCI: opennds ---
    log_step "Cleaning OpenNDS config..."
    if [ -f /etc/config/opennds ]; then
        echo "" > /etc/config/opennds
    fi

    # --- Clean UCI: firewall (remove all Parenta/guest rules) ---
    log_step "Cleaning firewall config..."
    # Remove guest zones (loop until none left)
    while true; do
        ZONE=$(uci show firewall 2>/dev/null | grep "name='guest'" | head -1 | cut -d'.' -f1-2 | cut -d'=' -f1)
        [ -z "$ZONE" ] && break
        log_step "  Removing zone: $ZONE"
        uci delete "$ZONE" 2>/dev/null
    done
    # Remove guest forwardings
    while true; do
        FWD=$(uci show firewall 2>/dev/null | grep "src='guest'" | head -1 | cut -d'.' -f1-2 | cut -d'=' -f1)
        [ -z "$FWD" ] && break
        log_step "  Removing forwarding: $FWD"
        uci delete "$FWD" 2>/dev/null
    done
    # Remove Parenta-specific firewall rules by name
    for RULE_NAME in 'Allow-SSH-Parenta' 'Guest-DNS' 'Guest-DHCP' 'Guest-Parenta' 'Guest-OpenNDS' 'Guest-Block-LAN' 'Guest-HTTP-out'; do
        while true; do
            RULE=$(uci show firewall 2>/dev/null | grep "name='$RULE_NAME'" | head -1 | cut -d'.' -f1-2 | cut -d'=' -f1)
            [ -z "$RULE" ] && break
            log_step "  Removing rule: $RULE_NAME"
            uci delete "$RULE" 2>/dev/null
        done
    done
    uci commit firewall 2>/dev/null

    # --- Clean filesystem ---
    log_step "Cleaning Parenta config files..."
    rm -f /etc/nftables.d/parenta.nft 2>/dev/null      # Old location (fw4 include)
    rm -f /etc/parenta/parenta.nft 2>/dev/null          # New location (standalone)
    rm -f "$DNSMASQ_CONFDIR/parenta-captive.conf" 2>/dev/null
    rm -f "$DNSMASQ_CONFDIR/parenta-blocklist.conf" 2>/dev/null
    rm -f "$DNSMASQ_CONFDIR/parenta-whitelist.conf" 2>/dev/null
    rm -f "$DNSMASQ_CONFDIR/parenta-studymode.conf" 2>/dev/null
    rm -f "$DNSMASQ_CONFDIR/parenta-antidoh.conf" 2>/dev/null

    # Clean rc.local entry
    if [ -f /etc/rc.local ]; then
        sed -i '/Parenta DNS protection/d' /etc/rc.local
    fi

    # --- Flush nftables table if loaded ---
    log_step "Flushing parenta nftables table..."
    nft delete table inet parenta 2>/dev/null

    log_info "Phase 0 complete — clean slate"
}

# =============================================================================
# PHASE 1: PRE-FLIGHT CHECKS
# =============================================================================
phase1_preflight() {
    CURRENT_PHASE="Phase 1: Pre-flight"
    log_info "=== Phase 1: Pre-flight Checks ==="

    # Must run as root
    if [ "$(id -u)" != "0" ]; then
        log_error "Must run as root"
        exit 1
    fi
    log_step "Running as root: OK"

    # Check OpenWrt
    if [ ! -f /etc/openwrt_release ]; then
        log_error "This does not appear to be an OpenWrt system!"
        exit 1
    fi
    OWT_VER=$(grep DISTRIB_RELEASE /etc/openwrt_release 2>/dev/null | cut -d= -f2 | tr -d "\"'")
    log_step "OpenWrt version: $OWT_VER"

    # Check for required packages
    log_info "Checking required packages..."
    REQUIRED_PACKAGES="opennds dnsmasq-full coreutils-base64"
    MISSING_PACKAGES=""

    for pkg in $REQUIRED_PACKAGES; do
        if ! opkg list-installed 2>/dev/null | grep -q "^$pkg "; then
            MISSING_PACKAGES="$MISSING_PACKAGES $pkg"
        else
            log_step "Package OK: $pkg"
        fi
    done

    if [ -n "$MISSING_PACKAGES" ]; then
        log_info "Installing missing packages:$MISSING_PACKAGES"
        run_cmd_critical "Update package list" "opkg update"

        # IMPORTANT: If dnsmasq-full is being installed, it may conflict with
        # existing dnsmasq (basic). We need to remove basic first.
        if echo "$MISSING_PACKAGES" | grep -q "dnsmasq-full"; then
            if opkg list-installed 2>/dev/null | grep -q "^dnsmasq "; then
                log_info "Removing basic dnsmasq before installing dnsmasq-full..."
                # Backup dhcp config — dnsmasq-full install may overwrite it
                cp /etc/config/dhcp /etc/config/dhcp.bak 2>/dev/null
                run_cmd "Remove basic dnsmasq" "opkg remove dnsmasq --force-removal-of-dependent-packages"
            fi
        fi

        for pkg in $MISSING_PACKAGES; do
            run_cmd "Install $pkg" "opkg install $pkg"
        done

        # Restore dhcp config if it was replaced by opkg
        if [ -f /etc/config/dhcp.bak ]; then
            if [ -f /etc/config/dhcp-opkg ]; then
                log_warn "dnsmasq-full overwrote dhcp config — restoring backup"
                mv /etc/config/dhcp.bak /etc/config/dhcp
                rm -f /etc/config/dhcp-opkg
            else
                rm -f /etc/config/dhcp.bak
            fi
        fi

        # Verify all installed
        for pkg in $REQUIRED_PACKAGES; do
            if ! opkg list-installed 2>/dev/null | grep -q "^$pkg "; then
                log_error "Package $pkg failed to install — cannot continue"
                exit 1
            fi
        done
    fi

    # Double-check dnsmasq-full vs dnsmasq basic (in case it wasn't in MISSING)
    if opkg list-installed 2>/dev/null | grep -q "^dnsmasq " && ! opkg list-installed 2>/dev/null | grep -q "^dnsmasq-full "; then
        log_warn "dnsmasq (basic) found — replacing with dnsmasq-full"
        cp /etc/config/dhcp /etc/config/dhcp.bak 2>/dev/null
        run_cmd "Remove basic dnsmasq" "opkg remove dnsmasq --force-removal-of-dependent-packages"
        run_cmd_critical "Install dnsmasq-full" "opkg install dnsmasq-full"
        if [ -f /etc/config/dhcp-opkg ]; then
            mv /etc/config/dhcp.bak /etc/config/dhcp
            rm -f /etc/config/dhcp-opkg
        else
            rm -f /etc/config/dhcp.bak
        fi
    fi

    # Disable LuCI (we use Parenta web interface instead)
    if [ -f /etc/init.d/uhttpd ]; then
        log_info "Disabling LuCI (uhttpd)..."
        run_cmd_optional "Stop uhttpd" "/etc/init.d/uhttpd stop"
        run_cmd_optional "Disable uhttpd" "/etc/init.d/uhttpd disable"
    fi

    log_info "Phase 1 complete"
}

# =============================================================================
# PHASE 2: NETWORK CONFIGURATION (br-guest)
# =============================================================================
phase2_network() {
    CURRENT_PHASE="Phase 2: Network"
    log_info "=== Phase 2: Network Configuration ==="

    # Create guest bridge device
    log_info "Creating guest bridge device..."
    run_cmd_critical "Create bridge device"    "uci set network.guest_dev=device"
    run_cmd_critical "Set bridge name"         "uci set network.guest_dev.name='br-guest'"
    run_cmd_critical "Set bridge type"         "uci set network.guest_dev.type='bridge'"

    # Create guest interface
    log_info "Creating guest interface..."
    run_cmd_critical "Create interface"        "uci set network.guest=interface"
    run_cmd_critical "Set proto=static"        "uci set network.guest.proto='static'"
    run_cmd_critical "Set device=br-guest"     "uci set network.guest.device='br-guest'"
    run_cmd_critical "Set IP=$GATEWAY_IP"      "uci set network.guest.ipaddr='$GATEWAY_IP'"
    run_cmd_critical "Set netmask"             "uci set network.guest.netmask='$GATEWAY_NETMASK'"

    run_cmd_critical "Commit network" "uci commit network"

    # Verify
    SAVED_IP=$(uci -q get network.guest.ipaddr)
    if [ "$SAVED_IP" = "$GATEWAY_IP" ]; then
        log_step "Verified: guest IP = $SAVED_IP"
    else
        log_error "Network verify failed! Expected $GATEWAY_IP, got '$SAVED_IP'"
        exit 1
    fi

    log_info "Phase 2 complete"
}

# =============================================================================
# PHASE 3: DHCP CONFIGURATION
# =============================================================================
phase3_dhcp() {
    CURRENT_PHASE="Phase 3: DHCP"
    log_info "=== Phase 3: DHCP Configuration ==="

    log_info "Creating DHCP pool for guest network..."
    run_cmd_critical "Create DHCP section"   "uci set dhcp.guest=dhcp"
    run_cmd_critical "Set interface=guest"    "uci set dhcp.guest.interface='guest'"
    run_cmd_critical "Set start=100"          "uci set dhcp.guest.start='100'"
    run_cmd_critical "Set limit=50"           "uci set dhcp.guest.limit='50'"
    run_cmd_critical "Set leasetime=1h"       "uci set dhcp.guest.leasetime='1h'"

    log_info "Setting DHCP options..."
    run_cmd_critical "Set gateway (opt 3)"          "uci add_list dhcp.guest.dhcp_option='3,$GATEWAY_IP'"
    run_cmd_critical "Set DNS server (opt 6)"       "uci add_list dhcp.guest.dhcp_option='6,$GATEWAY_IP'"
    run_cmd_critical "Set captive portal (opt 114)" "uci add_list dhcp.guest.dhcp_option='114,http://$GATEWAY_IP:$PARENTA_PORT/portal'"

    run_cmd_critical "Commit DHCP" "uci commit dhcp"

    # Verify
    DHCP_IFACE=$(uci -q get dhcp.guest.interface)
    if [ "$DHCP_IFACE" = "guest" ]; then
        log_step "Verified: DHCP interface = guest"
    else
        log_error "DHCP verify failed!"
        exit 1
    fi

    log_info "Phase 3 complete"
}

# =============================================================================
# PHASE 4: DNSMASQ CONFIGURATION
# =============================================================================
phase4_dnsmasq() {
    CURRENT_PHASE="Phase 4: dnsmasq"
    log_info "=== Phase 4: dnsmasq Configuration ==="

    run_cmd_critical "Create dnsmasq.d dir" "mkdir -p '$DNSMASQ_CONFDIR'"

    log_info "Configuring dnsmasq..."
    run_cmd_critical "Set confdir"                "uci set dhcp.@dnsmasq[0].confdir='$DNSMASQ_CONFDIR'"
    run_cmd_critical "Bind to br-guest"           "uci add_list dhcp.@dnsmasq[0].interface='br-guest'"
    run_cmd_critical "Bind to br-lan"             "uci add_list dhcp.@dnsmasq[0].interface='br-lan'"
    run_cmd_critical "Commit dnsmasq config"       "uci commit dhcp"

    # --- Config files ---
    log_info "Creating captive portal DNS config..."
    cat > "$DNSMASQ_CONFDIR/parenta-captive.conf" << EOF
# OpenNDS status.client resolution (redirect chain requirement)
address=/status.client/$GATEWAY_IP

# Local gateway FQDN (DO NOT use .local — iOS mDNS issue!)
address=/parenta.portal/$GATEWAY_IP
EOF
    verify_file "$DNSMASQ_CONFDIR/parenta-captive.conf" "captive portal DNS"

    log_info "Creating filter config files..."
    for CONF in parenta-blocklist.conf parenta-whitelist.conf parenta-studymode.conf; do
        touch "$DNSMASQ_CONFDIR/$CONF"
        verify_file "$DNSMASQ_CONFDIR/$CONF" "filter config"
    done

    log_info "Creating DoH blocker config..."
    cat > "$DNSMASQ_CONFDIR/parenta-antidoh.conf" << 'EOF'
# Block DNS-over-HTTPS canary domains to prevent filter bypass
address=/use-application-dns.net/
address=/mask.icloud.com/
address=/doh.dns.apple.com/
address=/dns.google/
address=/cloudflare-dns.com/
address=/dns.quad9.net/
EOF
    verify_file "$DNSMASQ_CONFDIR/parenta-antidoh.conf" "DoH blocker"

    log_info "Phase 4 complete"
}

# =============================================================================
# PHASE 5: WIFI CONFIGURATION
# =============================================================================
phase5_wifi() {
    CURRENT_PHASE="Phase 5: WiFi"
    log_info "=== Phase 5: WiFi Configuration ==="

    RADIOS=$(uci show wireless 2>/dev/null | grep -E "^wireless\.radio[0-9]+=wifi-device" | cut -d'.' -f2 | cut -d'=' -f1)

    if [ -z "$RADIOS" ]; then
        log_info "No radios found, running wifi detect..."
        rm -f /etc/config/wireless
        wifi detect > /etc/config/wireless 2>/dev/null
        RADIOS=$(uci show wireless 2>/dev/null | grep -E "^wireless\.radio[0-9]+=wifi-device" | cut -d'.' -f2 | cut -d'=' -f1)
    fi

    if [ -z "$RADIOS" ]; then
        log_error "No wireless radios found!"
        exit 1
    fi

    RADIO_COUNT=0
    for RADIO in $RADIOS; do
        RADIO_COUNT=$((RADIO_COUNT + 1))
        log_info "Configuring radio $RADIO_COUNT: $RADIO"

        run_cmd "Enable $RADIO"         "uci set wireless.${RADIO}.disabled='0'"
        run_cmd "Set channel (if unset)" "uci -q get wireless.${RADIO}.channel >/dev/null || uci set wireless.${RADIO}.channel='auto'"
        run_cmd "Set country=TH (if unset)" "uci -q get wireless.${RADIO}.country >/dev/null || uci set wireless.${RADIO}.country='TH'"

        # Auto-detect band and set HT mode for AX3000T performance
        CURRENT_HTMODE=$(uci -q get wireless.${RADIO}.htmode 2>/dev/null || echo "")
        if [ -z "$CURRENT_HTMODE" ]; then
            BAND=$(uci -q get wireless.${RADIO}.band 2>/dev/null || echo "")
            HWMODE=$(uci -q get wireless.${RADIO}.hwmode 2>/dev/null || echo "")
            if [ "$BAND" = "5g" ] || echo "$HWMODE" | grep -q "a"; then
                run_cmd "Set htmode=HE80 (5GHz)" "uci set wireless.${RADIO}.htmode='HE80'"
            else
                run_cmd "Set htmode=HE40 (2.4GHz)" "uci set wireless.${RADIO}.htmode='HE40'"
            fi
        else
            log_step "htmode already set: $CURRENT_HTMODE"
        fi

        # Remove ALL existing wifi-iface on this radio
        REMOVED=0
        for EI in $(uci show wireless 2>/dev/null | grep "device='$RADIO'" | cut -d'.' -f1-2 | cut -d'=' -f1); do
            IS=$(echo "$EI" | cut -d'.' -f2)
            run_cmd "Remove old iface: $IS" "uci delete wireless.$IS"
            REMOVED=$((REMOVED + 1))
        done
        [ $REMOVED -gt 0 ] && log_step "Removed $REMOVED old interface(s)"

        # Create Parenta WiFi interface
        IFACE_NAME="parenta_${RADIO}"
        log_info "Creating WiFi interface: $IFACE_NAME"
        run_cmd_critical "Create wifi-iface"       "uci set wireless.${IFACE_NAME}=wifi-iface"
        run_cmd_critical "Set device"              "uci set wireless.${IFACE_NAME}.device='$RADIO'"
        run_cmd_critical "Set network=guest"        "uci set wireless.${IFACE_NAME}.network='guest'"
        run_cmd_critical "Set mode=ap"             "uci set wireless.${IFACE_NAME}.mode='ap'"
        run_cmd_critical "Set ssid=$WIFI_SSID"     "uci set wireless.${IFACE_NAME}.ssid='$WIFI_SSID'"
        run_cmd_critical "Set encryption=none"      "uci set wireless.${IFACE_NAME}.encryption='none'"
        run_cmd_critical "Set client isolation"     "uci set wireless.${IFACE_NAME}.isolate='1'"
        run_cmd_critical "Enable interface"         "uci set wireless.${IFACE_NAME}.disabled='0'"
    done

    run_cmd_critical "Commit wireless" "uci commit wireless"

    # Verify
    SAVED=$(uci show wireless 2>/dev/null | grep -c "ssid='$WIFI_SSID'" || echo "0")
    if [ "$SAVED" -ge 1 ]; then
        log_step "Verified: $SAVED interface(s) with SSID '$WIFI_SSID'"
    else
        log_error "WiFi verify failed — no '$WIFI_SSID' found!"
    fi

    log_info "Phase 5 complete"
}

# =============================================================================
# PHASE 6: OPENNDS CONFIGURATION
# =============================================================================
phase6_opennds() {
    CURRENT_PHASE="Phase 6: OpenNDS"
    log_info "=== Phase 6: OpenNDS Configuration ==="

    # Ensure config section exists
    if ! uci -q get opennds.@opennds[0] >/dev/null 2>&1; then
        log_info "Creating OpenNDS config section..."
        mkdir -p /etc/config
        [ ! -f /etc/config/opennds ] && touch /etc/config/opennds
        run_cmd_critical "Create opennds section" "uci add opennds opennds"
    fi

    log_info "Configuring OpenNDS core..."
    run_cmd_critical "Enable"                      "uci set opennds.@opennds[0].enabled='1'"
    run_cmd_critical "Set gateway iface=br-guest"  "uci set opennds.@opennds[0].gatewayinterface='br-guest'"
    run_cmd_critical "Set gateway name"            "uci set opennds.@opennds[0].gatewayname='Parenta'"
    run_cmd_critical "Set gateway port"            "uci set opennds.@opennds[0].gatewayport='$OPENNDS_PORT'"

    log_info "Configuring FAS (Forward Auth)..."
    run_cmd_critical "Set FAS port"                "uci set opennds.@opennds[0].fasport='$PARENTA_PORT'"
    run_cmd_critical "Set FAS path"                "uci set opennds.@opennds[0].faspath='/fas/'"
    run_cmd_critical "Set FAS remote IP"           "uci set opennds.@opennds[0].fasremoteip='$GATEWAY_IP'"
    run_cmd_critical "Set FAS secure"              "uci set opennds.@opennds[0].fas_secure_enabled='1'"

    # Do NOT set gatewayfqdn with .local (iOS mDNS issue!)
    run_cmd_optional "Remove gatewayfqdn" "uci -q delete opennds.@opennds[0].gatewayfqdn"

    log_info "Configuring timeouts..."
    run_cmd_critical "Session timeout=1440 (24h)"  "uci set opennds.@opennds[0].sessiontimeout='1440'"
    run_cmd_critical "Preauth idle=30 min"         "uci set opennds.@opennds[0].preauthidletimeout='30'"
    run_cmd_critical "Auth idle=120 min"           "uci set opennds.@opennds[0].authidletimeout='120'"
    run_cmd_critical "Enable preemptive auth"      "uci set opennds.@opennds[0].allow_preemptive_authentication='1'"
    run_cmd_critical "Set debug=1"                 "uci set opennds.@opennds[0].debuglevel='1'"

    log_info "Configuring users_to_router (pre-auth access to gateway)..."
    # CRITICAL: Without this, OpenNDS ndsRTR chain will REJECT connections
    # to port 8080 from preauthenticated clients, making the login portal unreachable
    run_cmd_optional "Clear old users_to_router"    "uci -q delete opennds.@opennds[0].users_to_router"
    run_cmd_critical "Allow TCP 8080 to router"     "uci add_list opennds.@opennds[0].users_to_router='allow tcp port 8080'"

    log_info "Configuring walled garden..."
    # walledgarden_fqd_list: FQDNs that pre-auth clients can resolve and access
    # MUST include CPD (Captive Portal Detection) domains for each OS:
    #   - Apple: captive.apple.com
    #   - Google/Android: connectivitycheck.gstatic.com, clients3.google.com
    #   - Microsoft: www.msftconnecttest.com
    #   - Firefox: detectportal.firefox.com
    # Without these, the CPD popup shows "cannot connect to server"
    run_cmd_optional "Clear old walled garden FQDNs" "uci -q delete opennds.@opennds[0].walledgarden_fqd_list"
    run_cmd_critical "Add parenta.portal"       "uci add_list opennds.@opennds[0].walledgarden_fqd_list='parenta.portal'"
    run_cmd_critical "Add Apple CPD domains"    "uci add_list opennds.@opennds[0].walledgarden_fqd_list='captive.apple.com apple.com appleiphonecell.com ibook.info itools.info'"
    run_cmd_critical "Add Google CPD domains"   "uci add_list opennds.@opennds[0].walledgarden_fqd_list='connectivitycheck.gstatic.com clients3.google.com android.clients.google.com'"
    run_cmd_critical "Add Microsoft CPD domain" "uci add_list opennds.@opennds[0].walledgarden_fqd_list='www.msftconnecttest.com'"
    run_cmd_critical "Add Firefox CPD domain"   "uci add_list opennds.@opennds[0].walledgarden_fqd_list='detectportal.firefox.com'"

    # walledgarden_port_list: TCP ports that pre-auth clients can access
    # Format: SPACE-SEPARATED port numbers (NOT ip:port!)
    # MUST include 80 and 443:
    #   - iOS/Android CPD probes port 80 first (connectivitycheck, captive.apple.com)
    #   - If port 80 blocked → CPD popup shows "cannot connect to server"
    #   - 443 needed for HTTPS connectivity checks
    #   - 8080 = Parenta backend (login page)
    #   - 2050 = OpenNDS gateway port
    run_cmd_optional "Clear old walled garden ports"  "uci -q delete opennds.@opennds[0].walledgarden_port_list"
    run_cmd_critical "Add walled garden ports"  "uci add_list opennds.@opennds[0].walledgarden_port_list='80 443 $PARENTA_PORT $OPENNDS_PORT'"

    run_cmd_critical "Commit OpenNDS" "uci commit opennds"

    # Verify
    GW=$(uci -q get opennds.@opennds[0].gatewayinterface)
    EN=$(uci -q get opennds.@opennds[0].enabled)
    if [ "$GW" = "br-guest" ] && [ "$EN" = "1" ]; then
        log_step "Verified: OpenNDS enabled on br-guest"
    else
        log_error "OpenNDS verify failed! gw=$GW enabled=$EN"
    fi

    log_info "Phase 6 complete"
}

# =============================================================================
# PHASE 7: FIREWALL CONFIGURATION
# =============================================================================
phase7_firewall() {
    CURRENT_PHASE="Phase 7: Firewall"
    log_info "=== Phase 7: Firewall Configuration ==="

    # SSH safety rule (prevents lockout)
    log_info "Adding SSH safety rule..."
    SSH_EXISTS=$(uci show firewall 2>/dev/null | grep -c "Allow-SSH-Parenta" || echo "0")
    if [ "$SSH_EXISTS" = "0" ]; then
        run_cmd_critical "Create SSH rule"    "uci add firewall rule"
        run_cmd_critical "Name"              "uci set firewall.@rule[-1].name='Allow-SSH-Parenta'"
        run_cmd_critical "Src=*"             "uci set firewall.@rule[-1].src='*'"
        run_cmd_critical "Port=22"           "uci set firewall.@rule[-1].dest_port='22'"
        run_cmd_critical "Proto=tcp"         "uci set firewall.@rule[-1].proto='tcp'"
        run_cmd_critical "Target=ACCEPT"     "uci set firewall.@rule[-1].target='ACCEPT'"
    else
        log_step "SSH rule already exists"
    fi

    # Guest zone
    log_info "Creating guest zone..."
    run_cmd_critical "Create zone"         "uci add firewall zone"
    run_cmd_critical "Name=guest"          "uci set firewall.@zone[-1].name='guest'"
    run_cmd_critical "Network=guest"       "uci set firewall.@zone[-1].network='guest'"
    run_cmd_critical "Input=ACCEPT"        "uci set firewall.@zone[-1].input='ACCEPT'"
    run_cmd_critical "Output=ACCEPT"       "uci set firewall.@zone[-1].output='ACCEPT'"
    run_cmd_critical "Forward=REJECT"      "uci set firewall.@zone[-1].forward='REJECT'"
    run_cmd_critical "Masquerade=1"        "uci set firewall.@zone[-1].masq='1'"
    run_cmd_critical "MTU fix=1"           "uci set firewall.@zone[-1].mtu_fix='1'"

    # Guest → WAN forwarding
    log_info "Adding guest → WAN forwarding..."
    run_cmd_critical "Create forwarding"   "uci add firewall forwarding"
    run_cmd_critical "Src=guest"           "uci set firewall.@forwarding[-1].src='guest'"
    run_cmd_critical "Dest=wan"            "uci set firewall.@forwarding[-1].dest='wan'"

    # Helper function for firewall rules
    add_fw_rule() {
        local NAME="$1" SRC="$2" PORT="$3" PROTO="$4" TARGET="$5" DEST="${6:-}"
        run_cmd_critical "Create rule: $NAME"  "uci add firewall rule"
        run_cmd_critical "  name"              "uci set firewall.@rule[-1].name='$NAME'"
        run_cmd_critical "  src=$SRC"          "uci set firewall.@rule[-1].src='$SRC'"
        [ -n "$PORT" ]   && run_cmd_critical "  port=$PORT"    "uci set firewall.@rule[-1].dest_port='$PORT'"
        run_cmd_critical "  proto=$PROTO"      "uci set firewall.@rule[-1].proto='$PROTO'"
        run_cmd_critical "  target=$TARGET"    "uci set firewall.@rule[-1].target='$TARGET'"
        [ -n "$DEST" ]   && run_cmd_critical "  dest=$DEST"    "uci set firewall.@rule[-1].dest='$DEST'"
    }

    log_info "Adding service rules..."
    add_fw_rule "Guest-DNS"       "guest" "53"            "udp tcp" "ACCEPT"
    add_fw_rule "Guest-DHCP"      "guest" "67"            "udp"     "ACCEPT"
    add_fw_rule "Guest-Parenta"   "guest" "$PARENTA_PORT" "tcp"     "ACCEPT"
    add_fw_rule "Guest-OpenNDS"   "guest" "$OPENNDS_PORT" "tcp"     "ACCEPT"
    add_fw_rule "Guest-Block-LAN" "guest" ""              "all"     "REJECT" "lan"

    # HTTP/HTTPS out (OpenNDS intercept + browsing after auth)
    run_cmd_critical "Create HTTP-out rule"    "uci add firewall rule"
    run_cmd_critical "  name"                  "uci set firewall.@rule[-1].name='Guest-HTTP-out'"
    run_cmd_critical "  src=guest"             "uci set firewall.@rule[-1].src='guest'"
    run_cmd_critical "  dest=wan"              "uci set firewall.@rule[-1].dest='wan'"
    run_cmd_critical "  port=80 443"           "uci set firewall.@rule[-1].dest_port='80 443'"
    run_cmd_critical "  proto=tcp"             "uci set firewall.@rule[-1].proto='tcp'"
    run_cmd_critical "  target=ACCEPT"         "uci set firewall.@rule[-1].target='ACCEPT'"

    run_cmd_critical "Commit firewall" "uci commit firewall"

    # --- nftables rules for DNS protection ---
    # IMPORTANT: We CANNOT put our rules in /etc/nftables.d/ because fw4 includes
    # those files INSIDE its own `table inet fw4 { }` block, causing a
    # "nested table" syntax error. Instead we:
    # 1. Write rules to /etc/parenta/parenta.nft (our own location)
    # 2. Load them separately with `nft -f` after fw4 has started
    # 3. Add a line to /etc/rc.local to load on boot
    log_info "Creating nftables rules..."
    rm -f /etc/nftables.d/parenta.nft 2>/dev/null  # Remove old location if exists

    mkdir -p /etc/parenta
    cat > /etc/parenta/parenta.nft << 'NFTEOF'
# Parenta nftables rules — loaded SEPARATELY from fw4
# Do NOT place in /etc/nftables.d/ (fw4 nests includes inside its own table)

# Flush old rules if reloading
table inet parenta
delete table inet parenta

table inet parenta {

    set doh_servers {
        type ipv4_addr
        flags interval
        elements = {
            8.8.8.8, 8.8.4.4,
            1.1.1.1, 1.0.0.1,
            9.9.9.9, 149.112.112.112,
            208.67.222.222, 208.67.220.220,
            45.90.28.0/24, 45.90.30.0/24,
            94.140.14.14, 94.140.15.15,
            185.228.168.168, 185.228.169.168
        }
    }

    chain forward_guest {
        type filter hook forward priority filter + 5; policy accept;
        iifname "br-guest" jump guest_filter
    }

    chain guest_filter {
        tcp dport 853 counter reject with tcp reset comment "Block DoT"
        udp dport 853 counter drop comment "Block DoT UDP"
        ip daddr @doh_servers tcp dport 443 counter reject with tcp reset comment "Block DoH"
        udp dport 443 counter drop comment "Block QUIC/HTTP3"
    }

    chain dstnat_guest {
        type nat hook prerouting priority dstnat + 5; policy accept;
        iifname "br-guest" jump guest_dnat
    }

    chain guest_dnat {
        ip daddr != 192.168.2.1 udp dport 53 counter dnat to 192.168.2.1:53 comment "Redirect DNS UDP"
        ip daddr != 192.168.2.1 tcp dport 53 counter dnat to 192.168.2.1:53 comment "Redirect DNS TCP"
    }
}
NFTEOF
    verify_file "/etc/parenta/parenta.nft" "nftables rules"

    # Validate syntax
    if command -v nft >/dev/null 2>&1; then
        if nft -c -f /etc/parenta/parenta.nft 2>/dev/null; then
            log_step "nftables syntax check: OK"
        else
            log_warn "nftables syntax check failed!"
        fi
    fi

    # Add to rc.local so rules load on every boot AFTER fw4
    log_info "Adding nftables load to rc.local..."
    NFT_LOAD_CMD="nft -f /etc/parenta/parenta.nft  # Parenta DNS protection"
    if [ -f /etc/rc.local ]; then
        # Remove old entry if exists
        sed -i '/Parenta DNS protection/d' /etc/rc.local
        # Add before 'exit 0' if present, otherwise append
        if grep -q "^exit 0" /etc/rc.local; then
            sed -i "/^exit 0/i\\$NFT_LOAD_CMD" /etc/rc.local
        else
            echo "$NFT_LOAD_CMD" >> /etc/rc.local
        fi
    else
        echo "#!/bin/sh" > /etc/rc.local
        echo "$NFT_LOAD_CMD" >> /etc/rc.local
        echo "exit 0" >> /etc/rc.local
        chmod +x /etc/rc.local
    fi
    if grep -q "parenta.nft" /etc/rc.local; then
        log_step "rc.local updated — nftables will load on boot"
    else
        log_error "Failed to add nftables to rc.local"
    fi

    log_info "Phase 7 complete"
}

# =============================================================================
# PHASE 8: PARENTA DEPLOYMENT
# =============================================================================
phase8_parenta() {
    CURRENT_PHASE="Phase 8: Deploy"
    log_info "=== Phase 8: Parenta Deployment ==="

    run_cmd_critical "Create $PARENTA_DIR" "mkdir -p '$PARENTA_DIR'"
    run_cmd_critical "Create $CONFIG_DIR"  "mkdir -p '$CONFIG_DIR'"
    run_cmd_critical "Create $DATA_DIR"    "mkdir -p '$DATA_DIR'"
    run_cmd_critical "Create $WEB_DIR"     "mkdir -p '$WEB_DIR'"

    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
    log_step "Project dir: $PROJECT_DIR"

    # Binary
    if [ -f "$PROJECT_DIR/parenta" ]; then
        run_cmd_critical "Copy binary"         "cp '$PROJECT_DIR/parenta' '$PARENTA_DIR/parenta'"
        run_cmd_critical "Set executable"      "chmod +x '$PARENTA_DIR/parenta'"
        if [ -x "$PARENTA_DIR/parenta" ]; then
            log_step "Binary OK: $PARENTA_DIR/parenta"
        else
            log_error "Binary not executable!"
        fi
    elif [ -f "$PARENTA_DIR/parenta" ]; then
        log_step "Binary already exists"
    else
        log_warn "Binary not found — copy to $PARENTA_DIR/parenta manually"
    fi

    # Web files
    if [ -d "$PROJECT_DIR/web" ]; then
        run_cmd "Copy web files" "cp -r '$PROJECT_DIR/web/'* '$WEB_DIR/'"
        log_step "Web files: $(ls -1 "$WEB_DIR" 2>/dev/null | wc -l) file(s)"
    elif [ -d "$WEB_DIR" ] && [ "$(ls -A "$WEB_DIR" 2>/dev/null)" ]; then
        log_step "Web files already exist"
    else
        log_warn "Web files not found — copy to $WEB_DIR/ manually"
    fi

    # Config
    if [ ! -f "$CONFIG_DIR/parenta.json" ]; then
        if [ -f "$PROJECT_DIR/configs/parenta.json" ]; then
            run_cmd_critical "Copy config" "cp '$PROJECT_DIR/configs/parenta.json' '$CONFIG_DIR/parenta.json'"

            FAS_KEY=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)
            JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)

            if [ -z "$FAS_KEY" ] || [ -z "$JWT_SECRET" ]; then
                log_error "Failed to generate secrets!"
            else
                log_step "FAS key: ${FAS_KEY:0:8}..."
                log_step "JWT secret: ${JWT_SECRET:0:8}..."
            fi

            run_cmd "Inject FAS key"     "sed -i \"s|CHANGE_THIS_SECRET_KEY|$FAS_KEY|g\" '$CONFIG_DIR/parenta.json'"
            run_cmd "Inject JWT secret"  "sed -i \"s|CHANGE_THIS_JWT_SECRET|$JWT_SECRET|g\" '$CONFIG_DIR/parenta.json'"
            run_cmd "Set data_dir"       "sed -i 's|\"data_dir\": \"./data\"|\"data_dir\": \"/opt/parenta/data\"|g' '$CONFIG_DIR/parenta.json'"
            run_cmd "Set gateway_ip"     "sed -i 's|\"gateway_ip\": \".*\"|\"gateway_ip\": \"$GATEWAY_IP\"|g' '$CONFIG_DIR/parenta.json'"

            run_cmd_critical "Sync FAS key to OpenNDS" "uci set opennds.@opennds[0].faskey='$FAS_KEY'"
            run_cmd_critical "Commit FAS key"          "uci commit opennds"
        else
            log_warn "Config template not found"
        fi
    else
        log_step "Config already exists"
    fi

    # Init script
    if [ -f "$PROJECT_DIR/deploy/openwrt/parenta.init" ]; then
        run_cmd_critical "Copy init script"    "cp '$PROJECT_DIR/deploy/openwrt/parenta.init' /etc/init.d/parenta"
        run_cmd_critical "Set executable"      "chmod +x /etc/init.d/parenta"
    elif [ -f /etc/init.d/parenta ]; then
        log_step "Init script already exists"
    else
        log_warn "Init script not found — copy to /etc/init.d/parenta manually"
    fi

    log_info "Phase 8 complete"
}

# =============================================================================
# PHASE 9: SERVICE ACTIVATION
# =============================================================================
phase9_services() {
    CURRENT_PHASE="Phase 9: Services"
    log_info "=== Phase 9: Service Activation ==="

    log_info "Enabling services at boot..."
    run_cmd "Enable parenta" "/etc/init.d/parenta enable"
    run_cmd "Enable opennds" "/etc/init.d/opennds enable"

    # --- Restart sequence (ORDER IS CRITICAL) ---

    # 1. Network
    log_info "[1/6] Restarting network..."
    run_cmd_critical "Restart network" "/etc/init.d/network restart"
    sleep 5
    if ip addr show br-guest 2>/dev/null | grep -q "$GATEWAY_IP"; then
        log_step "br-guest UP: $GATEWAY_IP"
    else
        log_error "br-guest did not come up!"
        ip addr show >> "$LOG_FILE" 2>&1
    fi

    # 2. WiFi
    log_info "[2/6] Bringing up WiFi..."
    run_cmd "WiFi up" "wifi up"
    sleep 5

    # Check if WiFi interface joined br-guest
    # Multiple methods because not all OpenWrt builds have same tools
    log_info "Checking bridge membership..."
    check_bridge() {
        # Method 1: ip link show master (most common)
        if ip link show master br-guest 2>/dev/null | grep -qE "state UP|state UNKNOWN|state DORMANT"; then
            return 0
        fi
        # Method 2: check /sys/class/net (always available on Linux)
        if ls /sys/class/net/br-guest/brif/ 2>/dev/null | grep -q .; then
            return 0
        fi
        # Method 3: brctl (if bridge-utils installed)
        if command -v brctl >/dev/null 2>&1; then
            if brctl show br-guest 2>/dev/null | grep -v "bridge name" | grep -q .; then
                return 0
            fi
        fi
        return 1
    }

    RETRY=0
    while [ $RETRY -lt 6 ]; do
        if check_bridge; then
            MEMBERS=$(ls /sys/class/net/br-guest/brif/ 2>/dev/null | tr '\n' ' ')
            log_step "WiFi attached to br-guest: $MEMBERS"
            break
        fi
        RETRY=$((RETRY + 1))
        log_step "Waiting for bridge join... ($RETRY/6)"
        sleep 3
    done
    if [ $RETRY -eq 6 ]; then
        log_error "WiFi did NOT join br-guest after 18s!"
        log_error "br-guest members: $(ls /sys/class/net/br-guest/brif/ 2>/dev/null || echo 'NONE')"
        log_error "All interfaces:"
        ip link show 2>&1 | tee -a "$LOG_FILE"
        log_error "WiFi status:"
        iwinfo 2>&1 | tee -a "$LOG_FILE"
    fi

    # 3. dnsmasq
    log_info "[3/6] Restarting dnsmasq..."
    run_cmd "Restart dnsmasq" "/etc/init.d/dnsmasq restart"
    sleep 2
    if pgrep -x dnsmasq >/dev/null 2>&1; then
        log_step "dnsmasq running (PID $(pgrep -x dnsmasq | head -1))"
    else
        log_error "dnsmasq NOT running!"
    fi

    # 4. Firewall
    log_info "[4/6] Applying firewall..."
    # First reload fw4 (generates its own nftables rules)
    run_cmd "Reload fw4" "fw4 reload"
    sleep 2
    # THEN load our rules separately (must be AFTER fw4 to avoid conflict)
    if [ -f /etc/parenta/parenta.nft ]; then
        run_cmd "Load Parenta nftables" "nft -f /etc/parenta/parenta.nft"
    fi
    sleep 1
    if nft list table inet parenta >/dev/null 2>&1; then
        log_step "nftables 'inet parenta' loaded"
    else
        log_warn "nftables table not loaded — DNS protection inactive"
    fi

    # 5. OpenNDS
    log_info "[5/6] Starting OpenNDS..."
    /etc/init.d/opennds stop 2>/dev/null
    sleep 1
    run_cmd "Start OpenNDS" "/etc/init.d/opennds start"
    sleep 3
    if /usr/bin/ndsctl status >/dev/null 2>&1; then
        log_step "OpenNDS running on br-guest:$OPENNDS_PORT"
    else
        log_warn "OpenNDS not responding, retrying..."
        run_cmd "Restart OpenNDS" "/etc/init.d/opennds restart"
        sleep 3
        if /usr/bin/ndsctl status >/dev/null 2>&1; then
            log_step "OpenNDS started on retry"
        else
            log_error "OpenNDS failed! Check: logread | grep opennds"
        fi
    fi

    # 6. Parenta
    log_info "[6/6] Starting Parenta..."
    /etc/init.d/parenta stop 2>/dev/null
    sleep 1
    run_cmd "Start Parenta" "/etc/init.d/parenta start"
    sleep 2
    if pgrep -f "parenta" >/dev/null 2>&1; then
        log_step "Parenta running (PID $(pgrep -f parenta | head -1))"
    else
        log_error "Parenta failed! Check: logread | grep parenta"
    fi

    log_info "Phase 9 complete"
}

# =============================================================================
# PHASE 10: VERIFICATION
# =============================================================================
phase10_verify() {
    CURRENT_PHASE="Phase 10: Verify"
    log_info "=== Phase 10: Final Verification ==="

    V_PASS=0; V_FAIL=0; V_WARN=0

    check_pass() { log_info "[PASS] $1"; V_PASS=$((V_PASS + 1)); }
    check_fail() { log_error "[FAIL] $1"; V_FAIL=$((V_FAIL + 1)); }
    check_warn() { log_warn "[WARN] $1"; V_WARN=$((V_WARN + 1)); }

    verify() {
        if eval "$2" >/dev/null 2>&1; then check_pass "$1"; else check_fail "$1"; fi
    }
    verify_soft() {
        if eval "$2" >/dev/null 2>&1; then check_pass "$1"; else check_warn "$1"; fi
    }

    echo ""

    # Network
    verify      "br-guest UP at $GATEWAY_IP"                    "ip addr show br-guest 2>/dev/null | grep -q '$GATEWAY_IP'"
    verify      "WiFi in br-guest bridge"                       "ls /sys/class/net/br-guest/brif/ 2>/dev/null | grep -q ."
    verify_soft "SSID '$WIFI_SSID' broadcasting"                "iwinfo 2>/dev/null | grep -q '$WIFI_SSID'"

    # Services
    verify      "dnsmasq running"                               "pgrep -x dnsmasq"
    verify_soft "DHCP listening (:67)"                          "ss -ulnp 2>/dev/null | grep -q ':67 '"
    verify_soft "DNS listening (:53)"                           "ss -ulnp 2>/dev/null | grep -q ':53 '"
    verify      "OpenNDS running"                               "/usr/bin/ndsctl status"
    verify_soft "Parenta running"                               "pgrep -f parenta"

    # DNS
    verify_soft "status.client → $GATEWAY_IP"                   "nslookup status.client $GATEWAY_IP 2>/dev/null | grep -q '$GATEWAY_IP'"
    verify_soft "parenta.portal → $GATEWAY_IP"                  "nslookup parenta.portal $GATEWAY_IP 2>/dev/null | grep -q '$GATEWAY_IP'"

    # nftables
    verify_soft "nftables 'inet parenta' loaded"                "nft list table inet parenta"

    # Config files
    verify      "UCI: network.guest"                            "test \"\$(uci -q get network.guest.ipaddr)\" = '$GATEWAY_IP'"
    verify      "UCI: dhcp.guest"                               "test \"\$(uci -q get dhcp.guest.interface)\" = 'guest'"
    verify      "UCI: opennds enabled"                          "test \"\$(uci -q get opennds.@opennds[0].enabled)\" = '1'"
    verify      "UCI: guest firewall zone"                      "uci show firewall 2>/dev/null | grep -q \"name='guest'\""
    verify_soft "rc.local has nftables loader"                  "grep -q 'parenta.nft' /etc/rc.local 2>/dev/null"

    # Summary
    echo ""
    echo "=============================================="
    echo -e "  ${BOLD}PARENTA SETUP RESULTS${NC}"
    echo "=============================================="
    echo ""
    echo -e "  Checks:   ${GREEN}${V_PASS} passed${NC}  ${YELLOW}${V_WARN} warned${NC}  ${RED}${V_FAIL} failed${NC}"
    echo -e "  Setup:    ${RED}${TOTAL_ERRORS} error(s)${NC}  ${YELLOW}${TOTAL_WARNINGS} warning(s)${NC}"
    echo ""
    if [ $V_FAIL -eq 0 ] && [ $TOTAL_ERRORS -eq 0 ]; then
        echo -e "  ${GREEN}${BOLD}✓ ALL GOOD — Router is ready to ship!${NC}"
    elif [ $V_FAIL -eq 0 ]; then
        echo -e "  ${YELLOW}${BOLD}⚠ MOSTLY OK — $TOTAL_ERRORS error(s) during setup, but all checks passed${NC}"
    else
        echo -e "  ${RED}${BOLD}✗ ISSUES FOUND — $V_FAIL check(s) failed${NC}"
    fi
    echo ""
    echo "=============================================="
    echo "  Connection Info"
    echo "=============================================="
    echo ""
    echo "  Admin LAN:      192.168.1.1 (SSH always available)"
    echo "  Guest WiFi:     $GATEWAY_IP (captive portal)"
    echo "  Dashboard:      http://192.168.1.1:$PARENTA_PORT"
    echo "  Default login:  admin / parenta123"
    echo "  WiFi SSID:      $WIFI_SSID (open)"
    echo ""
    echo "  Test: Connect phone to '$WIFI_SSID' → captive portal pops up"
    echo ""
    echo "  Full log: $LOG_FILE"
    echo ""

    if [ $V_FAIL -gt 0 ] || [ $TOTAL_ERRORS -gt 0 ]; then
        echo "  Troubleshooting:"
        echo "    cat $LOG_FILE"
        echo "    logread | grep -E 'opennds|parenta|dnsmasq'"
        echo "    ip addr; ip link show master br-guest; iwinfo"
        echo "    /etc/init.d/opennds restart"
        echo "    /etc/init.d/parenta restart"
        echo "    reboot"
        echo ""
    fi
}

# =============================================================================
# MAIN
# =============================================================================
main() {
    echo "=== Parenta Setup — $(date) ===" > "$LOG_FILE"

    echo ""
    echo -e "${BOLD}==============================================${NC}"
    echo -e "${BOLD}  Parenta Setup Script for OpenWrt${NC}"
    echo -e "${BOLD}  Xiaomi AX3000T — Factory Configuration${NC}"
    echo -e "${BOLD}==============================================${NC}"
    echo ""

    phase0_cleanup
    phase1_preflight
    phase2_network
    phase3_dhcp
    phase4_dnsmasq
    phase5_wifi
    phase6_opennds
    phase7_firewall
    phase8_parenta
    phase9_services
    phase10_verify
}

main "$@"