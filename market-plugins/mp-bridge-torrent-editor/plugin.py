# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

# -*- coding: utf-8 -*-
# mp-bridge-torrent-editor — 批量做种规则编辑（独立实现，功能对标 MoviePilot 批量改做种类插件）。
# 能力全部经宿主 Tool RPC：downloaders.list / torrents_list / torrents_set_share_limits /
# torrents_set_tags / torrents_set_category。
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


def as_float(v, default=0.0):
    try:
        return float(v)
    except Exception:
        return default


def split_multi(raw):
    return [p.strip() for p in re.split(r"[\n,;]+", str(raw or "")) if p.strip()]


def _extract_items(out):
    if out is None:
        return []
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("results", "torrents", "items", "data", "templates"):
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
            self._json(200, {"ok": True, "plugin": PLUGIN_ID or "mp-bridge-torrent-editor"})
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
    if action == "select":
        return select_torrents(inp)
    if action == "set_limits":
        return set_limits(inp, cfg)
    if action == "set_tags":
        return set_tags(inp, cfg)
    if action == "save_template":
        return save_template(inp)
    if action == "list_templates":
        return list_templates()
    if action == "apply_template":
        return apply_template(inp, cfg)
    raise ValueError(f"unknown action: {action}")


def _match(t, cond):
    """单种子是否命中圈选条件（全部条件取交集；空条件=全选）。"""
    state = str(_pick(t, "state", "status")).lower()
    category = str(_pick(t, "category")).lower()
    tags = t.get("tags") if isinstance(t.get("tags"), list) else split_multi(_pick(t, "tags"))
    tags_l = {str(x).lower() for x in tags}
    name = str(_pick(t, "name")).lower()

    want_states = [s.lower() for s in split_multi(cond.get("states"))]
    if want_states and state not in want_states:
        return False
    want_cats = [c.lower() for c in split_multi(cond.get("categories"))]
    if want_cats and category not in want_cats:
        return False
    want_tags = [x.lower() for x in split_multi(cond.get("tags"))]
    if want_tags and not all(x in tags_l for x in want_tags):
        return False
    any_tags = [x.lower() for x in split_multi(cond.get("any_tags"))]
    if any_tags and not any(x in tags_l for x in any_tags):
        return False
    kw = str(cond.get("name_keyword") or "").strip().lower()
    if kw and kw not in name:
        return False
    return True


def select_torrents(cond):
    """跨下载器圈选；只读。"""
    dls = _extract_items(tool_call("downloaders.list"))
    picked, errors = [], []
    for dl in dls:
        if not isinstance(dl, dict):
            continue
        dl_id = _pick(dl, "id")
        if dl_id in ("", None):
            continue
        if cond.get("downloader_id") not in (None, "") and int(cond["downloader_id"]) != int(dl_id):
            continue
        try:
            torrents = _extract_items(tool_call("torrents_list", {"downloader_id": int(dl_id)}))
        except Exception as e:
            errors.append({"downloader": _pick(dl, "name"), "error": str(e)})
            continue
        for t in torrents:
            if isinstance(t, dict) and _match(t, cond):
                picked.append({"downloader_id": int(dl_id), "hash": _pick(t, "hash"),
                               "name": _pick(t, "name"), "state": _pick(t, "state"), "tags": t.get("tags")})
    return {"matched": len(picked), "torrents": picked[:200], "errors": errors}


def _hashes_from_selection(sel):
    by_dl = {}
    for t in sel:
        if t.get("hash") and t.get("downloader_id") not in (None, ""):
            by_dl.setdefault(int(t["downloader_id"]), []).append(t["hash"])
    if not by_dl:
        raise ValueError("圈选结果为空：先用 select 预览条件")
    return by_dl


def set_limits(inp, cfg):
    """批量改分享限制（ratio_limit/seeding_time_limit 必填，约定见工具描述）。"""
    if "ratio_limit" not in inp or "seeding_time_limit" not in inp:
        raise ValueError("ratio_limit 与 seeding_time_limit 必填（-2 无限 / -1 全局 / >=0 数值；时长单位分钟）")
    if inp.get("hashes"):
        by_dl = {int(inp.get("downloader_id") or 0): [str(h) for h in inp["hashes"]]}
    else:
        by_dl = _hashes_from_selection(inp.get("selection") or select_torrents(inp.get("criteria") or {}).get("torrents", []))
    updated = 0
    for dl_id, hashes in by_dl.items():
        out = tool_call("torrents_set_share_limits", {
            "downloader_id": dl_id, "hashes": hashes,
            "ratio_limit": as_float(inp["ratio_limit"]), "seeding_time_limit": as_int(inp["seeding_time_limit"]),
        })
        updated += as_int(_pick(out if isinstance(out, dict) else {}, "updated", "count"), len(hashes))
    log("info", "editor.limits", f"share limits set on {updated} torrent(s)", {"ratio": inp["ratio_limit"], "seed_min": inp["seeding_time_limit"]})
    if as_bool(cfg.get("notify"), True):
        notice("做种规则", f"已对 {updated} 个种子设置分享限制（ratio={inp['ratio_limit']}，做种 {inp['seeding_time_limit']} 分钟）。", "success")
    return {"updated": updated}


def set_tags(inp, cfg):
    """批量替换标签集（set 语义）。"""
    tags = inp.get("tags")
    if not isinstance(tags, list) or not tags:
        raise ValueError("tags 必填（字符串数组，set 语义整体替换）")
    if inp.get("hashes"):
        by_dl = {int(inp.get("downloader_id") or 0): [str(h) for h in inp["hashes"]]}
    else:
        by_dl = _hashes_from_selection(inp.get("selection") or select_torrents(inp.get("criteria") or {}).get("torrents", []))
    updated = 0
    for dl_id, hashes in by_dl.items():
        out = tool_call("torrents_set_tags", {"downloader_id": dl_id, "hashes": hashes, "tags": [str(x) for x in tags]})
        updated += as_int(_pick(out if isinstance(out, dict) else {}, "updated", "count"), len(hashes))
    log("info", "editor.tags", f"tags replaced on {updated} torrent(s)", {"tags": tags})
    if as_bool(cfg.get("notify"), True):
        notice("做种规则", f"已对 {updated} 个种子替换标签集：{'、'.join(str(x) for x in tags)}。", "success")
    return {"updated": updated}


def save_template(inp):
    name = str(_pick(inp, "name") or "").strip()
    if not name:
        raise ValueError("模板名必填")
    tpl = {"name": name, "criteria": inp.get("criteria") or {}, "action": inp.get("action") or {}}
    if not tpl["criteria"]:
        raise ValueError("criteria（圈选条件）必填")
    templates = kv_get_json("editor_templates") or {}
    templates[name] = tpl
    kv_set("editor_templates", json.dumps(templates, ensure_ascii=False))
    return {"saved": name, "total": len(templates)}


def list_templates():
    templates = kv_get_json("editor_templates") or {}
    return {"count": len(templates), "templates": sorted(templates.keys())}


def apply_template(inp, cfg):
    name = str(_pick(inp, "name") or _pick(cfg, "template_name") or "default")
    templates = kv_get_json("editor_templates") or {}
    tpl = templates.get(name)
    if not tpl:
        raise ValueError(f"模板 {name} 不存在（先 save_template）")
    act = tpl.get("action") or {}
    if act.get("tags") is not None:
        return set_tags({"criteria": tpl["criteria"], "tags": act["tags"]}, cfg)
    if "ratio_limit" in act or "seeding_time_limit" in act:
        return set_limits({"criteria": tpl["criteria"], **act}, cfg)
    sel = select_torrents(tpl.get("criteria") or {})
    return {"matched": sel["matched"], "note": "模板未定义动作（action.tags 或 action.limits），仅返回圈选"}


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(os.environ.get("PTP_PLUGIN_PORT", "19090"))), Handler).serve_forever()
