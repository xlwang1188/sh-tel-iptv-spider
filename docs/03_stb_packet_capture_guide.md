# 真实机顶盒抓包与参数逆向指南

为了让虚拟机能够伪装成合法的机顶盒通过电信平台的鉴权，必须从物理机顶盒中提取对应的硬件参数与业务参数。本文详细指导如何抓包、提取哪些关键参数，以及必须规避的“MAC 漂移”深坑。

---

## 1. 抓包方法

抓取机顶盒与电信服务器之间的通讯报文通常有三种方式：

### 方案 A：镜像交换机端口镜像（SPAN / Port Mirroring，推荐）
1. 将物理机顶盒接入网管型交换机 Port 1；
2. 将光猫 IPTV 口接入 Port 2；
3. 将运行 Wireshark 的电脑接入 Port 3，配置 Port 3 镜像监听 Port 1 的所有进出双向流量。

### 方案 B：软路由 / 双网卡电脑透明桥接（Bridge）
在软路由或 Linux 电脑上将两个网卡（如 eth1 和 eth2）加入同一个网桥 `br0`，两端分别接光猫和机顶盒，并在网桥上直接运行 tcpdump：
```bash
tcpdump -i br0 -w stb_traffic.pcap
```

---

## 2. 抓包流程与交互触发
启动抓包后，手持遥控器进行以下标准操作：
1. **给机顶盒通电开机**（抓取开机 DHCP/IPoE 协商及 EPG 认证登录握手）；
2. **进入首页，换台观看直播**（抓取组播加入与 EPG 列表）；
3. **打开回看菜单，挑选任意频道回看一段过去的节目**（抓取 `action=getTvodPlayUrl` 及 CDN 切片下载过程）；
4. 停止抓包并保存为 `.pcap` 文件。

---

## 3. 核心参数提取清单

使用本项目自带的 `tools/pcap_analyzer.py` 或 Wireshark 打开抓包文件，过滤 HTTP/TCP 协议：

```bash
python tools/pcap_analyzer.py stb_traffic.pcap
```

### 3.1 基础硬件与账号参数（填入 `.env` 或 `config.yaml`）

| 参数名称 | 示例值 | 提取位置 / 过滤表达式 | 说明 |
| :--- | :--- | :--- | :--- |
| `stb.mac` | `18:5e:0b:xx:xx:xx` | Ethernet 帧源 MAC 地址 | 机顶盒物理网卡 MAC（小写） |
| `stb.ip` | `30.120.xx.xx` | IP 报文源 IP 地址 | 电信 IPoE 内网分配的 IP |
| `stb.uid` | `4833xxxx@etv4` | 抓包中搜索 `u=` 或 `uid=` | 电信 IPTV 业务账号 |
| `stb.sn` | `00030032540702305260xxxx` | 搜索 `stbid=` 或机顶盒背面条形码 | 机器唯一序列号 SN |
| `stb.type`| `B860AV2.1-T` | 搜索 `stbtype=` 或机顶盒铭牌 | 机顶盒型号（中兴/华为等） |
| `stb.auth_host` | `222.68.208.73:7001` | 开机第一个 HTTP POST 请求的 Host | EPG 认证网关地址 |

### 3.2 协议与 HTTP 头细节

- **User-Agent**:
  抓包中机顶盒发出的 HTTP 头：
  ```http
  User-Agent: webkit;Resolution(PAL,720P,1080P)
  ```
- **TVOD 回看接口特征**:
  机顶盒向 `218.83.165.34:8084` 发起的 POST 请求：
  ```http
  POST /iptvepg/frame1413/function/ajax/epg7getChannelByAjax.jsp HTTP/1.1
  Content-Type: application/x-www-form-urlencoded
  
  action=getTvodPlayUrl&channelID=ch00000000000000001783&playbillID=00000000070043422421&startTime=1790355960&endTime=1790361360
  ```
- **切片下行特征**:
  机顶盒从 `124.75.28.x:8114` 下载 TS 分片时：
  ```http
  GET /SHDXFH_live/.../index.m3u8?... HTTP/1.1
  Range: bytes=0-
  Connection: keep-alive
  ```
  电信 CDN 响应：
  ```http
  HTTP/1.1 206 Partial Content
  Content-Range: bytes 0-11425699/11425700
  ```

---

## 4. ⚠️ 致命陷阱：物理机顶盒开机导致的“MAC 漂移”

在本项目实战排障中，发现了一个极其隐蔽的断流根源：

> [!CAUTION]
> **抓包完成后，请务必彻底关闭物理机顶盒电源或拔掉其网线！**

### 为什么会发生 MAC 漂移？
1. 虚拟机（ESXi/PVE）为了通过电信认证，网络接口被配置为克隆物理机顶盒的 MAC（如 `18:5e:0b:xx:xx:xx`）和 IP（如 `30.120.xx.xx`）。
2. 如果此时**物理机顶盒依然开机并插在网线上**，物理交换机 / 光猫将同时检测到两个端口接入了相同 MAC 地址的设备。
3. 物理机顶盒在后台会频繁（每 1~2 秒）发送 mDNS、SSDP 以及局域网心跳广播。
4. 一旦物理机顶盒发送广播包，光猫的 MAC 地址转发表会立刻将目标端口重定向到物理机顶盒。
5. **后果**: 当虚拟机正在从电信 CDN 以 100Mbps 下载回看切片时，下载仅 300KB 后，交换机转发表就被机顶盒“抢夺”过去，导致下行流全部发给了休眠中的机顶盒，虚拟机端瞬间收不到后续包并出现超时断流（播放器卡死在第 1 秒或 16 秒）。
