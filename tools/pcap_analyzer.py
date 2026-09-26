#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
IPTV STB PCAP Packet Analyzer & Speed Measurement Tool
用于分析机顶盒抓包（PCAP）并提取 EPG、TVOD 回看调度地址、切片吞吐量与耗时统计。
"""

import struct
import socket
import sys
import os
import datetime
from collections import Counter

def analyze_pcap(filepath):
    if not os.path.exists(filepath):
        print(f"Error: File not found: {filepath}")
        return

    print("=" * 70)
    print(f" Analyzing PCAP: {os.path.basename(filepath)}")
    print(f" Full Path: {os.path.abspath(filepath)}")
    print(f" Size: {os.path.getsize(filepath) / 1024 / 1024:.2f} MB")
    print("=" * 70)

    with open(filepath, 'rb') as f:
        gh = f.read(24)
        if len(gh) < 24:
            print("Invalid PCAP: header too short")
            return
        endian = '<' if gh[:4] == b'\xd4\xc3\xb2\xa1' else '>'

        idx = 0
        t0 = None
        t_last = None
        streams = {} # dp -> [t_first, t_last, total_bytes, content_length, uri]
        http_requests = []
        http_responses = []

        while True:
            hdr = f.read(16)
            if len(hdr) < 16:
                break
            idx += 1
            s, u, incl, orig = struct.unpack(endian + 'IIII', hdr)
            ts = s + u / 1e6
            if t0 is None:
                t0 = ts
            t_last = ts

            data = f.read(incl)
            if len(data) < 14:
                continue

            # Ethernet header
            et = struct.unpack('!H', data[12:14])[0]
            ip_start = 14
            if et == 0x8100: # 802.1Q VLAN
                et = struct.unpack('!H', data[16:18])[0]
                ip_start = 18
            if et != 0x0800:
                continue # Skip non-IPv4

            if len(data) < ip_start + 20:
                continue
            ip_hdr = data[ip_start:ip_start+20]
            ihl = (ip_hdr[0] & 0x0f) * 4
            proto = ip_hdr[9]
            src_ip = socket.inet_ntoa(ip_hdr[12:16])
            dst_ip = socket.inet_ntoa(ip_hdr[16:20])

            l4 = ip_start + ihl
            if proto == 6: # TCP
                if len(data) < l4 + 14:
                    continue
                sp, dp, seq, ack, of = struct.unpack('!HHIIH', data[l4:l4+14])
                hlen = ((of >> 12) & 0xf) * 4
                payload = data[l4+hlen:]
                payload_len = len(payload)

                # Track segment download streams on port 8114 (FonsView CDN)
                if sp == 8114:
                    if dp not in streams:
                        streams[dp] = [ts, ts, 0, 0, ""]
                    streams[dp][1] = ts
                    streams[dp][2] += payload_len

                if payload_len > 0:
                    try:
                        text = payload.decode('utf-8', errors='ignore')
                    except Exception:
                        text = ""

                    if any(text.startswith(m) for m in ['GET ', 'POST ']):
                        first_line = text.split('\r\n')[0]
                        headers = text.split('\r\n\r\n')[0]
                        body = text[len(headers)+4:] if len(text) > len(headers)+4 else ""
                        http_requests.append((ts - t0, f"{src_ip}:{sp} -> {dst_ip}:{dp}", first_line, body[:150]))

                    elif text.startswith('HTTP/'):
                        first_line = text.split('\r\n')[0]
                        headers = text.split('\r\n\r\n')[0]
                        cl = 0
                        for hline in headers.split('\r\n'):
                            if hline.lower().startswith('content-length:'):
                                try:
                                    cl = int(hline.split(':')[1].strip())
                                except:
                                    pass
                        if sp == 8114 and dp in streams:
                            streams[dp][3] = cl
                        http_responses.append((ts - t0, f"{src_ip}:{sp} -> {dst_ip}:{dp}", first_line, cl))

        dur = (t_last - t0) if (t0 and t_last) else 0
        print(f"\n[1] Overview:")
        print(f"    - Total Packets : {idx}")
        print(f"    - Total Time    : {dur:.2f} seconds")
        if t0:
            print(f"    - Start Time    : {datetime.datetime.fromtimestamp(t0)}")
            print(f"    - End Time      : {datetime.datetime.fromtimestamp(t_last)}")

        if http_requests:
            print(f"\n[2] Key HTTP Requests ({len(http_requests)} total):")
            for t_rel, conn, req, body in http_requests[:15]:
                print(f"    +{t_rel:6.2f}s [{conn}] {req}")
                if body.strip():
                    print(f"            Body: {body.strip()}")

        if streams:
            print(f"\n[3] HLS Segment Transfer Speeds (Port 8114 CDN Stream):")
            print(f"    {'Port':<10} {'Downloaded':<14} {'Duration':<10} {'Speed (MB/s)':<14} {'Speed (Mbps)':<12}")
            print(f"    {'-'*10} {'-'*14} {'-'*10} {'-'*14} {'-'*12}")
            seg_idx = 1
            for dp, (t1, t2, b, cl, _) in streams.items():
                if b < 100 * 1024:
                    continue # Skip handshake/incomplete
                s_dur = t2 - t1
                speed_mBps = (b / s_dur / 1024 / 1024) if s_dur > 0 else 0
                speed_mbps = (b * 8 / s_dur / 1e6) if s_dur > 0 else 0
                mb = b / 1024 / 1024
                print(f"    Seg #{seg_idx:<4} {mb:6.2f} MB       {s_dur:5.2f}s      {speed_mBps:6.2f} MB/s      {speed_mbps:6.2f} Mbps")
                seg_idx += 1
        print("=" * 70)

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python pcap_analyzer.py <path_to_pcap>")
        sys.exit(1)
    analyze_pcap(sys.argv[1])
