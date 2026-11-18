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
def _parse_field(field, min_v, max_v):
    vals=set()
    for part in str(field).split(","):
        part=part.strip()
        if part=="*":
            vals.update(range(min_v, max_v+1)); continue
        step=1
        if "/" in part:
            part, step_s = part.split("/",1); step=int(step_s)
        if part=="*":
            vals.update(range(min_v, max_v+1, step)); continue
        if "-" in part:
            a,b=part.split("-",1); a=int(a); b=int(b)
            vals.update(range(a, b+1, step)); continue
        vals.add(int(part))
    out=set()
    for v in vals:
        # 标准 cron 允许 7 表示周日
        if max_v==6 and v==7: v=0
        if min_v<=v<=max_v: out.add(v)
    return sorted(out)
def _nth_weekday(year, month, weekday, n):
    first=datetime(year, month, 1)
    return datetime(year, month, 1+((weekday-first.weekday())%7)+(n-1)*7)
def _last_weekday(year, month, weekday):
    nxt=datetime(year+1,1,1) if month==12 else datetime(year,month+1,1)
    last=nxt-timedelta(days=1)
    return last-timedelta(days=(last.weekday()-weekday)%7)
def _zone_offset(utc_dt, tz):
    """按 UTC 时刻返回目标时区偏移，仅用标准库；覆盖常见固定偏移与美/欧夏令时。"""
    name=str(tz or "UTC").strip()
    m=re.match(r"^(?:UTC|GMT)?\s*([+-])(\d{1,2})(?::?(\d{2}))?$", name, re.I)
    if m:
        sign=1 if m.group(1)=="+" else -1
        return timedelta(minutes=sign*(int(m.group(2))*60+int(m.group(3) or 0)))
    fixed={
        "UTC":0, "GMT":0, "Z":0,
        "Asia/Shanghai":480, "Asia/Beijing":480, "Asia/Hong_Kong":480,
        "Asia/Singapore":480, "Asia/Tokyo":540, "CST":480,
        "America/New_York":-300, "America/Chicago":-360,
        "America/Denver":-420, "America/Los_Angeles":-480,
        "Europe/London":0, "Europe/Paris":60, "Europe/Berlin":60,
    }
    base_min=fixed.get(name)
    if base_min is None: return timedelta(0)  # 未识别时区退回 UTC，不再误按 UTC+8
    base=timedelta(minutes=base_min); y=utc_dt.year
    if name.startswith("America/"):
        # 美国夏令时：3 月第二个周日 02:00 起，11 月第一个周日 02:00 止
        start=_nth_weekday(y,3,6,2)+timedelta(hours=2)-base
        end=_nth_weekday(y,11,6,1)+timedelta(hours=2)-base-timedelta(hours=1)
        if start<=utc_dt<end: return base+timedelta(hours=1)
    elif name.startswith("Europe/"):
        # 欧洲夏令时：3 月最后一个周日 01:00 UTC 起，10 月最后一个周日 01:00 UTC 止
        if _last_weekday(y,3,6)+timedelta(hours=1)<=utc_dt<_last_weekday(y,10,6)+timedelta(hours=1):
            return base+timedelta(hours=1)
    return base
def _next_runs(expr, count, tz="Asia/Shanghai"):
    parts=expr.split()
    if len(parts)!=5: raise ValueError("需要标准 5 段 cron: m h dom mon dow")
    minutes=_parse_field(parts[0],0,59); hours=_parse_field(parts[1],0,23)
    doms=_parse_field(parts[2],1,31); months=_parse_field(parts[3],1,12); dows=_parse_field(parts[4],0,6)
    if not all((minutes, hours, doms, months, dows)):
        raise ValueError("Cron 表达式包含无有效取值的字段（如 61 分钟、13 月）")
    dom_restricted=parts[2].strip()!="*"; dow_restricted=parts[4].strip()!="*"
    # 在 UTC 时间轴上推进，再换算为目标时区的墙上时间比较
    now_utc=datetime.now(timezone.utc).replace(tzinfo=None)
    t=now_utc.replace(second=0, microsecond=0)+timedelta(minutes=1)
    out=[]; guard=0; max_guard=60*24*366*4  # 4 年，覆盖闰日等稀有表达式
    while len(out)<count and guard<max_guard:
        guard+=1
        off=_zone_offset(t, tz)
        local=t+off
        day_match=local.day in doms
        dow_match=((local.weekday()+1)%7) in dows
        if dom_restricted and dow_restricted:
            day_ok=day_match or dow_match  # 标准 cron：dom 与 dow 同时受限取 OR
        else:
            day_ok=day_match and dow_match
        if local.month in months and day_ok and local.hour in hours and local.minute in minutes:
            out.append(local.strftime("%Y-%m-%d %H:%M:%S"))
            t+=timedelta(minutes=1); continue
        # 跳过不匹配的整天/整小时；必须在“本地时间”上推进再减回 UTC 偏移，
        # 否则 UTC 00:00 对应东八区 08:00，会跳过当天凌晨的匹配点。
        if local.month not in months or not day_ok:
            t=(local+timedelta(days=1)).replace(hour=0, minute=0)-off
        elif local.hour not in hours:
            t=(local+timedelta(hours=1)).replace(minute=0)-off
        else:
            t+=timedelta(minutes=1)
    return out
def handle_action(action, inp, cfg, host):
    if action!="next_runs": raise ValueError("unknown action")
    expr=str(inp.get("expression") or "").strip()
    if not expr: raise ValueError("Cron 表达式不能为空")
    count=min(20, max(1, as_int(inp.get("count"),5) or 5))
    tz=str(inp.get("timezone") or "Asia/Shanghai")
    runs=_next_runs(expr, count, tz)
    log("info","cron-helper","cron next runs calculated",{"expression":expr,"count":count,"timezone":tz})
    return {"expression":expr,"timezone":tz,"next_runs":runs}


if __name__ == "__main__":
    from http.server import ThreadingHTTPServer
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT","19090"))), Handler).serve_forever()
