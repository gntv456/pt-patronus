# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# ptp-cookiecloud-sync — CookieCloud 同步调度器（独立实现，替代宿主进程内 cookiecloud-tick cron，
# 见 issue #34 二梯队）。解密/站点匹配语义经 site_sync_cookies 工具留在宿主链；
# 本插件承接 cron 触发、回推开关与报告通知。
import json, os, time
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
        with urllib.request.urlopen(req, timeout=180) as res:
            raw = res.read().decode("utf-8")
            return json.loads(raw) if raw else {}
    except Exception as e:
        log("error", "host.call", f"{path}: {e}")
        return None


def tool_call(name, inp=None):
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


def kv_set_json(key, obj):
    host_call(f"/kv/{key}", {"value": obj}, method="PUT")


def kv_get_json(key):
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


def as_int(v, default=0):
    try:
        return int(v)
    except Exception:
        return default


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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "ptp-cookiecloud-sync"})
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
    if action == "sync":
        return sync(cfg)
    if action == "status":
        return {"last_sync": kv_get_json("last_sync")}
    raise ValueError(f"unknown action: {action}")


def sync(cfg):
    """触发一轮宿主 CookieCloud 同步（可选回推），通知按配置。"""
    push = as_bool(cfg.get("push"), False) or as_bool(cfg.get("_push_once"), False)
    out = tool_call("site_sync_cookies", {"push": push})

    updated = as_int(out.get("updated"), 0)
    added = as_int(out.get("added"), 0)
    pushed = as_int(out.get("pushed"), 0)
    errors = out.get("errors") or []

    summary = {"ts": time.time(), "updated": updated, "added": added,
               "skipped": as_int(out.get("skipped"), 0), "pushed": pushed,
               "errors": errors, "fetched_at": out.get("fetched_at")}
    kv_set_json("last_sync", summary)
    log("info", "cc.sync", f"sync done: +{updated}u +{added}a skip={summary['skipped']} push={pushed} err={len(errors)}", None)

    if errors and as_bool(cfg.get("notify_errors"), True):
        notice("CookieCloud", f"同步有 {len(errors)} 个失败：\n" + "\n".join(f"• {e}" for e in errors[:10]), "warning")
    elif (updated > 0 or added > 0) and as_bool(cfg.get("notify_updated"), True):
        body = f"更新 {updated}、新增 {added}、跳过 {summary['skipped']}"
        if push:
            body += f"、回推 {pushed}"
        notice("CookieCloud", body, "success")
    return summary


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
