# 轻量级 Python 抓取方案与 pcg-iptv 参考指南

在本项目研发过程中，除了基于 Go 语言的实时代理服务 `sh-tel-iptv-spider` 之外，开源社区的 **`pcg-iptv`** 项目（特别是其上海电信分支脚本 `shctiptv.py`）为我们逆向 CTC 鉴权协议与 `rtp2httpd` 结构映射提供了极其重要的参考与灵感。

为了方便用户在不同硬件条件（例如轻量级软路由 OpenWrt/ImmortalWrt、NAS 或嵌入式网关）下灵活选用，本项目将该轻量级单文件方案整理并收录于 `tools/pcg-iptv/` 目录中。

---

## 1. 方案对比：Go 实时代理 vs Python 轻量脚本

| 特性维度 | sh-tel-iptv-spider (Go 服务端) | pcg-iptv (Python 单文件脚本) |
| :--- | :--- | :--- |
| **运行形态** | 常驻后台 HTTP 微服务 (Web API) | 命令行 / Cron 定时离线运行脚本 |
| **运行依赖** | 编译为单个静态二进制，无任何外部运行时 | 需 Python 3 运行时（仅依赖标准库） |
| **资源开销** | 内存占用约 20MB ~ 30MB | 运行一次抓取耗时约 30 秒，运行完毕即退出 |
| **适用平台** | 虚拟机 (ESXi/PVE)、家庭 NAS、迷你主机 | 路由器 (OpenWrt/ImmortalWrt)、低配树莓派 |
| **直播支持** | 生成标准 M3U 联动 `udpxy` / `rtp2httpd` | 生成带 `rtp2httpd` 前缀的静态 `shctiptv2.m3u` |
| **回看支持** | ⭐ **内置 HLS 流水线预读缓存引擎** (秒开/0卡顿) | 生成静态 RTSP / 时移参数链接 |
| **适用场景** | 追求全功能、极致平滑回看与全家多端并发 | 仅需生成静态 M3U/XML，挂载在路由器定时更新 |

---

## 2. 脚本位置与文件结构

位于本项目中的：
```text
tools/pcg-iptv/
└── shctiptv.py       # 纯 Python 3 实现的上海电信 EPG 与 M3U 抓取脚本
```

### 依赖说明
- 纯 Python 3 标准库（`urllib`、`hashlib`、`xml` 等），**无需执行 `pip install` 安装任何第三方库**；
- 能够在几乎所有刷入 OpenWrt / ImmortalWrt 的路由器上开箱即用。

---

## 3. 使用方法

### 3.1 命令行直接运行
通过命令行参数直接传入机顶盒信息：

```bash
python3 tools/pcg-iptv/shctiptv.py \
  --user-id '4833xxxx@etv4' \
  --sn '00030032540702305260xxxx' \
  --mac '18:5e:0b:xx:xx:xx' \
  --rtp2httpd-url 'http://172.16.0.5:5140' \
  --epg-url 'http://172.16.0.5:3333/shctepg.xml'
```

### 3.2 脚本输出文件说明
运行成功后，会在输出目录生成以下文件：
1. **`shctiptv.m3u`**: 适用于标准 `udpxy` 的播放列表；
2. **`shctiptv2.m3u`**: 适用于 `rtp2httpd` 的播放列表（自动将 RTP 组播与回看路由封装为 `http://172.16.0.5:5140/rtp/...`）；
3. **`shctepg.xml`**: 标准 XMLTV 节目单文件。

---

## 4. OpenWrt 路由器定时任务配置 (Crontab)

如果希望将生成脚本直接部署在 OpenWrt 路由器上，由路由器自带的 uhttpd / Nginx 对外提供静态文件服务：

```bash
# 每天凌晨 4:30 自动抓取并更新节目单与播放列表
30 4 * * * /usr/bin/python3 /etc/iptv/shctiptv.py --user-id '你的账号' --sn '你的SN' --mac '你的MAC' >/tmp/shctiptv.log 2>&1
```

---

## 5. 致谢与溯源 (Acknowledgments)

特此向开源项目 **`pcg-iptv`** 及其作者致以衷心感谢！
- 项目仓库参考: [pcg-iptv](https://github.com/melody0709/cmcc_iptv_auto_py) / 上海电信抓取实现
- 该项目为上海电信 IPTV 的逆向流程提供了极具价值的代码范例与思路启发。
