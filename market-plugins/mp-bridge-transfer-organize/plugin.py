# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# mp-bridge-transfer-organize — 下载完成自动整理（独立实现，功能对标 MoviePilot 自动整理类插件）。
# 能力全部经宿主 Tool RPC：downloaders.list / torrents_list / transfer_organize /
# transfer_history / transfer_redo / transfer_sweep。
import json, os, re
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


def split_multi(raw):
    return [p.strip().lower() for p in re.split(r"[\n,;]+", str(raw or "")) if p.strip()]


def _extract_items(out):
    """宿主工具返回形状（list 或 {results|torrents|items|data:[...]}）归一。"""
    if out is None:
        return []
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("results", "torrents", "items", "data", "history"):
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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "mp-bridge-transfer-organize"})
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
    if action == "sweep":
        return sweep(cfg)
    if action == "completed_torrents":
        return completed_torrents(cfg)
    if action == "organize":
        return organize(inp)
    if action == "history":
        return history(inp)
    if action == "redo":
        return redo(inp)
    raise ValueError(f"unknown action: {action}")


def sweep(cfg):
    """触发宿主全量整理扫描（后台异步）。"""
    out = tool_call("transfer_sweep")
    kv_set("last_sweep", json.dumps({"note": out.get("note", ""), "started": True}, ensure_ascii=False))
    log("info", "organize.sweep", "organize sweep triggered", out)
    if as_bool(cfg.get("notify"), False):
        notice("下载整理", "已触发全量整理扫描（后台进行，完成后看整理历史）。", "info")
    return out


def completed_torrents(cfg):
    """跨下载器列出已完成种子（整理候选）。"""
    states = split_multi(cfg.get("completed_states")) or ["completed", "seeding", "uploading"]
    dls = _extract_items(tool_call("downloaders.list"))
    out = []
    for dl in dls:
        if not isinstance(dl, dict):
            continue
        dl_id = _pick(dl, "id")
        if dl_id in ("", None):
            continue
        try:
            torrents = _extract_items(tool_call("torrents_list", {"downloader_id": int(dl_id)}))
        except Exception as e:
            out.append({"downloader": _pick(dl, "name"), "error": str(e)})
            continue
        done = []
        for t in torrents:
            if not isinstance(t, dict):
                continue
            state = str(_pick(t, "state", "status")).lower()
            if state in states:
                done.append({"hash": _pick(t, "hash"), "name": _pick(t, "name"),
                             "state": state, "progress": _pick(t, "progress")})
        out.append({"downloader_id": dl_id, "downloader": _pick(dl, "name"),
                    "completed": len(done), "torrents": done})
    total = sum(x.get("completed", 0) for x in out if isinstance(x, dict))
    kv_set("last_completed", json.dumps({"total": total, "per_downloader": out}, ensure_ascii=False))
    return {"total": total, "downloaders": out}


def organize(inp):
    """单种子手动整理。"""
    dl_id = inp.get("downloader_id")
    h = str(inp.get("hash") or "").strip()
    if dl_id in (None, "") or not h:
        raise ValueError("需要 downloader_id 与 hash")
    out = tool_call("transfer_organize", {"downloader_id": int(dl_id), "hash": h})
    log("info", "organize.one", f"organize {h[:12]}…", {"downloader_id": dl_id})
    notice("下载整理", f"种子 {h[:12]}… 已提交整理。", "info")
    return out


def history(inp):
    """整理历史（status=success|failed|空）。"""
    q = {"status": str(inp.get("status") or ""), "limit": int(inp.get("limit") or 20)}
    out = tool_call("transfer_history", q)
    return {"history": _extract_items(out)}


def redo(inp):
    """按 history_id 重做整理。"""
    hid = inp.get("history_id")
    if hid in (None, ""):
        raise ValueError("需要 history_id")
    out = tool_call("transfer_redo", {"history_id": int(hid)})
    log("info", "organize.redo", f"redo history #{hid}", None)
    return out


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
