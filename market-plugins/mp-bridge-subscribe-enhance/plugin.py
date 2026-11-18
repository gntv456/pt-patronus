# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# mp-bridge-subscribe-enhance — 订阅增强（独立实现，功能对标 MoviePilot 订阅增强类插件）。
# 能力全部经宿主 Tool RPC：recommend_feeds / subscribe_list / subscribe_create /
# subscribe_check / subscribe_get / send_notification。
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
        with urllib.request.urlopen(req, timeout=120) as res:
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


def as_int(v, default=0):
    try:
        return int(v)
    except Exception:
        return default


def _extract_items(out):
    if out is None:
        return []
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("results", "items", "feeds", "data", "subscriptions", "trending"):
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


def _safe_json_body(raw):
    """畸形/空请求体不应让 handler 抛异常断开连接。"""
    try:
        return json.loads(raw.decode("utf-8", "replace")) if raw else {}
    except Exception:
        return {}
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
        return _safe_json_body(self.rfile.read(n))

    def _auth(self):
        return (not PLUGIN_TOKEN) or self.headers.get("Authorization") == f"Bearer {PLUGIN_TOKEN}"

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "mp-bridge-subscribe-enhance"})
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
    if action == "check_all":
        return check_all(cfg)
    if action == "hot_feeds":
        return hot_feeds(inp)
    if action == "subscribe_from_feed":
        return subscribe_from_feed(inp, cfg)
    if action == "report":
        return report(cfg)
    raise ValueError(f"unknown action: {action}")


def check_all(cfg):
    """逐个 force-check 全部订阅，汇总新增命中。"""
    subs = _extract_items(tool_call("subscribe_list"))
    gap = max(as_int(cfg.get("check_concurrency_note"), 3), 0)
    results, total_new, failed = [], 0, 0
    for s in subs:
        if not isinstance(s, dict):
            continue
        sid = _pick(s, "id")
        if sid in ("", None):
            continue
        try:
            out = tool_call("subscribe_check", {"id": int(sid)})
            new = as_int(out.get("checked") if isinstance(out, dict) else 0, 0)
        except Exception as e:
            new = 0
            failed += 1
            results.append({"id": sid, "title": _pick(s, "title"), "error": str(e)})
            continue
        total_new += new
        results.append({"id": sid, "title": _pick(s, "title"), "new": new})
        if gap:
            time.sleep(gap)

    summary = {"subscriptions": len(results), "new_hits": total_new, "failed": failed, "results": results}
    kv_set("last_check_all", json.dumps(summary, ensure_ascii=False))
    log("info", "sub.check_all", f"checked {len(results)} subs, {total_new} new hits, {failed} failed", None)
    if as_bool(cfg.get("check_notify"), True) and total_new > 0:
        lines = [f"• {r.get('title', r.get('id'))}：+{r.get('new', 0)}" for r in results if r.get("new")]
        notice("订阅批量检查", f"新增命中 {total_new}：\n" + "\n".join(lines[:20]), "success" if failed == 0 else "warning")
    return summary


def hot_feeds(inp):
    """热榜条目 + 已订阅标注。"""
    out = tool_call("recommend_feeds", {"source": str(inp.get("source") or ""), "category": str(inp.get("category") or "")})
    items = _extract_items(out)
    subs = _extract_items(tool_call("subscribe_list"))
    sub_keys = set()
    for s in subs:
        if isinstance(s, dict):
            sub_keys.add((str(_pick(s, "source")), str(_pick(s, "source_id"))))
            sub_keys.add((str(_pick(s, "title")).lower(),))
    feeds = []
    for it in items[:50]:
        if not isinstance(it, dict):
            continue
        key = (str(_pick(it, "source")), str(_pick(it, "source_id")))
        title_low = (str(_pick(it, "title")).lower(),)
        feeds.append({
            "title": _pick(it, "title"), "type": _pick(it, "type", "category"),
            "year": _pick(it, "year"), "rating": _pick(it, "rating", "score"),
            "source": _pick(it, "source"), "source_id": _pick(it, "source_id"),
            "subscribed": key in sub_keys or title_low in sub_keys,
        })
    return {"count": len(feeds), "feeds": feeds}


def subscribe_from_feed(inp, cfg):
    """按热榜条目建订阅；已存在（source+source_id 命中）则跳过。"""
    title = str(_pick(inp, "title"))
    typ = str(_pick(inp, "type", "category")) or "movie"
    source = str(_pick(inp, "source"))
    source_id = str(_pick(inp, "source_id"))
    if not (title and source and source_id):
        raise ValueError("需要 title/type/source/source_id（来自 hot_feeds 条目）")
    subs = _extract_items(tool_call("subscribe_list"))
    for s in subs:
        if isinstance(s, dict) and str(_pick(s, "source")) == source and str(_pick(s, "source_id")) == source_id:
            return {"created": False, "reason": "already subscribed", "title": title}
    out = tool_call("subscribe_create", {
        "title": title, "type": typ, "source": source, "source_id": source_id,
        "year": as_int(inp.get("year"), 0) or None, "resolution": str(_pick(inp, "resolution")),
    })
    log("info", "sub.created", f"subscribed {title} from feed", {"source": source, "source_id": source_id})
    return {"created": True, "title": title, "subscription": out}


def report(cfg):
    """订阅状态汇总 + 周报通知。"""
    subs = _extract_items(tool_call("subscribe_list"))
    by_type, by_status = {}, {}
    for s in subs:
        if not isinstance(s, dict):
            continue
        t = str(_pick(s, "type", "category")) or "unknown"
        st = str(_pick(s, "status", "state")) or "unknown"
        by_type[t] = by_type.get(t, 0) + 1
        by_status[st] = by_status.get(st, 0) + 1
    summary = {"total": len(subs), "by_type": by_type, "by_status": by_status}
    kv_set("last_report", json.dumps(summary, ensure_ascii=False))
    if as_bool(cfg.get("report_notify"), True):
        lines = [f"总数 {len(subs)}"] + [f"• {k}: {v}" for k, v in sorted(by_status.items())]
        notice("订阅周报", "\n".join(lines), "info")
    log("info", "sub.report", f"report generated: {len(subs)} subs", None)
    return summary


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
