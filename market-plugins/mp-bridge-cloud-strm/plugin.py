# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# mp-bridge-cloud-strm — 网盘 STRM 生成联动（独立实现，功能对标 MoviePilot 网盘 STRM 类插件）。
# 能力全部经宿主 Tool RPC：cloud_list / cloud_rules / cloud_generate_strm /
# cloud_generate_all / cloud_records。
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
        with urllib.request.urlopen(req, timeout=300) as res:
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
        for k in ("results", "rules", "drives", "records", "items", "data"):
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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "mp-bridge-cloud-strm"})
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
    if action == "generate_all":
        return generate_all(cfg)
    if action == "generate_rule":
        return generate_rule(inp)
    if action == "rules":
        return {"rules": _extract_items(tool_call("cloud_rules"))}
    if action == "drives":
        return {"drives": _extract_items(tool_call("cloud_list"))}
    if action == "records":
        return {"records": _extract_items(tool_call("cloud_records", {"drive_id": as_int(inp.get("drive_id"), 0)}))}
    if action == "health_report":
        return health_report(cfg)
    raise ValueError(f"unknown action: {action}")


def generate_all(cfg):
    """同步生成全部启用规则；宿主返回跨规则汇总（超时由 host 兜底）。"""
    out = tool_call("cloud_generate_all")
    summary = out if isinstance(out, dict) else {"detail": out}
    rules = as_int(_pick(summary, "rules", "rules_total"), 0)
    generated = as_int(_pick(summary, "generated", "count"), 0)
    failed = as_int(_pick(summary, "failed"), 0)
    kv_set("last_generate", json.dumps(summary, ensure_ascii=False))
    log("info", "strm.generate", f"generate_all done: rules={rules} generated={generated} failed={failed}", None)
    if failed > 0:
        notice("网盘 STRM", f"生成完成但有失败：规则 {rules}、生成 {generated}、失败 {failed}（详见插件日志）。", "warning")
    elif as_bool(cfg.get("generate_notify"), True) is False:
        pass  # 默认静音：夜间生成不打扰
    return summary


def generate_rule(inp):
    rid = inp.get("rule_id")
    if rid in (None, ""):
        raise ValueError("需要 rule_id（来自 rules）")
    out = tool_call("cloud_generate_strm", {"rule_id": int(rid)})
    log("info", "strm.generate_rule", f"generate rule #{rid}", None)
    return out


def health_report(cfg):
    """规则健康巡检：规则数/启用数/上次生成结果。"""
    rules = _extract_items(tool_call("cloud_rules"))
    drives = _extract_items(tool_call("cloud_list"))
    enabled = sum(1 for r in rules if isinstance(r, dict) and as_bool(r.get("enabled"), False))
    last = kv_get_json("last_generate") or {}
    last_failed = as_int(_pick(last, "failed"), 0)
    summary = {
        "rules_total": len(rules), "rules_enabled": enabled,
        "drives": len(drives), "last_generate": last,
    }
    kv_set("last_health", json.dumps(summary, ensure_ascii=False))
    log("info", "strm.health", f"health: {len(rules)} rules ({enabled} enabled), {len(drives)} drives", None)
    if as_bool(cfg.get("health_notify"), True):
        lines = [f"规则 {len(rules)} 条（启用 {enabled}）", f"网盘 {len(drives)} 台"]
        if last:
            lines.append(f"上次生成：rules={last_failed and last.get('rules', '?')} generated={last.get('generated', '?')} failed={last_failed}")
        if last_failed > 0:
            notice("网盘 STRM 巡检", "\n".join(lines) + "\n⚠ 上次生成存在失败，请检查网盘凭据/目录。", "warning")
        else:
            notice("网盘 STRM 巡检", "\n".join(lines), "info")
    return summary


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
