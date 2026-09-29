package api

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"iptv-spider-sh/global"
	"iptv-spider-sh/model"
	"iptv-spider-sh/modules/auth"
	"iptv-spider-sh/utils"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
)

var formattedSeekRegex = regexp.MustCompile(`^\d{14}-\d{14}$`)

var (
	hlsTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		DisableKeepAlives:     true, // Telecom CDN returns Connection: close. Never reuse connections!
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}

	hlsProxyClient = &http.Client{
		Transport: hlsTransport,
	}
)

func InitApiRouters(rg iris.Party) {
	rg.Get("/schedule", schedule)
	rg.Get("/run", func(ctx iris.Context) {
		taskName := ctx.FormValue("task")

		go func() {
			global.ConcurrencyControl.Do("", func() (interface{}, error) {
				switch taskName {
				case "clean-ch":
					auth.CleanChannelData()
				case "clean-chi":
					auth.CleanChannelInfoData()
				case "clean-epg":
					auth.CleanEPGDetailsData()
				case "clean":
					auth.CleanChannelData()
					auth.CleanChannelInfoData()
					auth.CleanEPGDetailsData()
				case "update-chi":
					auth.GetGlobalClient().FetchChannelList()
				case "update-epg":
					auth.GetGlobalClient().FetchChannelProg()
				case "upload-m3u":
					auth.GenerateAndUploadM3u()
				case "upload-xmltv":
					auth.GenerateAndUploadXmlTv()
				case "upload-xmltv7":
					auth.GenerateAndUploadXmlTvDays7()
				}
				return nil, nil
			})
		}()
		ctx.WriteString("OK")
	})

	rg.Get("/m3u8", generateM3u8)
	rg.Get("/tsM3u8", generateTsM3u8)
	rg.Get("/epg", generateXmlTv)
	rg.Get("/epg.xml", generateXmlTv)

	rg.Any("/catchup", catchup)
	rg.Any("/proxy/segment.ts", proxySegment)
	rg.Any("/proxy/ts", proxySegment)
}

func parseTimeValue(val string, cst *time.Location) int64 {
	val = strings.TrimSpace(val)
	if val == "" || strings.HasPrefix(val, "{") || strings.HasPrefix(val, "%7B") || strings.HasPrefix(val, "${") {
		return 0
	}
	// 14位格式：YYYYMMDDHHmmss
	if len(val) == 14 {
		if t, err := time.ParseInLocation("20060102150405", val, cst); err == nil {
			return t.Unix()
		}
	}
	// ISO 8601
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t.Unix()
	}
	// 数字时间戳（秒或毫秒）
	if s, err := strconv.ParseInt(val, 10, 64); err == nil && s > 0 {
		if s > 1000000000000 { // 13位毫秒时间戳
			s = s / 1000
		}
		if s >= 1000000000 && s < 3000000000 {
			return s
		}
	}
	return 0
}

func catchup(ctx iris.Context) {
	if ctx.Method() == http.MethodOptions {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST")
		ctx.Header("Access-Control-Allow-Headers", "*")
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	fmt.Printf("[CATCHUP_REQ] URL=%s, UA=%s\n", ctx.Request().URL.String(), ctx.Request().UserAgent())

	id := ctx.FormValue("id")
	if id == "" {
		id = ctx.FormValue("channel_id")
	}
	if id == "" {
		id = ctx.FormValue("channel")
	}
	if id == "" {
		id = ctx.FormValue("ch")
	}

	stream := ctx.FormValue("stream")
	playseek := ctx.FormValue("playseek")
	playseek2 := ctx.FormValue("playseek2")
	playseek3 := ctx.FormValue("playseek3")
	utc := ctx.FormValue("utc")
	utcend := ctx.FormValue("utcend")
	lutc := ctx.FormValue("lutc")
	start := ctx.FormValue("start")
	end := ctx.FormValue("end")
	s := ctx.FormValue("s")
	e := ctx.FormValue("e")
	timestamp := ctx.FormValue("timestamp")
	durationStr := ctx.FormValue("duration")

	cst := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cst)
	safeNow := now.Add(-2 * time.Minute)

	var startSec int64 = 0
	var endSec int64 = 0

	// 1. 尝试从 start / s / utc / timestamp 提取开始时间戳
	for _, sVal := range []string{s, utc, start, timestamp} {
		if t := parseTimeValue(sVal, cst); t > 0 {
			startSec = t
			break
		}
	}

	// 尝试从 e / utcend / end / lutc 提取结束时间戳
	for _, eVal := range []string{e, utcend, end, lutc} {
		if t := parseTimeValue(eVal, cst); t > 0 {
			endSec = t
			break
		} else if dur, err := strconv.ParseInt(eVal, 10, 64); err == nil && dur > 0 && dur < 86400*7 && startSec > 0 {
			endSec = startSec + dur
			break
		}
	}

	// 2. 尝试从 playseek 中匹配 YYYYMMDDHHmmss-YYYYMMDDHHmmss
	if startSec == 0 {
		for _, ps := range []string{playseek, playseek2, playseek3} {
			ps = strings.TrimSpace(ps)
			if ps == "" || strings.HasPrefix(ps, "{") || strings.HasPrefix(ps, "%7B") || strings.HasPrefix(ps, "${") {
				continue
			}
			if formattedSeekRegex.MatchString(ps) {
				parts := strings.Split(ps, "-")
				if len(parts) == 2 {
					if t1, err := time.ParseInLocation("20060102150405", parts[0], cst); err == nil {
						startSec = t1.Unix()
					}
					if t2, err := time.ParseInLocation("20060102150405", parts[1], cst); err == nil {
						endSec = t2.Unix()
					}
				}
				break
			}
		}
	}

	if dur, err := strconv.ParseInt(durationStr, 10, 64); err == nil && dur > 0 && startSec > 0 && endSec <= startSec {
		endSec = startSec + dur
	}
	if startSec > 0 && endSec <= startSec {
		endSec = startSec + 3600
	}

	// 3. 兜底时间（如果完全未指定时间）
	if startSec == 0 {
		startSec = now.Add(-65 * time.Minute).Unix()
		endSec = now.Add(-5 * time.Minute).Unix()
	}

	global.LOG.Info("Catchup request parsed",
		zap.String("id", id),
		zap.Int64("startSec", startSec),
		zap.Int64("endSec", endSec),
	)

	// 4. 首选模式：通过 EPG 和 getTvodPlayUrl 获取原生高码率 HLS 流 (HTTP m3u8)
	if id != "" {
		var chInfo model.ChannelInfo
		if err := global.DB.Where("mix_no = ? OR ch_id = ? OR comm_name = ?", id, id, id).Order("is4_k desc, is_hd desc").First(&chInfo).Error; err == nil && chInfo.ChID != "" {
			startMs := startSec * 1000
			var prog model.EPGDetails
			// 精确匹配覆盖请求时间的节目
			err := global.DB.Where("comm_name = ? AND start_time <= ? AND end_time > ?", chInfo.CommName, startMs, startMs).
				Order("start_time desc").
				First(&prog).Error
			// 若没匹配到，在前后30分钟窗口内匹配最邻近节目
			if err != nil {
				err = global.DB.Where("comm_name = ? AND start_time >= ? AND start_time <= ?", chInfo.CommName, startMs-30*60*1000, startMs+30*60*1000).
					Order("start_time asc").
					First(&prog).Error
			}

			if err == nil && prog.ID != "" {
				client := auth.GetGlobalClient()
				if client != nil {
					pStartSec := prog.StartTime / 1000
					pEndSec := prog.EndTime / 1000
					playUrl, err := client.FetchTvodPlayUrl(chInfo.ChID, prog.ID, pStartSec, pEndSec)
					if err == nil && playUrl != "" {
						global.LOG.Info("Catchup HLS found", zap.String("id", id), zap.String("prog", prog.Name), zap.String("url", playUrl))
						if err := serveRewrittenM3u8(ctx, playUrl); err == nil {
							return
						} else {
							global.LOG.Warn("serveRewrittenM3u8 failed", zap.Error(err))
							return
						}
					} else {
						global.LOG.Warn("FetchTvodPlayUrl failed", zap.Error(err), zap.String("chID", chInfo.ChID), zap.String("playbillID", prog.ID))
					}
				}
			}
		}
	}

	// 5. 兜底回退：如果 EPG 未匹配到或非 EPG 回看，走 RTSP 时移
	var targetBaseUrl string
	if stream != "" {
		targetBaseUrl = stream
	} else if id != "" {
		var channel model.Channel
		if err := global.DB.Where("user_channel_id = ? OR channel_name = ?", id, id).First(&channel).Error; err == nil && channel.TimeShiftURL != "" {
			trimmed := strings.TrimPrefix(channel.TimeShiftURL, "rtsp://")
			targetBaseUrl = fmt.Sprintf("%s%s", global.CONFIG.Epg.RtspUrl, trimmed)
		}
	}

	if targetBaseUrl == "" {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.WriteString("Channel or TimeShiftURL not found")
		return
	}

	for _, param := range []string{"&playseek=", "?playseek=", "&TRANSPORT=", "?TRANSPORT="} {
		if idx := strings.Index(targetBaseUrl, param); idx != -1 {
			targetBaseUrl = targetBaseUrl[:idx]
		}
	}

	startTime := time.Unix(startSec, 0).In(cst)
	endTime := time.Unix(endSec, 0).In(cst)
	if endTime.After(safeNow) {
		endTime = safeNow
	}
	if !endTime.After(startTime) {
		endTime = startTime.Add(30 * time.Minute)
	}
	targetSeek := fmt.Sprintf("%s-%s", startTime.Format("20060102150405"), endTime.Format("20060102150405"))

	sep := "&"
	if !strings.Contains(targetBaseUrl, "?") {
		sep = "?"
	}
	finalUrl := fmt.Sprintf("%s%splayseek=%s", targetBaseUrl, sep, targetSeek)
	global.LOG.Info("Catchup RTSP fallback redirect", zap.String("id", id), zap.String("url", finalUrl))
	ctx.Redirect(finalUrl, iris.StatusFound)
}

func serveRewrittenM3u8(ctx iris.Context, playUrl string) error {
	if ctx.Method() == http.MethodOptions {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		ctx.Header("Access-Control-Allow-Headers", "*")
		ctx.StatusCode(http.StatusNoContent)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx.Request().Context(), http.MethodGet, playUrl, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	req.Close = true
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", STBUserAgent)

	fetchClient := &http.Client{
		Transport: hlsTransport,
		Timeout:   10 * time.Second,
	}
	resp, err := fetchClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch m3u8 failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream m3u8 returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read m3u8 body failed: %w", err)
	}

	finalURL := resp.Request.URL
	requestHost := ctx.Host()
	if requestHost == "" {
		requestHost = "172.16.0.5:8888"
	}

	var newLines []string
	var lastResolvedUrl string
	var firstSegmentUrl string
	scanner := bufio.NewScanner(bytes.NewReader(bodyBytes))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			newLines = append(newLines, line)
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}

		refURL, err := url.Parse(trimmed)
		if err != nil {
			newLines = append(newLines, line)
			continue
		}
		resolved := finalURL.ResolveReference(refURL).String()
		if firstSegmentUrl == "" {
			firstSegmentUrl = resolved
		}
		if lastResolvedUrl != "" {
			registerNextSegment(lastResolvedUrl, resolved)
		}
		lastResolvedUrl = resolved

		proxied := fmt.Sprintf("http://%s/api/proxy/segment.ts?url=%s", requestHost, url.QueryEscape(resolved))
		newLines = append(newLines, proxied)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan m3u8 lines failed: %w", err)
	}

	if firstSegmentUrl != "" {
		triggerPreload(firstSegmentUrl, 2)
	}

	output := strings.Join(newLines, "\n") + "\n"

	ctx.Header("Content-Type", "application/vnd.apple.mpegurl")
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.Header("Access-Control-Allow-Headers", "*")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Content-Length", strconv.Itoa(len(output)))

	if ctx.Method() == http.MethodHead {
		return nil
	}

	_, err = ctx.WriteString(output)
	return err
}

func proxySegment(ctx iris.Context) {
	if ctx.Method() == http.MethodOptions {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		ctx.Header("Access-Control-Allow-Headers", "*")
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	targetUrl := ctx.FormValue("url")
	if targetUrl == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.WriteString("Missing url parameter")
		return
	}

	ServeSegment(ctx, targetUrl)
}

func schedule(ctx iris.Context) {
	type s struct {
		ID       int
		PreTime  time.Time
		NextTime time.Time
	}
	var schedule []s
	for _, entry := range global.CRON.Entries() {
		schedule = append(schedule, s{
			ID:       int(entry.ID),
			PreTime:  entry.Prev,
			NextTime: entry.Next,
		})
	}
	ctx.JSON(schedule)
}

// 生成m3u8文件 节目去重
func generateM3u8(ctx iris.Context) {
	udpxy := ctx.FormValue("udpxy")
	scheme := ctx.FormValue("scheme")
	xteve := ctx.FormValue("xteve")
	all := ctx.FormValue("all")
	ref := ctx.FormValue("ref")

	var bufStr string
	if xteve == "true" {
		bufStr = "xteve"
	} else if udpxy != "" {
		bufStr = udpxy
	} else if scheme != "" {
		bufStr = scheme
	}
	if all == "true" {
		bufStr += all
	}
	reqMD5Key := utils.CalcMD5KeyForRequest("generateM3u8", bufStr)
	if ref != "true" && global.CACHE.IsExist(reqMD5Key) {
		ctx.Header("Content-Disposition", "attachment; filename=iptv.m3u")
		ctx.Binary(global.CACHE.Get(reqMD5Key).([]byte))
		return
	}
	resp, _, _ := global.ConcurrencyControl.Do(reqMD5Key, func() (interface{}, error) {
		respBytes := auth.GenerateM3u8(udpxy, scheme, xteve, all)
		timeOut := time.Duration(global.CONFIG.Cache.DefTimeOut)
		global.CACHE.Put(reqMD5Key, respBytes, time.Minute*timeOut)
		return respBytes, nil
	})
	ctx.Header("Content-Disposition", "attachment; filename=iptv.m3u")
	ctx.Binary(resp.([]byte))
}

func generateTsM3u8(ctx iris.Context) {
	ref := ctx.FormValue("ref")
	reqMD5Key := utils.CalcMD5KeyForRequest("generateTsM3u8")
	if ref != "true" && global.CACHE.IsExist(reqMD5Key) {
		ctx.Header("Content-Disposition", "attachment; filename=iptv-ts.m3u")
		ctx.Binary(global.CACHE.Get(reqMD5Key).([]byte))
		return
	}
	resp, _, _ := global.ConcurrencyControl.Do(reqMD5Key, func() (interface{}, error) {
		respBytes := auth.GenerateTimeShiftM3u8()
		timeOut := time.Duration(global.CONFIG.Cache.DefTimeOut)
		global.CACHE.Put(reqMD5Key, respBytes, time.Minute*timeOut)
		return respBytes, nil
	})
	ctx.Header("Content-Disposition", "attachment; filename=iptv-ts.m3u")
	ctx.Binary(resp.([]byte))
}
