# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# ptp-dirwatch — 本地目录监视 → 整理流水线（独立实现，替代宿主进程内 dirwatch，见 issue #34）。
# 轮询语义：cron 周期扫描监视目录，媒体文件喂 transfer_discovered；per-dir 去重锚点存 KV
#（整理侧 ProcessDiscoveredFiles 还有硬链级去重兜底，锚点丢失不致重复入库）。
import json, os, re, time
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


def split_lines(raw):
    return [l.strip() for l in re.split(r"[\n\r]+", str(raw or "")) if l.strip()]


def split_multi(raw):
    return [p.strip().lower() for p in re.split(r"[,;]+", str(raw or "")) if p.strip()]


def parse_dirs(raw):
    """每行「路径[|库路径]」→ [(watch_dir, src_dir)]。"""
    out = []
    for line in split_lines(raw):
        parts = line.split("|", 1)
        watch = parts[0].strip()
        if not watch:
            continue
        src = (parts[1].strip() if len(parts) > 1 and parts[1].strip() else watch)
        out.append((watch, src))
    return out


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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "ptp-dirwatch"})
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
    if action == "scan":
        return scan(cfg)
    if action == "reset_seen":
        kv_set_json("seen", {})
        return {"reset": True}
    raise ValueError(f"unknown action: {action}")


def is_stable(path, wait_ms):
    """文件稳定性检查：size 在 wait_ms 后不变视为写完（近似 debounce）。"""
    try:
        s1 = os.path.getsize(path)
    except OSError:
        return False
    time.sleep(max(wait_ms, 0) / 1000.0)
    try:
        return os.path.getsize(path) == s1
    except OSError:
        return False


def scan(cfg):
    exts = tuple(split_multi(cfg.get("media_exts"))) or (".mkv", ".mp4")
    dirs = parse_dirs(cfg.get("dirs"))
    if not dirs:
        return {"scanned": 0, "note": "未配置监视目录（插件设置里每行一条）"}
    wait_ms = 2000  # 与旧宿主 debounce 默认一致；轮询粒度由 cron 决定

    seen = kv_get_json("seen") or {}
    results, fed, errors = [], 0, 0
    for watch, src in dirs:
        if not os.path.isdir(watch):
            errors.append(f"{watch}: 目录不存在")
            continue
        seen_set = set(seen.get(watch) or [])
        media, current = [], set()
        for root, _dirs, names in os.walk(watch):
            for name in names:
                if not name.lower().endswith(exts):
                    continue
                full = os.path.join(root, name)
                current.add(full)
                if full in seen_set:
                    continue
                if not is_stable(full, wait_ms):
                    continue  # 仍在写入，下轮再看
                media.append({"path": full, "size": os.path.getsize(full)})
        if not media:
            seen[watch] = sorted(current)
            continue
        try:
            out = tool_call("transfer_discovered", {"src_dir": src, "files": media})
            fed += len(media)
            results.append({"dir": watch, "fed": len(media), "result": out})
            log("info", "dirwatch.fed", f"{watch}: fed {len(media)} file(s)", {"src_dir": src})
        except Exception as e:
            errors.append(f"{watch}: {e}")
            continue  # 喂失败不更新锚点，下轮重试
        seen_set = set(seen.get(watch) or [])
        seen_set.update(m["path"] for m in media)
        seen[watch] = sorted(seen_set | current)

    kv_set_json("seen", seen)
    summary = {"scanned": len(dirs), "fed": fed, "results": results, "errors": errors}
    if fed:
        kv_set_json("last_fed", {"ts": time.time(), "fed": fed})
    if errors:
        notice("目录监视", "扫描有失败：\n" + "\n".join(f"• {e}" for e in errors[:10]), "warning")
    if fed and as_bool(cfg.get("notify"), False):
        notice("目录监视", f"已投喂 {fed} 个媒体文件给整理流水线。", "info")
    log("info", "dirwatch.scan", f"scanned {len(dirs)} dir(s), fed {fed}", None)
    return summary


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
