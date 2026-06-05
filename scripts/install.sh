#!/bin/bash
# install.sh — Install the Zabbix Agent 2 Docker Swarm plugin
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/AdureIO/zabbix-agent2-plugin-docker-swarm/main/scripts/install.sh | sudo bash
#
# Options (environment variables):
#   VERSION=v1.0.6          Install a specific version (default: latest)
#   INSTALL_DIR=<path>      Plugin binary directory (default: /var/lib/zabbix/plugins)
#   CONF_DIR=<path>         Zabbix conf.d directory   (default: /etc/zabbix/zabbix_agent2.d)
#   SOCKET=/var/run/docker.sock  Docker socket path
#   NO_RESTART=1            Skip restarting zabbix-agent2
#   UNINSTALL=1             Remove the plugin instead of installing
#   DRY_RUN=1               Print actions without executing

set -euo pipefail

# ---------------------------------------------------------------------------
# Config (can be overridden by environment variables)
# ---------------------------------------------------------------------------
REPO="${REPO:-AdureIO/zabbix-agent2-plugin-docker-swarm}"
INSTALL_DIR="${INSTALL_DIR:-/var/lib/zabbix/plugins}"
CONF_DIR="${CONF_DIR:-/etc/zabbix/zabbix_agent2.d}"
SOCKET="${SOCKET:-/var/run/docker.sock}"
NO_RESTART="${NO_RESTART:-0}"
UNINSTALL="${UNINSTALL:-0}"
DRY_RUN="${DRY_RUN:-0}"

PLUGIN_NAME="docker-swarm"
PLUGIN_BIN="${INSTALL_DIR}/${PLUGIN_NAME}"
PLUGIN_CONF="${CONF_DIR}/docker-swarm.conf"

# ---------------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()     { echo -e "${RED}[ERROR]${NC} $*" >&2; }
step()    { echo -e "\n${BOLD}==>${NC} $*"; }
run()     {
    if [[ "$DRY_RUN" == "1" ]]; then
        echo -e "${YELLOW}[DRY-RUN]${NC} $*"
    else
        eval "$@"
    fi
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
check_root() {
    if [[ "$EUID" -ne 0 ]]; then
        err "This script must be run as root (use sudo)."
        exit 1
    fi
}

check_deps() {
    local missing=()
    for cmd in curl sha256sum; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        err "Missing required tools: ${missing[*]}"
        err "Install them with: apt-get install -y curl coreutils"
        exit 1
    fi
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)
            err "Unsupported architecture: $(uname -m)"
            err "Only x86_64 and arm64 are supported."
            exit 1
            ;;
    esac
}

detect_version() {
    if [[ -n "${VERSION:-}" ]]; then
        echo "$VERSION"
        return
    fi
    info "Fetching latest release version from GitHub..." >&2
    local tag
    tag=$(curl -sf "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    if [[ -z "$tag" ]]; then
        err "Could not determine latest release. Check your internet connection."
        err "Set VERSION=vX.Y.Z to install a specific version."
        exit 1
    fi
    echo "$tag"
}

detect_agent_service() {
    # Return the systemd unit name for zabbix-agent2
    for name in zabbix-agent2 zabbix_agent2; do
        if systemctl list-units --full --all 2>/dev/null | grep -q "${name}.service"; then
            echo "$name"
            return
        fi
    done
    echo ""
}

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------
do_uninstall() {
    step "Uninstalling Docker Swarm plugin"

    local svc
    svc=$(detect_agent_service)

    if [[ -f "$PLUGIN_BIN" ]]; then
        run rm -f "$PLUGIN_BIN"
        ok "Removed $PLUGIN_BIN"
    else
        warn "Binary not found at $PLUGIN_BIN, nothing to remove."
    fi

    if [[ -f "$PLUGIN_CONF" ]]; then
        run rm -f "$PLUGIN_CONF"
        ok "Removed $PLUGIN_CONF"
    else
        warn "Config not found at $PLUGIN_CONF, nothing to remove."
    fi

    if [[ -n "$svc" && "$NO_RESTART" != "1" ]]; then
        info "Restarting $svc..."
        run systemctl restart "$svc"
        ok "$svc restarted."
    fi

    ok "Uninstall complete."
}

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
do_install() {
    local arch version binary_name binary_url checksum_url tmp_dir

    arch=$(detect_arch)
    version=$(detect_version)
    binary_name="${PLUGIN_NAME}-linux-${arch}"
    binary_url="https://github.com/${REPO}/releases/download/${version}/${binary_name}"
    checksum_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

    step "Installing Docker Swarm plugin ${version} (${arch})"
    info "Binary URL: ${binary_url}"

    # ---- Create install directory ------------------------------------------
    step "Preparing directories"
    run mkdir -p "$INSTALL_DIR"
    run mkdir -p "$CONF_DIR"
    ok "Directories ready."

    # ---- Download binary ---------------------------------------------------
    step "Downloading binary"
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    info "Downloading ${binary_name}..."
    if ! run curl -fsSL -o "${tmp_dir}/${PLUGIN_NAME}" "$binary_url"; then
        err "Download failed: ${binary_url}"
        err "Check that version ${version} exists and has a ${arch} binary."
        exit 1
    fi

    # ---- Verify checksum ---------------------------------------------------
    step "Verifying checksum"
    if run curl -fsSL -o "${tmp_dir}/checksums.txt" "$checksum_url" 2>/dev/null; then
        local expected actual
        expected=$(grep "${binary_name}" "${tmp_dir}/checksums.txt" | awk '{print $1}')
        if [[ -z "$expected" ]]; then
            warn "No checksum entry for ${binary_name} — skipping verification."
        else
            if [[ "$DRY_RUN" != "1" ]]; then
                actual=$(sha256sum "${tmp_dir}/${PLUGIN_NAME}" | awk '{print $1}')
                if [[ "$actual" != "$expected" ]]; then
                    err "Checksum mismatch!"
                    err "  Expected: $expected"
                    err "  Got:      $actual"
                    exit 1
                fi
            fi
            ok "Checksum verified."
        fi
    else
        warn "Could not download checksums.txt — skipping verification."
    fi

    # ---- Install binary ----------------------------------------------------
    step "Installing binary"
    run install -m 755 -o root -g root "${tmp_dir}/${PLUGIN_NAME}" "$PLUGIN_BIN"
    ok "Installed to ${PLUGIN_BIN}."

    # ---- Write plugin config -----------------------------------------------
    step "Writing plugin configuration"
    if [[ -f "$PLUGIN_CONF" ]]; then
        warn "${PLUGIN_CONF} already exists — creating backup at ${PLUGIN_CONF}.bak"
        run cp "$PLUGIN_CONF" "${PLUGIN_CONF}.bak"
    fi

    if [[ "$DRY_RUN" != "1" ]]; then
        cat > "$PLUGIN_CONF" <<EOF
# Docker Swarm Plugin — generated by install.sh ${version}
Plugins.DockerSwarm.System.Path=${PLUGIN_BIN}
Plugins.DockerSwarm.System.Timeout=30
EOF
        if [[ "$SOCKET" != "/var/run/docker.sock" ]]; then
            echo "# Plugins.DockerSwarm.SocketPath=${SOCKET}" >> "$PLUGIN_CONF"
        fi
    else
        echo -e "${YELLOW}[DRY-RUN]${NC} Would write to ${PLUGIN_CONF}:"
        echo "  Plugins.DockerSwarm.System.Path=${PLUGIN_BIN}"
        echo "  Plugins.DockerSwarm.System.Timeout=30"
    fi
    ok "Config written to ${PLUGIN_CONF}."

    # ---- Docker socket access ----------------------------------------------
    step "Configuring Docker socket access"
    if ! groups zabbix 2>/dev/null | grep -qw docker; then
        if getent group docker &>/dev/null; then
            run usermod -aG docker zabbix
            ok "Added zabbix user to docker group."
            warn "Group membership takes effect after restarting zabbix-agent2."
        else
            warn "docker group not found. Ensure Docker is installed and the zabbix"
            warn "user can read ${SOCKET} (e.g. sudo chmod 660 ${SOCKET})."
        fi
    else
        ok "zabbix user is already in the docker group."
    fi

    # ---- Verify agent config includes conf.d -------------------------------
    step "Checking Zabbix Agent 2 configuration"
    local agent_conf
    for f in /etc/zabbix/zabbix_agent2.conf /etc/zabbix_agent2.conf; do
        [[ -f "$f" ]] && agent_conf="$f" && break
    done

    if [[ -z "${agent_conf:-}" ]]; then
        warn "zabbix_agent2.conf not found. Ensure it includes:"
        warn "  Include=${CONF_DIR}/*.conf"
    else
        if ! grep -q "Include.*${CONF_DIR}" "$agent_conf" 2>/dev/null; then
            warn "${agent_conf} does not include ${CONF_DIR}/*.conf"
            warn "Add this line to ${agent_conf}:"
            warn "  Include=${CONF_DIR}/*.conf"
        else
            ok "${agent_conf} already includes conf.d directory."
        fi
    fi

    # ---- Restart agent -----------------------------------------------------
    local svc
    svc=$(detect_agent_service)

    if [[ -n "$svc" && "$NO_RESTART" != "1" ]]; then
        step "Restarting $svc"
        run systemctl restart "$svc"
        sleep 1
        if [[ "$DRY_RUN" != "1" ]] && systemctl is-active --quiet "$svc"; then
            ok "$svc is running."
        elif [[ "$DRY_RUN" != "1" ]]; then
            err "$svc failed to start. Check logs: journalctl -u $svc -n 50"
            exit 1
        fi
    elif [[ -z "$svc" ]]; then
        warn "Could not detect zabbix-agent2 service. Restart it manually."
    else
        warn "Skipping restart (NO_RESTART=1). Remember to restart zabbix-agent2."
    fi

    # ---- Smoke test --------------------------------------------------------
    step "Testing plugin"
    if command -v zabbix_get &>/dev/null && [[ "$DRY_RUN" != "1" ]]; then
        sleep 1
        if zabbix_get -s 127.0.0.1 -k "swarm.services.discovery" &>/dev/null; then
            ok "swarm.services.discovery responded successfully."
        else
            warn "swarm.services.discovery did not respond — the agent may still be"
            warn "loading the plugin. Try manually:"
            warn "  zabbix_get -s 127.0.0.1 -k 'swarm.services.discovery'"
        fi
    else
        info "zabbix_get not found on this host — skipping smoke test."
        info "Test manually on your Zabbix server:"
        info "  zabbix_get -s <agent-host> -k 'swarm.services.discovery'"
    fi

    # ---- Done --------------------------------------------------------------
    echo ""
    echo -e "${GREEN}${BOLD}Installation complete!${NC}"
    echo ""
    echo -e "  Plugin binary : ${BOLD}${PLUGIN_BIN}${NC}"
    echo -e "  Plugin config : ${BOLD}${PLUGIN_CONF}${NC}"
    echo -e "  Version       : ${BOLD}${version}${NC}"
    echo ""
    echo -e "Next steps:"
    echo -e "  1. Import ${BOLD}zabbix_template_docker_swarm.yaml${NC} in Zabbix"
    echo -e "     Configuration → Templates → Import"
    echo -e "  2. Link the ${BOLD}Docker Swarm${NC} template to your swarm host(s)"
    echo -e "  3. For replica resource stats, run this installer on ${BOLD}every swarm node${NC}"
    echo ""
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    echo -e "${BOLD}Zabbix Agent 2 — Docker Swarm Plugin Installer${NC}"
    echo "----------------------------------------"

    check_root
    check_deps

    if [[ "$UNINSTALL" == "1" ]]; then
        do_uninstall
    else
        do_install
    fi
}

main "$@"
