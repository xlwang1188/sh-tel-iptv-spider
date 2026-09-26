#!/bin/sh
# Auto-configure network routing for Shanghai Telecom IPTV on Linux / Alpine
# 上海电信 IPTV 专网路由自动配置脚本

GATEWAY=${1:-30.120.0.1}
DEV=${2:-eth0.85}

echo "Configuring IPTV static routes via $GATEWAY on device $DEV ..."

# Telecom FonsView CDN & Load Balancer
ip route replace 124.75.0.0/16 via "$GATEWAY" dev "$DEV"

# Telecom EPG & Channel Metadata Servers
ip route replace 218.83.0.0/16 via "$GATEWAY" dev "$DEV"

# Telecom Auth & Logging Portal
ip route replace 222.68.208.0/21 via "$GATEWAY" dev "$DEV"

# 4K & HD Multicast Stream Sources
ip route replace 233.18.204.0/24 via "$GATEWAY" dev "$DEV"
ip route replace 239.0.0.0/8 via "$GATEWAY" dev "$DEV"

echo "IPTV routes applied successfully:"
ip route show dev "$DEV"
