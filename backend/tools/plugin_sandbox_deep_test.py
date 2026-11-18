# -*- coding: utf-8 -*-
"""Deep sandbox test with local mock services for network-facing plugin actions.

Run from the repository root:
    python backend/tools/plugin_sandbox_deep_test.py

Exercises actions that the smoke suite skips (hosts write, Plex, group chat)
plus edge-case inputs. Writes tmp/plugin-phase2-results.json.
"""
import importlib.util, json, os, sys, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parent))
import plugin_sandbox_smoke as h

class Extra(BaseHTTPRequestHandler):
    def _j(self, code, obj, raw=None):
        data = (raw if raw is not None else json.dumps(obj).encode())
        self.send_response(code); self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data))); self.end_headers(); self.wfile.write(data)
    def do_GET(self):
        self.path = self.path.split("?")[0]
        if self.path == "/cf.txt":
            self._j(200, None, b"1.1.1.1 cdn.example.com\n2.2.2.2 api.example.com\n")
        elif self.path == "/identity":
            self._j(200, {"MediaContainer": {"machineIdentifier": "mock-1", "version": "1.40.0"}})
        elif self.path == "/library/sections":
            self._j(200, {"MediaContainer": {"Directory": [
                {"key": "1", "title": "Movies", "type": "movie"},
                {"key": "2", "title": "TV Shows", "type": "show"}]}})
        elif self.path.endswith("/refresh"):
            self._j(200, {"ok": True})
        else:
            self._j(200, {"ok": True, "path": self.path})
    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0") or "0")
        self.rfile.read(n)
        self._j(200, {"ok": True, "path": self.path})
    def log_message(self, *a): pass

srv = ThreadingHTTPServer(("127.0.0.1", 0), Extra)
threading.Thread(target=srv.serve_forever, daemon=True).start()
h.EXTRA_URL = "http://127.0.0.1:%d" % srv.server_address[1]

A = lambda name, inp=None, cfg=None: ("action", name, {"action": name, "input": inp or {}, "config": cfg or {}})
h.CASES = {
  "cloudflare-subscribe": [
      A("preview", {}, {"subscribe_urls": "{EXTRA_URL}/cf.txt", "marker": "PTPTEST"}),
      A("update_hosts", {}, {"subscribe_urls": "{EXTRA_URL}/cf.txt", "marker": "PTPTEST",
                             "hosts_path": "hosts-phase2.txt", "notify": False}),
  ],
  "ptp-plex-suite": [
      A("status", {}, {"base_url": "{EXTRA_URL}", "token": "mocktoken"}),
      A("libraries", {}, {"base_url": "{EXTRA_URL}", "token": "mocktoken"}),
      A("apply", {}, {"base_url": "{EXTRA_URL}", "token": "mocktoken", "refresh_on_apply": True, "notify": False}),
  ],
  "group-chat-zone": [
      A("run_main", {}, {"targets": "mock|{EXTRA_URL}/shout|k=v|hello", "notify": False}),
      A("run_zm", {}, {"zm_targets": "mock2|{EXTRA_URL}/shout|k=v|hi", "notify": False}),
  ],
  "json-toolbox": [
      A("get", {"json": "[1,2,3]", "path": "-1"}),
      A("get", {"json": '{"a":null}', "path": "a.b"}),
      A("validate", {"json": "[1,2,3]"}),
  ],
  "text-toolbox": [
      A("hash", {"text": "abc", "algo": "sha512"}),
      A("split_unique", {"text": "a,,b,a", "sep": ","}),
      A("stats", {"text": ""}),
  ],
  "twofa-helper": [
      A("list", {"accounts": "otpauth://totp/Test:alice?secret=JBSWY3DPEHPK3PXP&issuer=Test"}),
      A("generate", {"accounts": "otpauth://totp/Test:alice?secret=JBSWY3DPEHPK3PXP", "name": "alice"}),
  ],
  "cron-helper": [
      A("next_runs", {"expression": "*/15 9-17 * * 1-5", "count": 3}),
      A("next_runs", {"expression": "0 0 1 1 7", "count": 2}),
      A("next_runs", {"expression": "61 * * * *", "count": 1}),
  ],
  "keyword-alert": [
      A("test_match", {"payload": {"a": {"b": "ERROR"}}}, {"keywords": "error", "fields": "a.b", "case_sensitive": False}),
      A("test_match", {"payload": {"a": {"b": "ERROR"}}}, {"keywords": "error", "fields": "a.b", "case_sensitive": True}),
  ],
  "webhook-transformer": [
      A("preview", {"payload": {"items": [{"id": 1}, {"id": 2}]}}, {"mappings": "first=items.0.id\nsecond=items.1.id"}),
      A("preview", {"payload": {"x": 1}}, {"mappings": "bad=items.9.id"}),
  ],
  "http-probe": [
      A("probe", {"url": "{EXTRA_URL}/identity", "method": "POST", "headers": "X-Test: 1", "body": "hi", "expected_status": 200},
        {"allow_private_network": "true", "timeout_seconds": 5, "max_body_bytes": 200}),
      A("probe", {"url": "{EXTRA_URL}/identity", "expected_status": 404}, {"allow_private_network": "true", "timeout_seconds": 5}),
  ],
  "event-audit": [A("echo", {"message": "deep"}), E("plugin.test", {"nested": {"x": 1}}, {"include_payload": False})] if False else [
      A("echo", {"message": "deep"}),
      ("event", "plugin.test", {"type": "plugin.test", "data": {"nested": {"x": 1}}, "config": {"include_payload": False}}),
  ],
}

results = []
for pid in sorted(h.CASES):
    try: r = h.run_plugin(pid)
    except Exception as exc: r = {"plugin": pid, "health": "HARNESS_ERROR", "cases": [], "stderr": str(exc)}
    results.append(r)
    print("[%s] health=%s" % (pid, r["health"]), flush=True)
Path("tmp/plugin-phase2-results.json").write_text(json.dumps(results, ensure_ascii=False, indent=2), "utf-8")
srv.shutdown(); srv.server_close()
# 清理 phase2 写入的测试文件
for name in ("hosts-phase2.txt", "hosts-phase2.txt.ptp-bak"):
    p = Path("market-plugins/cloudflare-subscribe") / name
    if p.exists(): p.unlink()
print("DONE phase2")
