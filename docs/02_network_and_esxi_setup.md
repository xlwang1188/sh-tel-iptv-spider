# 网络拓扑与 ESXi / Alpine 虚拟机配置指南

在将电信机顶盒能力迁移到虚拟化平台（如 VMware ESXi、Proxmox VE、PVE、物理软路由）时，网络配置是最关键的一环。本文详细说明从光猫单线复用到虚拟机双网卡策略路由的完整配置过程。

---

## 1. 物理拓扑与光猫单线复用（VLAN 85）

上海电信光猫通常将上网（Internet）与 IPTV 绑定到不同端口，或通过 VLAN 划分：
- **Internet**: 通常使用上网 VLAN（如 VLAN 1 或未经标记）
- **IPTV**: 上海电信 IPTV 专网固定使用 **VLAN 85**

### 单线复用连接模式（推荐）
若从光猫到弱电箱/软路由只有一根网线：
1. **光猫端配置**: 将 IPTV（VLAN 85）与宽带上网（Internet）绑定到同一个 LAN 口（如 LAN1），开启 VLAN Tag 模式；
2. **交换机/网卡端**: 进入主交换机或 ESXi 物理网卡，接收带有 VLAN 85 标记的流量。

---

## 2. VMware ESXi 虚拟交换机配置

在 ESXi 管理后台创建专用于 IPTV 的虚拟端口组：

1. **进入网络设置**: `网络` -> `虚拟交换机` -> `添加标准虚拟交换机`（或复用现有 vSwitch）。
2. **添加端口组**:
   - 名称: `IPTV-VLAN85`
   - **VLAN ID**: 填写 `85`（如果光猫出来带有 Tag），或填 `0`（如果光猫 ITV 口直通未打 Tag 的独立物理网卡）；
   - **安全设置**:
     - 混杂模式: **接受**
     - MAC 地址更改: **接受**
     - 伪信号: **接受**
     *(由于虚拟机需要克隆物理机顶盒的 MAC 地址，必须在 ESXi 端口组安全中允许 MAC 变更和伪信号！)*

---

## 3. Alpine Linux 虚拟机网卡与路由配置

建议使用极简、低资源开销的 **Alpine Linux**（内存仅需 256MB~512MB 即可跑满多路流）。

虚拟机分配两张虚拟网卡：
- **网卡 1 (eth0)**: 接入 `IPTV-VLAN85` 端口组（绑定克隆的物理机顶盒 MAC，如 `18:5e:0b:xx:xx:xx`）
- **网卡 2 (eth1)**: 接入局域网 `VM Network`（如静态分配 `172.16.0.5`）

### 3.1 配置文件 `/etc/network/interfaces`

```text
auto lo
iface lo inet loopback

# 局域网管理网卡 (eth1)
auto eth1
iface eth1 inet static
    address 172.16.0.5
    netmask 255.255.254.0
    gateway 172.16.0.1
    # 默认网关走局域网路由器，保证可通过内网和外网访问 Alpine
    metric 10

# 电信 IPTV 网卡 (eth0 及 VLAN 85 子接口)
auto eth0
iface eth0 inet manual
    hwaddress ether 18:5e:0b:xx:xx:xx

auto eth0.85
iface eth0.85 inet static
    hwaddress ether 18:5e:0b:xx:xx:xx
    address 30.120.xx.xx
    netmask 255.255.0.0
    # 电信 IPTV 内网网关通常为 30.120.0.1
    # 注意：不要将默认路由全部指向它，而是通过专用路由表转发
    metric 210
    
    # 启动时自动写入电信内网专用策略路由
    up ip route add 124.75.0.0/16 via 30.120.0.1 dev eth0.85
    up ip route add 218.83.0.0/16 via 30.120.0.1 dev eth0.85
    up ip route add 222.68.208.0/21 via 30.120.0.1 dev eth0.85
    up ip route add 233.18.204.0/24 via 30.120.0.1 dev eth0.85
    up ip route add 239.0.0.0/8 via 30.120.0.1 dev eth0.85
    
    down ip route del 124.75.0.0/16 via 30.120.0.1 dev eth0.85
    down ip route del 218.83.0.0/16 via 30.120.0.1 dev eth0.85
    down ip route del 222.68.208.0/21 via 30.120.0.1 dev eth0.85
    down ip route del 233.18.204.0/24 via 30.120.0.1 dev eth0.85
    down ip route del 239.0.0.0/8 via 30.120.0.1 dev eth0.85
```

### 3.2 电信核心网段说明
- `30.120.0.0/16`: 机顶盒专网 IPoE 网段
- `124.75.0.0/16`: 电信 FonsView CDN 切片服务器及负载均衡集群
- `218.83.0.0/16`: 电信 EPG 业务及频道接口集群
- `222.68.208.0/21`: 电信认证服务器及日志上报集群
- `233.18.204.0/24` & `239.0.0.0/8`: 4K 及高清频道组播流源

---

## 4. 组播转单播服务 (udpxy) 安装与配置

在 Alpine 下安装 udpxy 以实现直播组播转单播：

```bash
apk update
apk add udpxy

# 启动 udpxy 监听在局域网 5140 端口，监听组播网卡 eth0.85
udpxy -a 172.16.0.5 -p 5140 -m eth0.85 -c 10
```

测试组播是否畅通：
```bash
curl -i "http://172.16.0.5:5140/rtp/233.18.204.188:5140" | head -c 1000
```
若能持续收到二进制数据（以 `0x47` 开头的 TS 包），则组播完全打通！

---

## 5. 服务常驻管理 (OpenRC Service)

创建 `/etc/init.d/iptv-spider` 服务脚本：

```bash
#!/sbin/openrc-run

name="iptv-spider"
description="Shanghai Telecom IPTV Spider Service"
command="/home/xlwang/sh-tel-iptv-spider/iptv-spider"
command_args=""
command_background="yes"
command_user="xlwang:xlwang"
directory="/home/xlwang/sh-tel-iptv-spider"
pidfile="/run/iptv-spider.pid"
output_log="/home/xlwang/sh-tel-iptv-spider/log/service.log"
error_log="/home/xlwang/sh-tel-iptv-spider/log/service.err"

depend() {
    need net
    after firewall
}

start_pre() {
    mkdir -p /home/xlwang/sh-tel-iptv-spider/log
    mkdir -p /home/xlwang/hls_cache
    chown -R xlwang:xlwang /home/xlwang/hls_cache
}
```

启用开机自启并启动：
```bash
chmod +x /etc/init.d/iptv-spider
rc-update add iptv-spider default
rc-service iptv-spider start
```
