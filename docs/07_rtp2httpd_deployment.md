# rtp2httpd 组播/单播代理部署指南

在 IPTV 系统中，直播电视频道采用 UDP/RTP 组播方式下发（如 `rtp://233.18.204.188:5140`）。为了让局域网内的智能电视、电脑、手机通过标准 HTTP 协议流畅播放直播，本项目配套使用现代化高性能媒体代理服务 **`rtp2httpd`**。

---

## 1. 为什么选择 rtp2httpd 而不是传统 udpxy？

传统 `udpxy` 已经多年未维护，在现代 4K/8K 超高清 IPTV 和多设备并发场景下存在明显短板。**`rtp2httpd`** 相比 `udpxy` 具有巨大优势：

| 核心指标 / 功能 | 传统 udpxy | rtp2httpd |
| :--- | :--- | :--- |
| **开发架构** | 传统单线程 / 多进程 fork | 现代轻量级 C 架构，高性能事件循环 |
| **零拷贝支持** | 不支持 | 支持 Linux 内核 `MSG_ZEROCOPY` |
| **内存与丢包防护**| 环形缓冲较小，易产生 UDP 溢出 | 支持可调缓冲池与 4MB+ 超大 Socket 缓冲区 |
| **协议支持** | 仅支持 RTP/UDP 组播 | 支持 **RTP/UDP 组播、RTSP 单播、FCC 快速换台** |
| **URL 兼容性** | 仅原生格式 | 100% 兼容 udpxy 的 `/rtp/` 与 `/udp/` URL 语法 |
| **监控与管理** | 极简文本状态 | 自带现代可视化 `/status` 状态面板与 `/player` 网页播放器 |
| **网卡精准绑定** | 依赖系统路由表 | 支持针对组播、RTSP 独立强绑指定的 IPTV 网卡接口 |

---

## 2. 自动化一键安装部署

项目在 `scripts/rtp2httpd/` 目录下提供了全自动化安装脚本：

```bash
cd sh-tel-iptv-spider/scripts/rtp2httpd
chmod +x install_rtp2httpd.sh
sudo ./install_rtp2httpd.sh
```

脚本将自动完成：
1. 自动检测系统架构（x86_64 或 aarch64）；
2. 从官方发布源下载静态编译的可执行文件并放入 `/usr/local/bin/rtp2httpd`；
3. 将优化好的 `rtp2httpd.conf` 复制到 `/etc/rtp2httpd/`；
4. 调优内核网络参数（`net.core.rmem_max = 4194304`）；
5. 自动配置开机自启服务（Alpine Linux 的 OpenRC 或 Debian/Ubuntu 的 systemd）。

---

## 3. 手动部署与关键配置详解

配置文件位于 `/etc/rtp2httpd/rtp2httpd.conf`：

```ini
[global]
# 日志级别: 0-quiet 1-error 2-warn 3-info 4-debug
verbosity = 3

# 开启与 udpxy 的路径兼容 (允许客户端请求 /rtp/233.x.x.x:5140)
udpxy = yes

# 状态页面与网页播放器
status-page-path = /status
player-page-path = /player

# 核心重点：上游网卡绑定 (必须绑定电信 IPTV 网卡接口，如 eth0.85)
# 优先级高于系统全局路由表，保证组播 IGMP 加入与发流 100% 走电信专网
upstream-interface-multicast = eth0.85
upstream-interface-rtsp = eth0.85
upstream-interface-http = eth0.85

# 内存缓冲池大小 (32768 * 1536 字节 = 48MB 缓冲区，增强多客户端并发能力)
buffer-pool-max-size = 32768

# UDP 套接字接收缓冲区 (4MB，杜绝 4K 频道 30+ Mbps 瞬时突发丢包)
udp-rcvbuf-size = 4194304

# 自动关联本地 iptv-spider 生成的 M3U 播放列表
external-m3u = http://127.0.0.1:8888/api/m3u8
external-m3u-update-interval = 7200

# 模拟机顶盒 RTSP 客户端
rtsp-user-agent = IPTV_STB

[bind]
# 监听端口 (默认监听全网卡 5140 端口)
* 5140
```

---

## 4. 服务管理与开机自启

### Alpine Linux (OpenRC)
```bash
# 复制服务脚本
cp scripts/rtp2httpd/rtp2httpd.initd /etc/init.d/rtp2httpd
chmod +x /etc/init.d/rtp2httpd

# 添加开机自启并启动
rc-update add rtp2httpd default
rc-service rtp2httpd restart

# 查看运行状态
rc-service rtp2httpd status
```

### Ubuntu / Debian / CentOS (systemd)
```bash
# 复制 service 文件
sudo cp scripts/rtp2httpd/rtp2httpd.service /etc/systemd/system/rtp2httpd.service
sudo systemctl daemon-reload
sudo systemctl enable rtp2httpd
sudo systemctl restart rtp2httpd
```

---

## 5. 验证与测试

### 5.1 查看 Web 可视化仪表盘
打开浏览器访问：
- **运行状态监控**: `http://<服务器IP>:5140/status`  
  实时查看当前在线客户端数量、每个客户端连接的频道、实时传输速率及丢包统计。
- **内置网页播放器**: `http://<服务器IP>:5140/player`  
  支持直接在浏览器内测试观看组播频道。

### 5.2 命令行流畅度测试
在局域网内任意终端执行：
```bash
# 请求 CCTV-4K 组播频道
curl -i "http://172.16.0.5:5140/rtp/233.18.204.188:5140" | head -c 1000000 | wc -c
```
若能瞬间接收满 1,000,000 字节，说明 `rtp2httpd` 组播分发工作正常且带宽极度充裕！
