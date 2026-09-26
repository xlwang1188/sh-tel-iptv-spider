package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
	"iptv-spider-sh/global"
)

var (
	CacheDir      = getCacheDir()
	MaxCacheFiles = 30 // ~300MB, supports multiple concurrent clients
	STBUserAgent  = "webkit;Resolution(PAL,720P,1080P,2106P,4K)"
)

func getCacheDir() string {
	if dir := os.Getenv("CACHE_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	if _, err := os.Stat("/home/xlwang/hls_cache"); err == nil {
		return "/home/xlwang/hls_cache"
	}
	dir := "./hls_cache"
	_ = os.MkdirAll(dir, 0755)
	return dir
}

var (
	nextSegMap sync.Map // url -> nextUrl
	inFlight   sync.Map // url -> chan struct{}
)

func registerNextSegment(currUrl, nextUrl string) {
	nextSegMap.Store(currUrl, nextUrl)
}

func getNextSegment(currUrl string) (string, bool) {
	val, ok := nextSegMap.Load(currUrl)
	if !ok {
		return "", false
	}
	return val.(string), true
}

func getCachePath(targetUrl string) string {
	h := sha256.Sum256([]byte(targetUrl))
	return filepath.Join(CacheDir, hex.EncodeToString(h[:16])+".ts")
}

func isCached(filePath string) bool {
	info, err := os.Stat(filePath)
	return err == nil && info.Size() > 100*1024
}

func cleanCacheIfNeeded() {
	entries, err := os.ReadDir(CacheDir)
	if err != nil {
		return
	}
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	var files []fileInfo
	now := time.Now()
	for _, e := range entries {
		name := e.Name()
		p := filepath.Join(CacheDir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Clean stale temp/preload files older than 2 minutes
		if strings.Contains(name, ".tmp.") || strings.Contains(name, ".preload.") {
			if now.Sub(info.ModTime()) > 2*time.Minute {
				_ = os.Remove(p)
			}
			continue
		}
		if !e.IsDir() && filepath.Ext(name) == ".ts" {
			files = append(files, fileInfo{path: p, modTime: info.ModTime()})
		}
	}
	if len(files) > MaxCacheFiles {
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.Before(files[j].modTime)
		})
		toDelete := len(files) - MaxCacheFiles
		for i := 0; i < toDelete; i++ {
			_ = os.Remove(files[i].path)
		}
	}
}

func serveFromCache(ctx iris.Context, cachePath string, targetUrl string) bool {
	f, err := os.Open(cachePath)
	if err != nil {
		return false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.Size() < 100*1024 {
		return false
	}
	// Update atime/mtime for True LRU eviction when multiple clients are playing
	_ = os.Chtimes(cachePath, time.Now(), time.Now())

	ctx.Header("Content-Type", "video/mp2t")
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.Header("Access-Control-Allow-Headers", "*")
	ctx.Header("Cache-Control", "public, max-age=86400")
	http.ServeContent(ctx.ResponseWriter(), ctx.Request(), "segment.ts", stat.ModTime(), f)

	global.LOG.Info("Served segment from cache", zap.String("path", cachePath))
	// Trigger preload of next 2 segments
	if nextUrl, ok := getNextSegment(targetUrl); ok {
		triggerPreload(nextUrl, 2)
	}
	return true
}

func ServeSegment(ctx iris.Context, targetUrl string) {
	cachePath := getCachePath(targetUrl)

	// 1. Check if already cached
	if isCached(cachePath) {
		if serveFromCache(ctx, cachePath, targetUrl) {
			return
		}
	}

	// 2. Check if currently in-flight preloading; wait briefly if so
	if val, ok := inFlight.Load(targetUrl); ok {
		ch := val.(chan struct{})
		select {
		case <-ch:
			if isCached(cachePath) {
				if serveFromCache(ctx, cachePath, targetUrl) {
					return
				}
			}
		case <-time.After(1500 * time.Millisecond):
		}
	}

	// 3. Not cached: fetch from CDN
	req, err := http.NewRequestWithContext(ctx.Request().Context(), http.MethodGet, targetUrl, nil)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.WriteString(err.Error())
		return
	}
	req.Close = true
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", STBUserAgent)

	// Ensure Range is always set (Telecom CDN performs best with Range: bytes=0-)
	rangeHdr := ctx.GetHeader("Range")
	if rangeHdr == "" {
		rangeHdr = "bytes=0-"
	}
	req.Header.Set("Range", rangeHdr)

	resp, err := hlsProxyClient.Do(req)
	if err != nil {
		global.LOG.Warn("Upstream segment fetch failed", zap.Error(err), zap.String("url", targetUrl))
		ctx.StatusCode(http.StatusBadGateway)
		ctx.WriteString(err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		ctx.StatusCode(resp.StatusCode)
		return
	}

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		ctx.Header("Content-Length", cl)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		ctx.Header("Content-Range", cr)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		ctx.Header("Accept-Ranges", ar)
	} else {
		ctx.Header("Accept-Ranges", "bytes")
	}
	ctx.Header("Content-Type", "video/mp2t")
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.Header("Access-Control-Allow-Headers", "*")
	ctx.Header("Cache-Control", "public, max-age=86400")
	ctx.StatusCode(resp.StatusCode)

	if ctx.Method() == http.MethodHead {
		return
	}

	// Cache if it is a full segment (200 OK or 206 starting at 0)
	canCache := resp.StatusCode == http.StatusOK ||
		(resp.StatusCode == http.StatusPartialContent && strings.HasPrefix(resp.Header.Get("Content-Range"), "bytes 0-"))

	if canCache {
		tmpPath := cachePath + fmt.Sprintf(".tmp.%d", time.Now().UnixNano())
		tmpFile, err := os.Create(tmpPath)
		if err == nil {
			mw := io.MultiWriter(ctx.ResponseWriter(), tmpFile)
			_, copyErr := io.Copy(mw, resp.Body)
			tmpFile.Close()
			if copyErr == nil {
				_ = os.Rename(tmpPath, cachePath)
				cleanCacheIfNeeded()
				global.LOG.Info("Cached segment via streaming", zap.String("path", cachePath))
			} else {
				_ = os.Remove(tmpPath)
			}
		} else {
			_, _ = io.Copy(ctx.ResponseWriter(), resp.Body)
		}
	} else {
		_, _ = io.Copy(ctx.ResponseWriter(), resp.Body)
	}

	if nextUrl, ok := getNextSegment(targetUrl); ok {
		triggerPreload(nextUrl, 2)
	}
}

func triggerPreload(nextUrl string, depth int) {
	if nextUrl == "" || depth <= 0 {
		return
	}
	cachePath := getCachePath(nextUrl)
	if isCached(cachePath) {
		if nextNextUrl, ok := getNextSegment(nextUrl); ok && depth > 1 {
			triggerPreload(nextNextUrl, depth-1)
		}
		return
	}

	ch := make(chan struct{})
	if _, loaded := inFlight.LoadOrStore(nextUrl, ch); loaded {
		return
	}

	go func() {
		defer func() {
			inFlight.Delete(nextUrl)
			close(ch)
		}()

		time.Sleep(200 * time.Millisecond)

		if isCached(cachePath) {
			if nextNextUrl, ok := getNextSegment(nextUrl); ok && depth > 1 {
				triggerPreload(nextNextUrl, depth-1)
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextUrl, nil)
		if err != nil {
			return
		}
		req.Close = true
		req.Header.Set("Connection", "close")
		req.Header.Set("User-Agent", STBUserAgent)
		req.Header.Set("Range", "bytes=0-")

		resp, err := hlsProxyClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return
		}

		tmpPath := cachePath + fmt.Sprintf(".preload.%d", time.Now().UnixNano())
		f, err := os.Create(tmpPath)
		if err != nil {
			return
		}
		_, err = io.Copy(f, resp.Body)
		f.Close()

		if err == nil {
			_ = os.Rename(tmpPath, cachePath)
			cleanCacheIfNeeded()
			global.LOG.Info("Preloaded segment successfully", zap.String("path", cachePath))

			// Chain preload the next segment
			if nextNextUrl, ok := getNextSegment(nextUrl); ok && depth > 1 {
				triggerPreload(nextNextUrl, depth-1)
			}
		} else {
			_ = os.Remove(tmpPath)
		}
	}()
}
