# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# mp-bridge-media-refresh — 媒体库巡检（独立实现，功能对标 MoviePilot 媒体库刷新类插件）。
# 能力全部经宿主 Tool RPC：mediaserver_list / mediaserver_libraries /
# mediaserver_library_scan / mediaserver_refresh / mediaserver_duplicates。
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


def kv_set(key, value):
    host_call("/kv/set", {"key": key, "value": value})


def kv_get_json(key):
    r = host_call("/kv/get", {"key": key}) or {}
    raw = r.get("value", "")
    try:
        return json.loads(raw) if raw else None
    except Exception:
        return None


def as_bool(v, default=False):
    if v is None:
        return default
    if isinstance(v, bool):
        return v
    return str(v).strip().lower() in ("1", "true", "yes", "on", "y")


def split_multi(raw):
    return [p.strip() for p in re.split(r"[\n,;]+", str(raw or "")) if p.strip()]


def _extract_items(out):
    if out is None:
        return []
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("results", "servers", "libraries", "items", "data", "scanned"):
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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "mp-bridge-media-refresh"})
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
    if action == "scan_all":
        return scan_all(cfg)
    if action == "servers":
        return servers()
    if action == "refresh_library":
        return refresh_library(inp)
    if action == "dup_report":
        return dup_report(cfg)
    raise ValueError(f"unknown action: {action}")


def _excluded(cfg):
    return set(split_multi(cfg.get("exclude_libraries")))


def servers():
    out = tool_call("mediaserver_list")
    svrs = []
    for s in _extract_items(out):
        if isinstance(s, dict):
            svrs.append({"id": _pick(s, "id"), "name": _pick(s, "name"),
                         "type": _pick(s, "type"), "address": _pick(s, "address")})
    return {"count": len(svrs), "servers": svrs}


def scan_all(cfg):
    """逐服务器触发全库扫描；排除库不参与。"""
    excl = _excluded(cfg)
    svrs = _extract_items(tool_call("mediaserver_list"))
    results, failed = [], 0
    for s in svrs:
        if not isinstance(s, dict):
            continue
        sid = _pick(s, "id")
        if sid in ("", None):
            continue
        # 取库清单，逐个排除后传 library_id=空 让宿主扫全部（排除在插件侧做细粒度）；
        # 无排除需求时直接传空扫全部，少一轮 libraries 调用。
        try:
            if excl:
                libs = _extract_items(tool_call("mediaserver_libraries", {"server_id": int(sid)}))
                targets = [_pick(l, "id") or _pick(l, "name") for l in libs
                           if isinstance(l, dict) and str(_pick(l, "name")).lower() not in {e.lower() for e in excl}]
                scanned = 0
                for lid in targets:
                    if not lid:
                        continue
                    tool_call("mediaserver_library_scan", {"server_id": int(sid), "library_id": str(lid)})
                    scanned += 1
                results.append({"server": _pick(s, "name"), "scanned_libraries": scanned, "excluded": len(excl)})
            else:
                out = tool_call("mediaserver_library_scan", {"server_id": int(sid), "library_id": ""})
                results.append({"server": _pick(s, "name"), "detail": out})
        except Exception as e:
            failed += 1
            results.append({"server": _pick(s, "name"), "error": str(e)})

    summary = {"servers": len(results), "failed": failed, "results": results}
    kv_set("last_scan", json.dumps(summary, ensure_ascii=False))
    log("info", "media.scan", f"scan triggered on {len(results)} server(s), {failed} failed", None)
    if failed > 0:
        lines = "\n".join(f"• {r.get('server')}：{r.get('error')}" for r in results if r.get("error"))
        notice("媒体库扫描", f"{len(results)} 台服务器中有 {failed} 台失败：\n{lines}", "warning")
    elif as_bool(cfg.get("scan_notify"), False):
        notice("媒体库扫描", f"已触发 {len(results)} 台服务器全库扫描。", "info")
    return summary


def refresh_library(inp):
    sid = inp.get("server_id")
    lid = str(inp.get("library_id") or "")
    if sid in (None, "") or not lid:
        raise ValueError("需要 server_id 与 library_id（来自 servers/servers+libraries）")
    out = tool_call("mediaserver_refresh", {"server_id": int(sid), "library_id": lid})
    log("info", "media.refresh", f"refreshed server#{sid} library {lid}", None)
    return out


def dup_report(cfg):
    """逐服务器逐库检测重复项。"""
    excl = {e.lower() for e in _excluded(cfg)}
    svrs = _extract_items(tool_call("mediaserver_list"))
    total_dups, per, failed = 0, [], 0
    for s in svrs:
        if not isinstance(s, dict):
            continue
        sid = _pick(s, "id")
        if sid in ("", None):
            continue
        try:
            libs = _extract_items(tool_call("mediaserver_libraries", {"server_id": int(sid)}))
        except Exception as e:
            failed += 1
            per.append({"server": _pick(s, "name"), "error": str(e)})
            continue
        for l in libs:
            if not isinstance(l, dict):
                continue
            lname = str(_pick(l, "name"))
            if lname.lower() in excl:
                continue
            lid = _pick(l, "id") or lname
            try:
                dups = _extract_items(tool_call("mediaserver_duplicates", {"server_id": int(sid), "library_id": str(lid)}))
            except Exception as e:
                failed += 1
                per.append({"server": _pick(s, "name"), "library": lname, "error": str(e)})
                continue
            n = len(dups)
            total_dups += n
            if n:
                per.append({"server": _pick(s, "name"), "library": lname, "duplicates": n})
    summary = {"total_duplicates": total_dups, "failed": failed, "detail": per}
    kv_set("last_dup_report", json.dumps(summary, ensure_ascii=False))
    log("info", "media.dups", f"dup report: {total_dups} duplicate group(s), {failed} failed", None)
    if total_dups > 0 or failed > 0 or not as_bool(cfg.get("dup_notify"), True):
        lines = [f"• {d.get('server')} / {d.get('library')}：{d.get('duplicates')} 组" for d in per if d.get("duplicates")]
        body = f"发现 {total_dups} 组重复项" + (f"，{failed} 台失败" if failed else "")
        if lines:
            body += "：\n" + "\n".join(lines[:20])
        notice("媒体库重复项报告", body, "warning" if total_dups else "info")
    return summary


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
