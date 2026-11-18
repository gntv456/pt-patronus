# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
import json, os, re, time, socket, ipaddress, hashlib, base64, subprocess, urllib.request, urllib.error, urllib.parse, hmac, struct
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from datetime import datetime, timezone, timedelta
from pathlib import Path
from email.utils import parsedate_to_datetime

PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")
HOST_URL = os.environ.get("PTP_HOST_URL", "")
HOST_TOKEN = os.environ.get("PTP_HOST_TOKEN", "")
PLUGIN_ID = os.environ.get("PTP_PLUGIN_ID", "")

def host_call(path, body=None, method="POST"):
    if not HOST_URL or not HOST_TOKEN: return None
    data=None; headers={"Authorization": f"Bearer {HOST_TOKEN}"}
    if body is not None:
        data=json.dumps(body, ensure_ascii=False).encode("utf-8"); headers["Content-Type"]="application/json"
    req=urllib.request.Request(HOST_URL.rstrip("/")+path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as res:
            raw=res.read().decode("utf-8"); return json.loads(raw) if raw else {}
    except Exception:
        return None
def log(level, event, message, data=None): host_call("/log", {"level":level,"event":event,"message":message,"data":data or {}})
def notice(title, body, level="info"): host_call("/notice", {"title":title,"body":body,"level":level})
def publish_event(etype, data=None): host_call("/events", {"type":etype,"data":data or {}})
def kv_get(key):
    r=host_call("/kv/get",{"key":key}) or {}; return r.get("value","")
def kv_set(key, value): host_call("/kv/set",{"key":key,"value":value})
def as_bool(v, default=False):
    if v is None: return default
    if isinstance(v,bool): return v
    return str(v).strip().lower() in ("1","true","yes","on","y")
def as_int(v, default=0):
    try: return int(v)
    except Exception: return default
def split_multi(raw):
    return [p.strip() for p in re.split(r"[\n,;]+", str(raw or "")) if p.strip()]
def http_json(url, method="GET", headers=None, data=None, timeout=10, form=None):
    body=None; hdrs=dict(headers or {}); hdrs.setdefault("User-Agent","PTPatronus-Plugin/1.0")
    if form is not None:
        body=urllib.parse.urlencode(form).encode(); hdrs["Content-Type"]="application/x-www-form-urlencoded"; method="POST"
    elif data is not None:
        body=json.dumps(data).encode(); hdrs["Content-Type"]="application/json"
    req=urllib.request.Request(url, data=body, headers=hdrs, method=method.upper())
    with urllib.request.urlopen(req, timeout=timeout) as res:
        raw=res.read().decode("utf-8","replace"); return json.loads(raw) if raw.strip().startswith(("{","[")) else {"raw":raw}

class Handler(BaseHTTPRequestHandler):
    def _json(self,s,p):
        raw=json.dumps(p, ensure_ascii=False).encode(); self.send_response(s); self.send_header("Content-Type","application/json; charset=utf-8"); self.send_header("Content-Length",str(len(raw))); self.end_headers(); self.wfile.write(raw)
    def _body(self):
        n=int(self.headers.get("Content-Length","0") or "0"); return json.loads(self.rfile.read(n).decode()) if n else {}
    def _auth(self): return (not PLUGIN_TOKEN) or self.headers.get("Authorization")==f"Bearer {PLUGIN_TOKEN}"
    def do_GET(self):
        self._json(200,{"ok":True,"plugin":PLUGIN_ID or "plugin"}) if self.path=="/health" else self._json(404,{"error":"not found"})
    def do_POST(self):
        if not self._auth(): self._json(401,{"error":"unauthorized"}); return
        payload=self._body()
        if self.path=="/action":
            try: self._json(200,{"ok":True,"output":handle_action(payload.get("action"), payload.get("input") or {}, payload.get("config") or {}, payload.get("host") or {})})
            except Exception as e: log("error","action.error",str(e),{"action":payload.get("action")}); self._json(200,{"ok":False,"error":str(e)})
            return
        if self.path=="/event":
            try: handle_event(payload.get("type") or payload.get("event") or "", payload.get("data") or {}, payload.get("config") or {}); self._json(200,{"ok":True})
            except Exception as e: log("error","event.error",str(e)); self._json(200,{"ok":False,"error":str(e)})
            return
        self._json(404,{"error":"not found"})
    def log_message(self,*a): return



def handle_event(event_type, data, cfg):
    return
def _guard_target_url(url, cfg, allow_private_default=False):
    u=urllib.parse.urlparse(str(url or "").strip())
    if u.scheme not in ("http","https") or not u.hostname:
        raise ValueError("仅支持有效 HTTP/HTTPS URL")
    if not as_bool(cfg.get("allow_private_network"), allow_private_default):
        try:
            for info in socket.getaddrinfo(u.hostname, None):
                ip=ipaddress.ip_address(info[4][0])
                if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved:
                    raise ValueError("目标为内网/回环地址，如属预期请在配置中开启 allow_private_network")
        except ValueError:
            raise
        except Exception:
            pass
    return u.geturl()
def _fetch_lines(urls, timeout=15, cfg=None):
    cfg=cfg or {}
    lines=[]
    for u in urls:
        u=_guard_target_url(u, cfg)
        req=urllib.request.Request(u, headers={"User-Agent":"PTPatronus-CF/1.0"})
        with urllib.request.urlopen(req, timeout=timeout) as res:
            text=res.read().decode("utf-8","replace")
        for ln in text.splitlines():
            ln=ln.strip()
            if not ln or ln.startswith("#"): continue
            lines.append(ln)
    # unique keep order
    seen=set(); out=[]
    for ln in lines:
        if ln not in seen: seen.add(ln); out.append(ln)
    return out
def _wrap(marker, lines):
    begin=f"# BEGIN {marker}"; end=f"# END {marker}"
    return "\n".join([begin, *lines, end]) + "\n"
def _resolve_hosts_path(raw_path, cfg):
    """只写系统 hosts 文件、插件目录，或 allowed_roots 显式授权的目录。"""
    raw=str(raw_path or "").strip()
    if not raw: return None
    p=Path(raw).expanduser()
    if not p.is_absolute():
        p=Path(os.environ.get("PTP_PLUGIN_DIR", ".")) / p
    try:
        rp=p.resolve(strict=False)
    except Exception:
        raise ValueError("hosts 路径无效")
    if rp.exists() and rp.is_dir():
        raise ValueError("hosts 路径不能是目录")
    allowed=[]
    for d in ("/etc/hosts", r"C:\Windows\System32\drivers\etc\hosts",
              os.environ.get("PTP_PLUGIN_DIR", ".")):
        try: allowed.append(Path(d).resolve(strict=False))
        except Exception: pass
    for a in split_multi(cfg.get("allowed_roots")):
        ap=Path(a).expanduser()
        if not ap.is_absolute():
            ap=Path(os.environ.get("PTP_PLUGIN_DIR", ".")) / ap
        try: allowed.append(ap.resolve(strict=False))
        except Exception: pass
    for root in allowed:
        if rp==root: return rp
        try:
            if root in rp.parents: return rp
        except Exception: pass
    raise ValueError("hosts 路径不在允许范围；请在 allowed_roots 中显式授权该目录")
def handle_action(action, inp, cfg, host):
    urls=split_multi(cfg.get("subscribe_urls"))
    if not urls: raise ValueError("未配置 subscribe_urls")
    lines=_fetch_lines(urls, cfg=cfg)
    marker=str(cfg.get("marker") or "PTPATRONUS-CF")
    block=_wrap(marker, lines)
    if action=="preview":
        return {"count":len(lines),"preview":block[:4000]}
    if action!="update_hosts": raise ValueError("unknown action")
    path=str(cfg.get("hosts_path") or "").strip()
    if not path:
        kv_set("cf_hosts_block", block)
        if as_bool(cfg.get("notify"), True): notice("Cloudflare订阅", f"已保存 {len(lines)} 行到插件 KV", "info")
        return {"saved":"kv","count":len(lines)}
    p=_resolve_hosts_path(path, cfg)
    old=p.read_text(encoding="utf-8", errors="ignore") if p.exists() else ""
    begin=f"# BEGIN {marker}"; end=f"# END {marker}"
    if begin in old and end in old:
        pre=old.split(begin)[0]; post=old.split(end,1)[1]
        # drop leading newline of post marker line
        if post.startswith("\n"): post=post[1:]
        new=pre+block+post
    else:
        if old and not old.endswith("\n"): old+="\n"
        new=old+"\n"+block
    # 写入前备份，失败可回滚
    bak=p.with_name(p.name+".ptp-bak")
    if p.exists():
        try: bak.write_bytes(p.read_bytes())
        except Exception as e: raise ValueError(f"备份 hosts 文件失败: {e}")
    p.write_text(new, encoding="utf-8")
    if as_bool(cfg.get("notify"), True): notice("Cloudflare订阅", f"已更新 {p} ({len(lines)} 行)", "info")
    log("info","cloudflare-subscribe","hosts updated",{"path":str(p),"count":len(lines),"backup":str(bak) if bak.exists() else None})
    return {"saved":str(p),"count":len(lines),"backup":str(bak) if bak.exists() else None}


if __name__ == "__main__":
    from http.server import ThreadingHTTPServer
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT","19090"))), Handler).serve_forever()
