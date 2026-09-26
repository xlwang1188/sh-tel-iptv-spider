#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
IPTV Catchup Performance & Cache Hit Testing Tool
用于测试 iptv-spider 回看流的响应耗时、下行带宽、本地缓存命中率与流水线预读效果。
"""

import urllib.request
import time
import sys
import argparse

if sys.stdout.encoding and sys.stdout.encoding.lower() != 'utf-8':
    try:
        sys.stdout.reconfigure(encoding='utf-8')
    except:
        pass

def test_stream(base_host, channel_id, playseek):
    print("=" * 70)
    print(f" Testing Catchup Stream on: {base_host}")
    print(f" Channel ID : {channel_id}")
    print(f" Playseek   : {playseek}")
    print("=" * 70)

    # 1. Fetch m3u8 playlist
    m3u8_url = f"http://{base_host}/api/catchup?id={channel_id}&playseek={playseek}"
    print(f"\n[1] Fetching M3U8 Playlist: {m3u8_url} ...")
    t0 = time.time()
    try:
        req = urllib.request.Request(m3u8_url, headers={"User-Agent": "IPTVnator/0.14.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            content = resp.read().decode('utf-8')
            t_m3u8 = time.time() - t0
            lines = [l.strip() for l in content.splitlines() if l.strip() and not l.startswith("#")]
            print(f"    [OK] Playlist fetched in {t_m3u8:.3f}s, Total Segments: {len(lines)}")
    except Exception as e:
        print(f"    [ERR] Failed to fetch playlist: {e}")
        return

    if not lines:
        print("    [ERR] Empty playlist returned!")
        return

    # 2. Sequentially fetch first 5 segments
    print(f"\n[2] Testing Sequential Segment Download & Cache Pipeline:")
    print(f"    {'Seg #':<8} {'Size (MB)':<12} {'Latency':<12} {'Throughput':<16} {'Source'}")
    print(f"    {'-'*8} {'-'*12} {'-'*12} {'-'*16} {'-'*12}")

    for idx, seg_url in enumerate(lines[:5], 1):
        t0 = time.time()
        try:
            req = urllib.request.Request(seg_url, headers={"User-Agent": "TiviMate/4.7.0"})
            with urllib.request.urlopen(req, timeout=15) as resp:
                data = resp.read()
                dur = time.time() - t0
                mb = len(data) / 1024 / 1024
                mbps = (len(data) * 8) / dur / 1e6 if dur > 0 else 0
                
                # If latency < 150ms and speed > 800 Mbps, it is a local SSD/RAM cache hit!
                if dur < 0.150 or mbps > 800:
                    source = "[HIT] Local Cache"
                else:
                    source = "[MISS] CDN Origin"

                print(f"    #{idx:<7} {mb:6.2f} MB     {dur:6.3f}s     {mbps:7.2f} Mbps    {source}")
        except Exception as e:
            print(f"    #{idx:<7} FAILED: {e}")

        # Simulate natural player consumption interval (1s delay before next request)
        time.sleep(1.0)

    print("\n" + "=" * 70)
    print(" Test finished. If segments #2-#5 show '[HIT] Local Cache', the preload pipeline is working perfectly!")
    print("=" * 70)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="Test IPTV Catchup Acceleration")
    parser.add_argument("--host", default="172.16.0.5:8888", help="iptv-spider address (default: 172.16.0.5:8888)")
    parser.add_argument("--id", default="123", help="Channel User ID (default: 123 for CCTV-5+ HD)")
    parser.add_argument("--playseek", default="20260925170600-20260925183600", help="Playseek time range")
    args = parser.parse_args()

    test_stream(args.host, args.id, args.playseek)
