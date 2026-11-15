# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# mp-bridge-site-signin — 全站定时签到 + 结果通知（独立实现，功能对标 MoviePilot 签到类插件）。
# 能力全部经宿主 Tool RPC：site_checkin_all / site_messages / site_freeleech。
import json, os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")
HOST_URL = os.environ.get("PTP_HOST_URL", "")
HOST_TOKEN = os.environ.get("PTP_HOST_TOKEN", "")
PLUGIN_ID = os.environ.get("PTP_PLUGIN_ID", "")


def host_call(path, body=None, method="POST"):
    if not HOST_URL or not HOST_TOKEN:
        return None
    data = None
    headers = {"Authorization": f"Bearer {HOST_TOKEN}"}
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    import urllib.request
    req = urllib.request.Request(HOST_URL.rstrip("/") + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=120) as res:
            raw = res.read().decode("utf-8")
            return json.loads(raw) if raw else {}
    except Exception as e:
        log("error", "host.call", f"{path}: {e}")
        return None


def tool_call(name, inp=None):
    """宿主 Tool RPC：POST /tools/{name}，body={"input":{...}}。"""
    r = host_call(f"/tools/{name}", {"input": inp or {}})
    if r is None:
        raise RuntimeError(f"host tool {name} unreachable")
    if isinstance(r, dict) and r.get("error"):
        raise RuntimeError(f"{name}: {r.get('error')}{'（缺权限 ' + str(r.get('permission')) + '）' if r.get('permission') else ''}")
    return r


def log(level, event, message, data=None):
    host_call("/log", {"level": level, "event": event, "message": message, "data": data or {}})


def notice(title, body, level="info"):
    host_call("/notice", {"title": title, "body": body, "level": level})


def kv_set(key, value):
    """宿主契约：PUT /kv/{key}，body={"value":...}（SDK client.py 同形）。"""
    host_call(f"/kv/{key}", {"value": value}, method="PUT")


def kv_get_json(key):
    """宿主契约：GET /kv/{key} → {"found":bool,"value":...}。"""
    r = host_call(f"/kv/{key}", method="GET") or {}
    if not r.get("found"):
        return None
    val = r.get("value")
    if isinstance(val, (dict, list, bool, int, float)):
        return val
    if isinstance(val, str) and val.strip():
        try:
            return json.loads(val)
        except Exception:
            return None
    return None


def as_bool(v, default=False):
    if v is None:
        return default
    if isinstance(v, bool):
        return v
    return str(v).strip().lower() in ("1", "true", "yes", "on", "y")


def _extract_items(out):
    """site_checkin_all/site_messages/site_freeleech 的返回形状（list 或 {results:[...]}）归一。"""
    if out is None:
        return []
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("results", "items", "messages", "data"):
            v = out.get(k)
            if isinstance(v, list):
                return v
        return [out]
    return []


def _pick(d, *keys):
    if not isinstance(d, dict):
        return ""
    for k in keys:
        if d.get(k) not in (None, ""):
            return d.get(k)
    return ""


class Handler(BaseHTTPRequestHandler):
    def _json(self, s, p):
        raw = json.dumps(p, ensure_ascii=False).encode()
        self.send_response(s)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _body(self):
        n = int(self.headers.get("Content-Length", "0") or "0")
        return json.loads(self.rfile.read(n).decode()) if n else {}

    def _auth(self):
        return (not PLUGIN_TOKEN) or self.headers.get("Authorization") == f"Bearer {PLUGIN_TOKEN}"

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "mp-bridge-site-signin"})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        if not self._auth():
            self._json(401, {"error": "unauthorized"})
            return
        payload = self._body()
        if self.path == "/action":
            try:
                out = handle_action(payload.get("action"), payload.get("input") or {}, payload.get("config") or {})
                self._json(200, {"ok": True, "output": out})
            except Exception as e:
                log("error", "action.error", str(e), {"action": payload.get("action")})
                self._json(200, {"ok": False, "error": str(e)})
            return
        if self.path == "/event":
            self._json(200, {"ok": True})
            return
        self._json(404, {"error": "not found"})

    def log_message(self, *a):
        return


def handle_event(event_type, data, cfg):
    return


def handle_action(action, inp, cfg):
    if action == "run_checkin":
        return run_checkin(cfg)
    if action == "fetch_messages":
        return fetch_messages(inp)
    if action == "fetch_freeleech":
        return fetch_freeleech()
    if action == "last_result":
        return {"result": kv_get_json("last_checkin")}
    raise ValueError(f"unknown action: {action}")


def run_checkin(cfg):
    """全站签到 → 汇总 → 通知 → KV 缓存。"""
    out = tool_call("site_checkin_all")
    items = _extract_items(out)

    total = ok = failed = skipped = 0
    ok_names, fail_lines = [], []
    for it in items:
        if not isinstance(it, dict):
            continue
        total += 1
        name = _pick(it, "site_name", "site", "name", "title") or "?"
        success = it.get("success", it.get("ok"))
        status = str(_pick(it, "status", "state", "message", "msg") or "")
        low = status.lower()
        if success is False or any(w in low for w in ("fail", "error", "失败", "错误")):
            failed += 1
            fail_lines.append(f"• {name}：{status or '失败'}")
        elif any(w in low for w in ("skip", "已签", "跳过", "already", "signed")):
            skipped += 1
            ok_names.append(name)
        else:
            ok += 1
            ok_names.append(name)

    summary = {"total": total, "ok": ok, "failed": failed, "skipped": skipped,
               "ok_names": ok_names, "failures": fail_lines}
    kv_set("last_checkin", json.dumps(summary, ensure_ascii=False))
    log("info", "signin.done", f"checkin finished: {ok} ok / {failed} failed / {skipped} skipped", summary)

    if as_bool(cfg.get("notify"), True):
        if failed > 0:
            level = "warning"
            body = f"成功 {ok}、跳过 {skipped}、失败 {failed}：\n" + "\n".join(fail_lines[:20])
            notice("站点签到", body, level)
        elif as_bool(cfg.get("notify_on_success"), False):
            level = str(cfg.get("notify_level") or "info")
            body = f"全部成功（{ok} 站" + (f"，跳过 {skipped}" if skipped else "") + "）。"
            notice("站点签到", body, level)
    return summary


def fetch_messages(inp):
    out = tool_call("site_messages", {"mailbox": str(inp.get("mailbox") or "inbox")})
    items = _extract_items(out)
    msgs = []
    for it in items[:50]:
        if isinstance(it, dict):
            msgs.append({
                "site": _pick(it, "site_name", "site", "name"),
                "title": _pick(it, "title", "subject"),
                "sender": _pick(it, "sender", "from"),
                "time": _pick(it, "time", "date", "created_at"),
            })
    return {"count": len(msgs), "messages": msgs}


def fetch_freeleech():
    out = tool_call("site_freeleech")
    items = _extract_items(out)
    promos = []
    for it in items[:50]:
        if isinstance(it, dict):
            promos.append({
                "site": _pick(it, "site_name", "site", "name"),
                "kind": _pick(it, "kind", "type", "discount", "promotion"),
                "until": _pick(it, "until", "end_time", "expires"),
            })
    return {"count": len(promos), "promotions": promos}


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
