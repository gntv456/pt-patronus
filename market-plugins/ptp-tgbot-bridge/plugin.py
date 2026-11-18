# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# ptp-tgbot-bridge — TG Bot 桥接面（独立实现，见 issue #34 二梯队）。
# bot 轮询本体在宿主进程（依赖 in-process AI 流式会话）；本插件承接
# 状态巡检 / 配置重载 / AI 单发对话，经 tgbot_status/tgbot_reload/agent_chat 工具。
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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "ptp-tgbot-bridge"})
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
    if action == "status":
        return tool_call("tgbot_status")
    if action == "reload":
        return tool_call("tgbot_reload")
    if action == "health":
        return health(cfg)
    if action == "ask":
        return ask(inp)
    raise ValueError(f"unknown action: {action}")


def health(cfg):
    """巡检：配置启用但未运行 → 告警 +（可选）自动重载；正常静默。"""
    st = tool_call("tgbot_status")
    enabled = as_bool(st.get("config_enabled"), False)
    running = as_bool(st.get("running"), False)
    safe = as_bool(st.get("safe_mode"), False)
    problem = None
    if safe:
        problem = "宿主处于 SAFE_MODE（插件/bot 全停，属预期）"
        level = "info"
    elif enabled and not running:
        problem = "TG bot 已配置但未运行（可能改配置后未重启）"
        level = "warning"
    if problem is None:
        kv_set_json("last_health", {"ok": True, "ts": st})
        return {"ok": True}
    log("warn", "tgbot.health", problem, st)
    if enabled and not running and as_bool(cfg.get("auto_reload"), True):
        tool_call("tgbot_reload")
        recheck = tool_call("tgbot_status")
        fixed = as_bool(recheck.get("running"), False)
        problem += f"；已自动 reload，重载后 running={fixed}"
        if fixed:
            level = "info"
    if as_bool(cfg.get("notify"), True):
        notice("TG Bot 巡检", problem, level if problem.startswith("宿主") or "running=True" in problem else "warning")
    kv_set_json("last_health", {"ok": False, "problem": problem, "status": st})
    return {"ok": False, "problem": problem, "status": st}


def ask(inp):
    text = str((inp or {}).get("text") or "").strip()
    if not text:
        raise ValueError("text 必填")
    out = tool_call("agent_chat", {"text": text})
    return {"reply": out.get("reply")}


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
