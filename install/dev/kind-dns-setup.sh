#!/bin/bash

# DNS Setup Script for Kind Nodes
# This script sets up dnsmasq to handle DNS properly in Kind clusters
# Especially important for Rosetta/Rancher compatibility
# This script runs before kubelet starts via systemd ExecStartPre

set -euo pipefail

external_dns="${1:-1.1.1.1}"

echo "Setting up DNS with external DNS: ${external_dns}"

# Check if dnsmasq is already installed and configured
if dnsmasq -v 2>/dev/null && systemctl is-active dnsmasq >/dev/null 2>&1; then
    echo "dnsmasq is already running"
    exit 0
fi

# Save the original Docker/internal DNS server
read -r _ internal_dns < <(grep nameserver /etc/resolv.conf | head -1)
echo "Internal DNS: ${internal_dns}"

# Temporarily switch to a public DNS for package installation
echo "nameserver 1.1.1.1" > /etc/resolv.conf

# Update package lists and install dnsmasq
echo "Installing dnsmasq..."
apt-get update
apt-get install -y dnsmasq

# Configure dnsmasq
cat > /etc/dnsmasq.conf <<EOF
no-poll
no-resolv
listen-address=127.0.0.1
server=//${internal_dns}
server=${external_dns}
log-queries
log-facility=/var/log/dnsmasq.log
EOF

# Point the system to our new local dnsmasq server
echo "nameserver 127.0.0.1" > /etc/resolv.conf

# Restart dnsmasq to apply configuration
echo "Starting dnsmasq..."
systemctl restart dnsmasq
systemctl enable dnsmasq

# Verify dnsmasq is working
if systemctl is-active dnsmasq >/dev/null 2>&1; then
    echo "dnsmasq is running successfully"
else
    echo "ERROR: dnsmasq failed to start"
    exit 1
fi

# Test DNS resolution
echo "Testing DNS resolution..."
if nslookup kubernetes.default.svc.cluster.local 127.0.0.1 >/dev/null 2>&1; then
    echo "DNS resolution test passed"
else
    echo "WARNING: DNS resolution test failed"
fi

echo "DNS setup completed"
