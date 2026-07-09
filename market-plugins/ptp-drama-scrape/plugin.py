"""短剧刮削 — 按目录名到寸光集（ptang.top）检索元数据，写 tvshow.nfo + poster.jpg。"""
import json
import os
import re
import shutil
import threading
import time
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")
HOST_URL = os.environ.get("PTP_HOST_URL", "")
HOST_TOKEN = os.environ.get("PTP_HOST_TOKEN", "")
DEFAULT_EXT = ".mp4,.mkv,.avi,.ts,.mov,.wmv,.flv,.m4v,.rmvb,.mpg,.mpeg,.webm"
UA = "PTPatronusPlugin/1.0 (drama-scrape)"


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


# ---------------- 寸光集 client (mirrors backend/internal/media/xingguangji.go) ----------------

def _fetch_bytes_with_deadline(url, headers, deadline):
    """urlopen(timeout=) only guards per-recv; this enforces a hard total deadline so a
    slow-drip/throttled response can never wedge the worker."""
    box = {}

    def _worker():
        try:
            req = urllib.request.Request(url, headers=headers)
            with urllib.request.urlopen(req, timeout=deadline) as res:
                box["data"] = res.read()
        except Exception as exc:  # noqa: BLE001 - propagate any network error
            box["error"] = exc

    t = threading.Thread(target=_worker, daemon=True)
    t.start()
    t.join(deadline)
    if t.is_alive():
        raise TimeoutError(f"request exceeded {deadline}s: {url[:80]}")
    if "error" in box:
        raise box["error"]
    return box.get("data") or b""


def _fetch_text_with_deadline(url, headers, deadline):
    return _fetch_bytes_with_deadline(url, headers, deadline).decode("utf-8", "replace")


def xg_get(base, key, path, params):
    url = base.rstrip("/") + path
    if params:
        url = url + "?" + urllib.parse.urlencode(params)
    headers = {"Accept": "application/json", "User-Agent": UA}
    if key:
        headers["Authorization"] = f"Bearer {key}"
    return json.loads(_fetch_text_with_deadline(url, headers, 20) or "{}")


def xg_search(base, key, name):
    r = xg_get(base, key, "/api/explore", {"q": name, "page": "1", "pageSize": "10"})
    return r.get("items") or []


def xg_detail(base, key, item_id):
    return xg_get(base, key, f"/api/explore/{item_id}", {})


def resolve_url(base, u):
    if not u:
        return ""
    if u.startswith("http://") or u.startswith("https://"):
        return u
    if u.startswith("/"):
        return base.rstrip("/") + u
    return u


# ---------------- name cleaning + matching ----------------

BRACKET_RE = re.compile(r"【.*?】|\[.*?\]|\(.*?\)|（.*?）|「.*?」|【.*?】")
NOISE_RE = re.compile(r"(S\d{1,2}E?\d{0,3}|[Ee][Pp]?\d{1,3}|\d{3,4}[pP]|[48][Kk]|HEVC|H\.?26[45]|h264|AVC|10bit|GB|fps|X264|[Xx]26[45]|AAC|FLAC|WEB-DL|BluRay|BDRip|mp4|mkv)", re.I)
SEP_RE = re.compile(r"[._]+")


def clean_name(raw):
    s = BRACKET_RE.sub(" ", str(raw or ""))
    s = NOISE_RE.sub(" ", s)
    s = SEP_RE.sub(" ", s)
    s = re.sub(r"\s+", " ", s).strip(" -—")
    return s


def pick_match(name, items):
    if not items:
        return None
    target = name.lower()
    best, best_score = items[0], -1
    for it in items:
        title = str(it.get("title") or "").lower()
        alt = str(it.get("altTitle") or "").lower()
        score = -1
        if target and (target == title or target == alt):
            score = 100
        elif target and (target in title or target in alt or title in target):
            score = 50
        elif title:
            score = 10
        if score > best_score:
            best, best_score = it, score
    return best


# ---------------- NFO + image ----------------

def write_tvshow_nfo(dest, detail):
    root = ET.Element("tvshow")
    ET.SubElement(root, "title").text = str(detail.get("title") or "")
    if detail.get("altTitle"):
        ET.SubElement(root, "originaltitle").text = str(detail.get("altTitle"))
    ET.SubElement(root, "sorttitle").text = str(detail.get("title") or "")
    if detail.get("description"):
        ET.SubElement(root, "plot").text = str(detail.get("description"))
    if detail.get("releaseYear"):
        ET.SubElement(root, "year").text = str(detail.get("releaseYear"))
    for tag in str(detail.get("tags") or "").replace("，", ",").split(","):
        tag = tag.strip()
        if tag:
            ET.SubElement(root, "genre").text = tag
    if detail.get("region"):
        ET.SubElement(root, "country").text = str(detail.get("region"))
    ET.SubElement(root, "studio").text = "寸光集"
    rating = float(detail.get("rating") or 0)
    if rating > 0:
        ratings = ET.SubElement(root, "ratings")
        r = ET.SubElement(ratings, "rating", {"name": "cunguangji", "max": "10", "default": "true"})
        ET.SubElement(r, "value").text = str(rating)
        ET.SubElement(r, "votes").text = "0"
    uid = ET.SubElement(root, "uniqueid", {"type": "cunguangji", "default": "true"})
    uid.text = str(detail.get("id") or "")
    for order, cast in enumerate(detail.get("casts") or []):
        name = str(cast.get("name") or "").strip()
        if not name:
            continue
        actor = ET.SubElement(root, "actor")
        ET.SubElement(actor, "name").text = name
        if cast.get("role"):
            ET.SubElement(actor, "role").text = str(cast.get("role"))
        ET.SubElement(actor, "order").text = str(order)
    ET.SubElement(root, "season").text = "1"
    if detail.get("totalEpisodes"):
        ET.SubElement(root, "episode").text = str(detail.get("totalEpisodes"))
    try:
        ET.indent(root, space="  ")
    except Exception:
        pass
    ET.ElementTree(root).write(os.path.join(dest, "tvshow.nfo"), encoding="utf-8", xml_declaration=True)


def download_image(url, dest):
    data = _fetch_bytes_with_deadline(url, {"User-Agent": UA}, 30)
    with open(dest, "wb") as f:
        f.write(data)
    return len(data)


def transfer_file(src, dst, mode):
    if mode == "link":
        try:
            os.link(src, dst)
            return
        except OSError:
            shutil.copy2(src, dst)
    elif mode == "copy":
        shutil.copy2(src, dst)
    elif mode == "move":
        shutil.move(src, dst)


# ---------------- scan ----------------

def parse_exts(value):
    parts = [p.strip().lower() for p in str(value or DEFAULT_EXT).split(",") if p.strip()]
    return [p if p.startswith(".") else f".{p}" for p in parts]


def list_media(folder, exts):
    out = []
    for dirpath, _dirs, files in os.walk(folder):
        for name in files:
            if Path(name).suffix.lower() in exts:
                out.append(Path(dirpath) / name)
    return out


def scrape_folder(folder, cfg, exts):
    base = str(cfg.get("cunguangji_base") or "https://ptang.top").strip() or "https://ptang.top"
    key = str(cfg.get("cunguangji_key") or "").strip()
    target_dir = str(cfg.get("target_dir") or "").strip()
    mode = str(cfg.get("transfer_type") or "none")
    overwrite = bool(cfg.get("overwrite"))
    name = clean_name(Path(folder).name)
    if not name:
        return {"status": "skip", "reason": "empty name"}

    items = xg_search(base, key, name)
    match = pick_match(name, items)
    if not match:
        return {"status": "miss", "name": name}
    detail = xg_detail(base, key, match.get("id"))
    if not detail or not detail.get("title"):
        detail = match

    dest = os.path.join(target_dir, name) if target_dir else str(folder)
    os.makedirs(dest, exist_ok=True)
    nfo_path = os.path.join(dest, "tvshow.nfo")
    if os.path.exists(nfo_path) and not overwrite and not target_dir:
        return {"status": "exists", "name": name}

    write_tvshow_nfo(dest, detail)
    result = {"status": "ok", "name": name, "title": detail.get("title"),
              "id": detail.get("id"), "nfo": nfo_path, "poster": None}
    cover = resolve_url(base, detail.get("cover") or match.get("cover"))
    if cover:
        try:
            poster_path = os.path.join(dest, "poster.jpg")
            download_image(cover, poster_path)
            result["poster"] = poster_path
        except Exception as exc:
            log("warning", "drama.poster.error", str(exc), {"name": name, "cover": cover})

    if target_dir and mode != "none":
        for video in list_media(folder, exts):
            transfer_file(str(video), os.path.join(dest, video.name), mode)
    return result


# ---------------- async worker (host /action has a 30s cap; batch scans run here) ----------------

_worker_lock = threading.Lock()
_worker_thread = None
_worker_status = {"running": False, "done": False, "ok": 0, "miss": 0, "errors": 0}


def do_scan(cfg):
    roots = [r.strip() for r in str(cfg.get("source_dirs") or "").splitlines() if r.strip()]
    excludes = [e.strip() for e in str(cfg.get("exclude_keywords") or "").splitlines() if e.strip()]
    exts = parse_exts(cfg.get("media_ext")) if cfg.get("media_ext") else parse_exts(DEFAULT_EXT)
    if not roots:
        raise ValueError("source_dirs 未配置")

    ok = miss = exists = skipped = 0
    errors = []
    for root in roots:
        if not os.path.isdir(root):
            errors.append(f"not a dir: {root}")
            continue
        for name in sorted(os.listdir(root)):
            child = os.path.join(root, name)
            if not os.path.isdir(child):
                continue
            if any(ex and ex in name for ex in excludes):
                skipped += 1
                continue
            if not list_media(child, exts):
                continue
            try:
                r = scrape_folder(child, cfg, exts)
                status = r.get("status")
                if status == "ok":
                    ok += 1
                elif status == "miss":
                    miss += 1
                elif status == "exists":
                    exists += 1
                else:
                    skipped += 1
            except Exception as exc:
                errors.append(f"{name}: {exc}")
                log("error", "drama.scrape.error", str(exc), {"dir": name})
            _worker_status.update({"ok": ok, "miss": miss, "exists": exists,
                                   "skipped": skipped, "errors": len(errors)})
    return {"ok": ok, "miss": miss, "exists": exists, "skipped": skipped, "errors": errors}


def _scan_worker(cfg):
    global _worker_thread
    started = time.strftime("%Y-%m-%d %H:%M:%S")
    _worker_status.update({"running": True, "done": False, "started_at": started,
                           "ok": 0, "miss": 0, "exists": 0, "skipped": 0, "errors": 0, "error": None})
    try:
        result = do_scan(cfg)
        _worker_status.update({"running": False, "done": True, "finished_at": time.strftime("%Y-%m-%d %H:%M:%S")})
        notice("短剧刮削", f"完成：成功 {result['ok']}，未匹配 {result['miss']}，已存在 {result['exists']}，失败 {len(result['errors'])}。", "info")
    except Exception as exc:
        _worker_status.update({"running": False, "done": True, "error": str(exc)})
        log("error", "drama.scan.fatal", str(exc))
        notice("短剧刮削", f"扫描失败：{exc}", "warning")
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
    base = str(cfg.get("cunguangji_base") or "https://ptang.top").strip() or "https://ptang.top"
    key = str(cfg.get("cunguangji_key") or "").strip()
    if action == "scrape_one":
        name = clean_name(inp.get("name") or "")
        if not name:
            raise ValueError("input.name is required")
        items = xg_search(base, key, name)
        return {"name": name, "results": [
            {"id": it.get("id"), "title": it.get("title"), "altTitle": it.get("altTitle"),
             "year": it.get("releaseYear"), "rating": it.get("rating"),
             "cover": resolve_url(base, it.get("cover"))}
            for it in items[:10]
        ]}
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
