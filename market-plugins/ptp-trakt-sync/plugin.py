# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# ptp-trakt-sync — Trakt.tv watchlist → 宿主订阅（打样「核心功能出墙」，见 issue #34）。
# Device-code OAuth（无需回调地址）；条目经 Trakt Search 映射 TMDB ID 后建订阅（source=tmdb）。
import json, os, time, urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PLUGIN_TOKEN = os.environ.get("PTP_PLUGIN_TOKEN", "")
HOST_URL = os.environ.get("PTP_HOST_URL", "")
HOST_TOKEN = os.environ.get("PTP_HOST_TOKEN", "")
PLUGIN_ID = os.environ.get("PTP_PLUGIN_ID", "")

TRAKT_API = "https://api.trakt.tv"
TRAKT_SITE = "https://trakt.tv"


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


def kv_set_raw(key, value):
    host_call(f"/kv/{key}", {"value": value}, method="PUT")


def kv_get_raw(key):
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


def _pick(d, *keys):
    if not isinstance(d, dict):
        return ""
    for k in keys:
        if d.get(k) not in (None, ""):
            return d.get(k)
    return ""


def _extract_items(out):
    if out is None:
        return []
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("results", "items", "data", "subscriptions"):
            v = out.get(k)
            if isinstance(v, list):
                return v
        return [out]
    return []


def trakt_request(method, path, token=None, body=None, extra_headers=None):
    """直连 Trakt API（插件进程内出网；trakt.tv 是公网第三方，不经宿主代理）。"""
    import urllib.request
    headers = {
        "Content-Type": "application/json",
        "trakt-api-version": "2",
        "trakt-api-key": "",  # 由调用方填 client_id
    }
    if extra_headers:
        headers.update(extra_headers)
    if token:
        headers["Authorization"] = f"Bearer {token}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(TRAKT_API + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=30) as res:
        raw = res.read().decode("utf-8")
        return json.loads(raw) if raw.strip().startswith(("{", "[")) else {}


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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "ptp-trakt-sync"})
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
    if action == "auth_start":
        return auth_start(cfg)
    if action == "auth_check":
        return auth_check(cfg)
    if action == "auth_status":
        return auth_status()
    if action == "sync":
        return sync(cfg)
    if action == "status":
        return {"last_sync": kv_get_raw("last_sync")}
    if action == "unlink":
        kv_set_raw("trakt_token", ""); kv_set_raw("last_sync", "")
        return {"unlinked": True}
    raise ValueError(f"unknown action: {action}")


# ===== 授权（device code 流程）=====

def auth_start(cfg):
    cid = str(_pick(cfg, "client_id"))
    if not cid:
        raise ValueError("先在插件设置里填 Trakt Client ID（trakt.tv/settings/applications 新建应用）")
    out = trakt_request("POST", "/oauth/device/code", extra_headers={"trakt-api-key": cid},
                        body={"client_id": cid})
    kv_set_raw("trakt_device", {"device_code": out.get("device_code"), "interval": out.get("interval", 5),
                                "expires_in": out.get("expires_in", 600), "ts": time.time()})
    return {
        "user_code": out.get("user_code"),
        "verification_url": out.get("verification_url", f"{TRAKT_SITE}/activate"),
        "expires_in": out.get("expires_in", 600),
        "next": "用户在浏览器打开 verification_url 输入 user_code 后，执行 auth_check 完成授权。",
    }


def auth_check(cfg):
    cid = str(_pick(cfg, "client_id"))
    secret = str(_pick(cfg, "client_secret"))
    dev = kv_get_raw("trakt_device") or {}
    device_code = dev.get("device_code")
    if not (cid and secret and device_code):
        raise ValueError("缺 client_id/client_secret 或尚未 auth_start")
    import urllib.request
    data = urllib.parse.urlencode({
        "code": device_code, "client_id": cid, "client_secret": secret,
    }).encode()
    req = urllib.request.Request(f"{TRAKT_API}/oauth/device/token", data=data,
                                 headers={"Content-Type": "application/x-www-form-urlencoded"},
                                 method="POST")
    try:
        with urllib.request.urlopen(req, timeout=30) as res:
            tok = json.loads(res.read().decode())
    except Exception as e:
        code = getattr(e, "code", None)
        if code == 400:
            return {"pending": True, "reason": "用户尚未完成浏览器输入（可稍后再试）"}
        if code == 410:
            return {"expired": True, "reason": "device code 已过期，请重新 auth_start"}
        raise
    tok["obtained_at"] = time.time()
    kv_set_raw("trakt_token", tok)
    kv_set_raw("trakt_device", "")
    return {"authorized": True, "expires_in": tok.get("expires_in"),
            "account": _pick(tok, "account") or None}


def _access_token(cfg):
    tok = kv_get_raw("trakt_token") or {}
    access = tok.get("access_token")
    if not access:
        raise ValueError("尚未授权：先 auth_start → 浏览器输入 → auth_check")
    # 过期前 60s 用 refresh token 续期（刷新即存）。
    expires_in = as_int(tok.get("expires_in"), 7776000)
    if time.time() - as_float(tok.get("obtained_at"), 0) > expires_in - 60:
        cid = str(_pick(cfg, "client_id"))
        secret = str(_pick(cfg, "client_secret"))
        if cid and secret and tok.get("refresh_token"):
            import urllib.request
            data = urllib.parse.urlencode({
                "refresh_token": tok["refresh_token"], "client_id": cid,
                "client_secret": secret, "grant_type": "refresh_token",
            }).encode()
            req = urllib.request.Request(f"{TRAKT_API}/oauth/token", data=data,
                                         headers={"Content-Type": "application/x-www-form-urlencoded"},
                                         method="POST")
            with urllib.request.urlopen(req, timeout=30) as res:
                tok = json.loads(res.read().decode())
            tok["obtained_at"] = time.time()
            kv_set_raw("trakt_token", tok)
            access = tok.get("access_token")
    return access


def as_float(v, default=0.0):
    try:
        return float(v)
    except Exception:
        return default


def auth_status():
    tok = kv_get_raw("trakt_token") or {}
    if not tok.get("access_token"):
        return {"authorized": False}
    left = as_int(tok.get("expires_in"), 0) - int(time.time() - as_float(tok.get("obtained_at"), 0))
    return {"authorized": True, "expires_in": left, "expired": left <= 0}


# ===== 同步 =====

def _trakt_ids(entry):
    """watchlist 条目 → (trakt_type, ids dict, title, year)。"""
    t = entry.get("type")  # movie | show
    obj = entry.get(t) if isinstance(entry.get(t), dict) else {}
    ids = obj.get("ids") or {}
    return t, ids, obj.get("title"), obj.get("year")


def map_to_tmdb(ids, trakt_type, cid, access):
    """Trakt ID → TMDB ID（经 Trakt Search）。映射失败返回 None。"""
    if ids.get("tmdb"):
        return str(ids["tmdb"])
    slug = ids.get("slug") or ids.get("trakt")
    if not slug:
        return None
    ext = "movie" if trakt_type == "movie" else "show"
    out = trakt_request("GET", f"/search/{ext}/{urllib.parse.quote(str(slug))}?field=slug",
                        extra_headers={"trakt-api-key": cid},
                        token=access)
    for hit in out or []:
        if hit.get(ext) and (hit[ext].get("ids") or {}).get("tmdb"):
            return str(hit[ext]["ids"]["tmdb"])
    return None


def sync(cfg):
    cid = str(_pick(cfg, "client_id"))
    access = _access_token(cfg)
    want_movies = as_bool(cfg.get("sync_movies"), True)
    want_shows = as_bool(cfg.get("sync_shows"), True)
    resolution = str(_pick(cfg, "resolution"))

    wl = trakt_request("GET", "/users/me/watchlist", extra_headers={"trakt-api-key": cid}, token=access)
    # 增量锚点：watchlist 列表级 last_updated_at（无则全量）。
    anchors = wl if isinstance(wl, dict) else {}
    last_anchor = kv_get_raw("watchlist_anchor") or ""
    cur_anchor = str(_pick(anchors, "last_updated_at"))

    subs = _extract_items(tool_call("subscribe_list"))
    existing = {(str(_pick(s, "source")), str(_pick(s, "source_id"))) for s in subs if isinstance(s, dict)}

    created, skipped_exist, failed, unmapped = [], [], [], []
    for entry in wl if isinstance(wl, list) else []:
        if not isinstance(entry, dict):
            continue
        ttype, ids, title, year = _trakt_ids(entry)
        if ttype == "movie" and not want_movies:
            continue
        if ttype == "show" and not want_shows:
            continue
        if not title:
            continue
        tmdb_id = map_to_tmdb(ids, ttype, cid, access)
        if not tmdb_id:
            unmapped.append(f"{title} ({year})")
            continue
        if ("tmdb", tmdb_id) in existing:
            skipped_exist.append(title)
            continue
        try:
            out = tool_call("subscribe_create", {
                "title": title, "type": "movie" if ttype == "movie" else "tv",
                "source": "tmdb", "source_id": tmdb_id, "year": as_int(year, 0) or None,
                "resolution": resolution,
            })
            created.append(title)
            log("info", "trakt.created", f"subscribed {title} (tmdb:{tmdb_id})", None)
        except Exception as e:
            failed.append(f"{title}: {e}")

    summary = {"watchlist": len(wl) if isinstance(wl, list) else 0, "created": created,
               "skipped_existing": len(skipped_exist), "failed": failed, "unmapped": unmapped}
    if cur_anchor:
        kv_set_raw("watchlist_anchor", cur_anchor)
    kv_set_raw("last_sync", {"ts": time.time(), "created": len(created),
                             "skipped": len(skipped_exist), "failed": len(failed), "unmapped": len(unmapped)})
    log("info", "trakt.sync", f"watchlist sync: +{len(created)} skip={len(skipped_exist)} fail={len(failed)} unmapped={len(unmapped)}", None)
    if as_bool(cfg.get("notify"), True) and created:
        notice("Trakt 同步", f"watchlist 新增订阅 {len(created)}：\n" + "\n".join(f"• {t}" for t in created[:20]),
               "success" if not failed else "warning")
    return summary


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
