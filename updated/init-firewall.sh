#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

# Configure a strict egress firewall for the devcontainer while preserving
# DNS, localhost, host-network connectivity, and a curated allowlist.

# Preserve Docker's internal DNS NAT rules before flushing tables.
DOCKER_DNS_RULES=$(iptables-save -t nat | grep "127\.0\.0\.11" || true)

# Reset any existing rules/chains and previous allowlist set.
iptables -F
iptables -X
iptables -t nat -F
iptables -t nat -X
iptables -t mangle -F
iptables -t mangle -X
ipset destroy allowed-domains 2>/dev/null || true

# Restore Docker-managed DNS NAT rules so container name resolution still works.
if [ -n "$DOCKER_DNS_RULES" ]; then
    echo "Restoring Docker DNS rules..."
    iptables -t nat -N DOCKER_OUTPUT 2>/dev/null || true
    iptables -t nat -N DOCKER_POSTROUTING 2>/dev/null || true
    echo "$DOCKER_DNS_RULES" | xargs -L 1 iptables -t nat
else
    echo "No Docker DNS rules to restore"
fi

# Baseline allowances needed before default DROP policies are applied.
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A INPUT -p udp --sport 53 -j ACCEPT
iptables -A OUTPUT -p tcp --dport 22 -j ACCEPT
iptables -A INPUT -p tcp --sport 22 -m state --state ESTABLISHED -j ACCEPT
iptables -A INPUT -i lo -j ACCEPT
iptables -A OUTPUT -o lo -j ACCEPT

# All allowed remote destinations are tracked in this ipset.
ipset create allowed-domains hash:net

# Allow all published GitHub networks from GitHub metadata. This covers
# GitHub API/web/git/release assets used by gh and many tool installers.
echo "Fetching GitHub IP ranges..."
gh_ranges=$(curl -s https://api.github.com/meta)
if [ -z "$gh_ranges" ]; then
    echo "ERROR: Failed to fetch GitHub IP ranges"
    exit 1
fi

if ! echo "$gh_ranges" | jq -e '.web and .api and .git' >/dev/null; then
    echo "ERROR: GitHub API response missing required fields"
    exit 1
fi

echo "Processing GitHub IPs..."
gh_cidrs=$(echo "$gh_ranges" | jq -r '.. | strings | select(test("^[0-9]{1,3}(\\.[0-9]{1,3}){3}/[0-9]{1,2}$"))')
if [ -z "$gh_cidrs" ]; then
    echo "ERROR: No CIDR ranges found in GitHub meta response"
    exit 1
fi

while read -r cidr; do
    if [[ ! "$cidr" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}/[0-9]{1,2}$ ]]; then
        echo "ERROR: Invalid CIDR range from GitHub meta: $cidr"
        exit 1
    fi
    ipset add --exist allowed-domains "$cidr"
done < <(printf '%s\n' "$gh_cidrs" | sort -u | aggregate -q)

resolve_ipv4s_with_retry() {
    local domain="$1"
    local attempts="${2:-5}"
    local delay_seconds="${3:-1}"
    local dig_ips=""
    local getent_ips=""
    local ips=""

    for _ in $(seq 1 "$attempts"); do
        dig_ips="$(dig +short A "$domain" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' || true)"
        getent_ips="$(getent ahostsv4 "$domain" 2>/dev/null | awk '{print $1}' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' || true)"
        ips="$(printf '%s\n%s\n' "$dig_ips" "$getent_ips" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -u || true)"
        if [ -n "$ips" ]; then
            printf '%s\n' "$ips"
            return 0
        fi
        sleep "$delay_seconds"
    done

    return 1
}

allow_domain() {
    local domain="$1"
    local requirement="$2"
    local reason="$3"
    local cidr_bits="${4:-32}"
    local ips=""

    echo "Resolving $domain (${reason})..."
    ips="$(resolve_ipv4s_with_retry "$domain" 5 1 || true)"

    if [ -z "$ips" ]; then
        if [ "$requirement" = "required" ]; then
            echo "ERROR: Failed to resolve required domain $domain ($reason)"
            exit 1
        else
            echo "WARNING: Failed to resolve optional domain $domain ($reason), skipping..."
            return 0
        fi
    fi

    while read -r ip; do
        if [[ ! "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
            echo "WARNING: Invalid IP from DNS for $domain: $ip, skipping..."
            continue
        fi
        if [ "$cidr_bits" = "32" ]; then
            ipset add --exist allowed-domains "$ip"
        else
            local cidr
            cidr="$(echo "$ip" | awk -F. -v bits="$cidr_bits" '{print $1 "." $2 "." $3 ".0/" bits}')"
            ipset add --exist allowed-domains "$cidr"
        fi
    done < <(echo "$ips")
}

DOMAIN_SPECS=()
add_required_domain() {
    DOMAIN_SPECS+=("$1|required|$2")
}
add_optional_domain() {
    DOMAIN_SPECS+=("$1|optional|$2")
}
add_required_domain_with_cidr() {
    DOMAIN_SPECS+=("$1|required|$2|$3")
}
add_optional_domain_with_cidr() {
    DOMAIN_SPECS+=("$1|optional|$2|$3")
}

# GitHub CLI tooling.
add_optional_domain "cli.github.com" "gh CLI package metadata/download host"

# Core package registries used by npm/pnpm/yarn and JS tooling.
add_required_domain "registry.npmjs.org" "npm registry for npm/pnpm/corepack package manager assets"
add_optional_domain "npm.jsr.io" "JSR registry support used by modern pnpm/npm workflows"
add_optional_domain "repo.yarnpkg.com" "Yarn modern releases (yarn set version / corepack flows)"

# Claude Code + telemetry endpoints.
add_required_domain "api.anthropic.com" "Claude Code API"
add_optional_domain "sentry.io" "error reporting used by some CLIs"
add_optional_domain "statsig.anthropic.com" "Claude feature flag/telemetry endpoint"
add_optional_domain "statsig.com" "Claude feature flag/telemetry backend"

# VS Code/devcontainer extension update endpoints.
add_optional_domain "marketplace.visualstudio.com" "VS Code extension marketplace"
add_optional_domain "vscode.blob.core.windows.net" "VS Code extension asset blobs"
add_optional_domain "update.code.visualstudio.com" "VS Code update checks"

# OpenAI/Codex endpoints.
add_required_domain "api.openai.com" "OpenAI API"
add_required_domain "auth.openai.com" "OpenAI auth handoffs"
add_required_domain "chatgpt.com" "Codex responses endpoint under chatgpt.com/backend-api/..."
add_optional_domain "chat.openai.com" "legacy ChatGPT host used by some auth/browser flows"
add_optional_domain "openai.com" "OpenAI web redirects and auth support"

# OpenCode provider endpoints.
add_required_domain_with_cidr "opencode.ai" "OpenCode Zen API/auth endpoint (allow /24 due edge IP churn)" "24"
add_required_domain_with_cidr "api.z.ai" "Z.AI Coding Plan provider API endpoint (allow /24 due edge IP churn)" "24"
add_optional_domain "z.ai" "Z.AI API console and account management"

# Gemini and Google auth endpoints.
add_required_domain "generativelanguage.googleapis.com" "Gemini API"
add_required_domain "accounts.google.com" "Google account auth"
add_required_domain "oauth2.googleapis.com" "OAuth token exchange for Gemini CLI"
add_optional_domain "play.googleapis.com" "Google API backend dependencies"
add_optional_domain "aiplatform.googleapis.com" "Vertex AI endpoint (if Gemini CLI uses Vertex mode)"
add_optional_domain "cloudcode-pa.googleapis.com" "Google developer tooling backend"
add_optional_domain "iamcredentials.googleapis.com" "GCP service-account token exchange"

# MCP servers configured in this image.
add_required_domain_with_cidr "mcp.grep.app" "grep MCP server (allow /24 due edge IP churn)" "24"
add_required_domain_with_cidr "mcp.context7.com" "Context7 MCP server (allow /24 due edge IP churn)" "24"
add_optional_domain "accounts.context7.com" "Context7 auth/session endpoints"
add_required_domain_with_cidr "mcp.exa.ai" "Exa MCP server (allow /24 due edge IP churn)" "24"
add_optional_domain "auth.exa.ai" "Exa auth endpoints"
add_optional_domain "accounts.exa.ai" "Exa account endpoints"

# Additional tool install scripts fetched by install scripts in updated/.
add_optional_domain "raw.githubusercontent.com" "beads/beads_viewer installer scripts"
add_optional_domain "mise.run" "mise bootstrap install script"
add_optional_domain "getmic.ro" "micro editor installer script"
add_optional_domain "deb.debian.org" "Debian apt repositories for optional addon installs"
add_optional_domain "security.debian.org" "Debian security apt repository for optional addon installs"
add_optional_domain_with_cidr "cdn.playwright.dev" "Playwright browser download CDN (allow /24 due CDN edge IP churn)" "24"
add_optional_domain_with_cidr "playwright.download.prss.microsoft.com" "Playwright browser download fallback CDN (allow /24 due CDN edge IP churn)" "24"
add_optional_domain "storage.googleapis.com" "Playwright Chromium CFT redirected download host"

# mise-managed runtime/tool domains.
add_optional_domain "nodejs.org" "mise node backend official Node.js binary downloads"
add_optional_domain "unofficial-builds.nodejs.org" "optional Node unofficial builds via mise node.mirror_url"
add_optional_domain "pypi.org" "uv/pip Python package index"
add_optional_domain "files.pythonhosted.org" "Python package file downloads"
add_optional_domain "astral.sh" "uv installer and release bootstrap endpoint"
add_optional_domain "go.dev" "Go version metadata and release index"
add_optional_domain "dl.google.com" "Go toolchain tarballs used by go/mise installs"
add_optional_domain "proxy.golang.org" "Go module proxy used by go command"
add_optional_domain "sum.golang.org" "Go checksum database used by go command"
add_optional_domain "rustup.rs" "rustup installer landing host"
add_optional_domain "sh.rustup.rs" "rustup shell installer script"
add_optional_domain "static.rust-lang.org" "Rust toolchain/dist downloads"
add_optional_domain "crates.io" "Rust crate registry API"
add_optional_domain "index.crates.io" "Rust sparse index"
add_optional_domain "static.crates.io" "Rust crate tarball downloads"

for spec in "${DOMAIN_SPECS[@]}"; do
    IFS='|' read -r domain requirement reason cidr_bits <<<"$spec"
    allow_domain "$domain" "$requirement" "$reason" "${cidr_bits:-32}"
done

# Allow host-network communication (docker bridge gateway).
HOST_IP=$(ip route | grep default | cut -d" " -f3)
if [ -z "$HOST_IP" ]; then
    echo "ERROR: Failed to detect host IP"
    exit 1
fi

HOST_NETWORK=$(echo "$HOST_IP" | sed "s/\.[0-9]*$/.0\/24/")
echo "Host network detected as: $HOST_NETWORK"
iptables -A INPUT -s "$HOST_NETWORK" -j ACCEPT
iptables -A OUTPUT -d "$HOST_NETWORK" -j ACCEPT

# Apply default deny and then permit established + allowlisted outbound.
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT DROP
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -m set --match-set allowed-domains dst -j ACCEPT
iptables -A OUTPUT -j REJECT --reject-with icmp-admin-prohibited

echo "Firewall configuration complete"

verify_url_reachable() {
    local url="$1"
    if ! curl --connect-timeout 5 "$url" >/dev/null 2>&1; then
        echo "ERROR: Firewall verification failed - unable to reach $url"
        exit 1
    fi
    echo "Firewall verification passed - able to reach $url as expected"
}

# Ensure deny-by-default still works.
if curl --connect-timeout 5 https://example.com >/dev/null 2>&1; then
    echo "ERROR: Firewall verification failed - was able to reach https://example.com"
    exit 1
else
    echo "Firewall verification passed - unable to reach https://example.com as expected"
fi

# Verify core endpoints that should always be reachable for this image.
verify_url_reachable "https://api.github.com/zen"
verify_url_reachable "https://chatgpt.com/"
verify_url_reachable "https://api.openai.com/"
verify_url_reachable "https://opencode.ai/zen"
verify_url_reachable "https://api.z.ai/"
verify_url_reachable "https://mcp.grep.app/"
verify_url_reachable "https://oauth2.googleapis.com/token"
