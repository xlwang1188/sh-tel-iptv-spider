# 播放器配置实战指南 (TiviMate / IPTVnator / Apple TV)

本项目生成的 M3U 播放列表与 XMLTV 节目单完全遵循国际标准，并完美兼容各大主流 IPTV 客户端的回看（时移）功能。

---

## 1. TiviMate（Android TV / 智能电视首选）

TiviMate 是目前电视端体验最好、回看支持最完善的专业 IPTV 播放器。

### 1.1 添加播放列表
1. 打开 TiviMate -> `设置` -> `播放列表` -> `添加播放列表`；
2. 选择 **M3U 播放列表**；
3. 输入 URL 地址：
   ```text
   http://172.16.0.5:8888/api/m3u8
   ```
   *(请将 `172.16.0.5` 替换为你实际部署 iptv-spider 的局域网 IP)*
4. 点击下一步完成。

### 1.2 配置 EPG 节目单
1. 进入 `设置` -> `EPG` -> `EPG 来源` -> `添加来源`；
2. 输入 XMLTV 地址：
   ```text
   http://172.16.0.5:8888/api/epg.xml
   ```
3. 更新频率可设为：**每天更新一次** 或 **启动时更新**。

### 1.3 配置回看（Catchup）
1. 进入 `设置` -> `播放列表` -> 点击刚才添加的播放列表；
2. 找到 **回看 (Catch-up)** 设置项：
   - **回看类型**: 选择 **附加 (Append)** 或 **Xtream Codes**；
   - **回看天数**: 填写 `7`（电信提供过去 7 天完整回看）；
   - **回看地址格式**（若需自定义填写）：
     ```text
     http://172.16.0.5:8888/api/catchup?id=${channel-id}&playseek=${(b)yyyyMMddHHmmss}-${(e)yyyyMMddHHmmss}
     ```
3. **缓冲区大小设置**:
   - `设置` -> `播放` -> `缓冲区大小`；
   - 建议设置为 **中等 (Medium)** 或 **无/小** 即可（因为服务端自带流水线预读引擎，本地响应仅需 20ms，无需播放器分配过大缓冲区）。

---

## 2. IPTVnator（Windows / macOS / Linux 桌面客户端）

IPTVnator 是一款开源跨平台的现代化 IPTV 客户端，原生支持回看。

1. **添加播放列表**:
   - 点击右上角齿轮 `Settings` -> `Add via URL`；
   - 填写 Playlist URL: `http://172.16.0.5:8888/api/m3u8`；
   - 填写 EPG URL: `http://172.16.0.5:8888/api/epg.xml`；
2. **使用回看**:
   - 在左侧频道列表中点击带有时钟图标的频道（如 CCTV-5 HD）；
   - 右侧将展开该频道的 EPG 节目单；
   - 挑选任意已经播出完毕的节目，点击右侧的 **播放按钮**，即刻秒开回看。

---

## 3. Apple TV / iOS 平台（APTV / VidHub / Senplayer）

### 3.1 APTV（推荐）
1. 打开 APTV -> 配置订阅 -> 添加订阅；
2. 填写 M3U 链接：`http://172.16.0.5:8888/api/m3u8`；
3. APTV 会自动识别 M3U 头部注入的 `catchup="append"` 标签，无需任何额外配置即可自动激活回看按钮。

### 3.2 VidHub
1. 添加多媒体库 -> IPTV；
2. 输入播放列表 URL 与 EPG URL；
3. 支持通过遥控器左右拖动进度条实现实时时移。
