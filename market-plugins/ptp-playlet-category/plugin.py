"""短剧自动分类 — 用 ffprobe 探测子目录里视频时长，短剧目录整体移动到分类目录。"""
import json
import os
import random
import shutil
import subprocess
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")
HOST_URL = os.environ.get("PTP_HOST_URL", "")
HOST_TOKEN = os.environ.get("PTP_HOST_TOKEN", "")
DEFAULT_EXT = ".mp4,.mkv,.avi,.ts,.mov,.wmv,.flv,.m4v,.rmvb,.mpg,.mpeg,.webm"


def host_call(path, body):
    if not HOST_URL or not HOST_TOKEN:
        return None
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        HOST_URL.rstrip("/") + path, data=data,
        headers={"Authorization": f"Bearer {HOST_TOKEN}", "Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as res:
            raw = res.read().decode("utf-8")
            return json.loads(raw) if raw else {}
    except Exception:
        return None


def log(level, event, message, data=None):
    host_call("/log", {"level": level, "event": event, "message": message, "data": data or {}})


def notice(title, body, level="info"):
    host_call("/notice", {"title": title, "body": body, "level": level})


def parse_exts(value):
    parts = [p.strip().lower() for p in str(value or DEFAULT_EXT).split(",") if p.strip()]
    return [p if p.startswith(".") else f".{p}" for p in parts]


def to_int(value, default):
    try:
        return int(float(str(value)))
    except Exception:
        return default


def probe_duration(video):
    cmd = ["ffprobe", "-v", "error", "-show_entries", "format=duration",
           "-of", "default=noprint_wrappers=1:nokey=1", str(video)]
    proc = subprocess.run(cmd, capture_output=True, text=True)
    if proc.returncode != 0:
        raise RuntimeError(f"ffprobe exit {proc.returncode}")
    return float(proc.stdout.strip() or 0.0)


def list_media(folder, exts):
    found = []
    for dirpath, _dirs, files in os.walk(folder):
        for name in files:
            if Path(name).suffix.lower() in exts:
                found.append(Path(dirpath) / name)
    return found


def is_short_drama(folder, exts, sample_count, max_seconds):
    media = list_media(folder, exts)
    if not media:
        return None  # 不是视频目录
    sample = media if len(media) <= sample_count else random.sample(media, sample_count)
    durations = []
    for video in sample:
        try:
            durations.append(probe_duration(video))
        except Exception as exc:
            log("warning", "playlet.probe.error", str(exc), {"file": str(video)})
    if not durations:
        return False
    return all(d <= max_seconds for d in durations)


def do_scan(cfg):
    source = str(cfg.get("source_path") or "").strip()
    category_dir = str(cfg.get("category_dir") or "短剧").strip() or "短剧"
    max_minutes = to_int(cfg.get("episode_duration"), 8)
    sample_count = max(1, to_int(cfg.get("sample_count"), 3))
    exts = parse_exts(cfg.get("media_ext"))
    if not source or not os.path.isdir(source):
        raise ValueError("source_path 未配置或不存在")

    dest_root = os.path.join(source, category_dir)
    scanned = moved = skipped = non_short = 0
    errors = []
    for name in sorted(os.listdir(source)):
        child = os.path.join(source, name)
        if not os.path.isdir(child) or name == category_dir:
            continue
        scanned += 1
        try:
            verdict = is_short_drama(child, exts, sample_count, max_minutes * 60)
        except Exception as exc:
            errors.append(f"{name}: {exc}")
            _worker_status.update({"scanned": scanned, "moved": moved, "errors": len(errors)})
            continue
        if verdict is None:
            skipped += 1
            continue
        if not verdict:
            non_short += 1
            continue
        target = os.path.join(dest_root, name)
        os.makedirs(dest_root, exist_ok=True)
        if os.path.exists(target):
            errors.append(f"{name}: 目标已存在 {target}")
            continue
        shutil.move(child, target)
        moved += 1
        log("info", "playlet.moved", name, {"from": child, "to": target})
        _worker_status.update({"scanned": scanned, "moved": moved, "errors": len(errors)})
    return {"scanned": scanned, "moved": moved, "not_short": non_short,
            "skipped": skipped, "errors": errors}


_worker_lock = threading.Lock()
_worker_thread = None
_worker_status = {"running": False, "done": False, "scanned": 0, "moved": 0, "errors": 0}


def _scan_worker(cfg):
    global _worker_thread
    _worker_status.update({"running": True, "done": False, "started_at": time.strftime("%Y-%m-%d %H:%M:%S"),
                           "scanned": 0, "moved": 0, "errors": 0, "error": None})
    try:
        result = do_scan(cfg)
        _worker_status.update({"running": False, "done": True,
                               "finished_at": time.strftime("%Y-%m-%d %H:%M:%S")})
        notice("短剧自动分类", f"扫描 {result['scanned']} 个目录，移动 {result['moved']} 个短剧目录。", "info")
    except Exception as exc:
        _worker_status.update({"running": False, "done": True, "error": str(exc)})
        log("error", "playlet.scan.fatal", str(exc))
        notice("短剧自动分类", f"扫描失败：{exc}", "warning")
    finally:
        with _worker_lock:
            _worker_thread = None


def start_scan(cfg):
    global _worker_thread
    with _worker_lock:
        if _worker_thread and _worker_thread.is_alive():
            return {"started": False, "already_running": True, "status": dict(_worker_status)}
        _worker_thread = threading.Thread(target=_scan_worker, args=(cfg,), daemon=True)
        _worker_thread.start()
        return {"started": True, "status": dict(_worker_status)}


def handle_action(action, inp, cfg, host):
    if action == "scan":
        return start_scan(cfg)
    if action == "status":
        return dict(_worker_status)
    raise ValueError(f"unknown action: {action}")


def handle_event(event_type, data, cfg):
    return None


class Handler(BaseHTTPRequestHandler):
    def _json(self, status, payload):
        raw = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _body(self):
        length = int(self.headers.get("Content-Length", "0") or "0")
        return json.loads(self.rfile.read(length).decode("utf-8")) if length else {}

    def _auth(self):
        return not PLUGIN_TOKEN or self.headers.get("Authorization") == f"Bearer {PLUGIN_TOKEN}"

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"ok": True})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        if not self._auth():
            self._json(401, {"error": "unauthorized"})
            return
        payload = self._body()
        if self.path == "/action":
            try:
                output = handle_action(payload.get("action"), payload.get("input") or {},
                                        payload.get("config") or {}, payload.get("host") or {})
                self._json(200, {"ok": True, "output": output})
            except Exception as exc:
                log("error", "action.error", str(exc))
                self._json(200, {"ok": False, "error": str(exc)})
            return
        if self.path == "/event":
            self._json(200, {"ok": True})
            return
        self._json(404, {"error": "not found"})

    def log_message(self, *args):
        return


if __name__ == "__main__":
    port = int(os.environ.get("PTP_PLUGIN_PORT", "19090"))
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
