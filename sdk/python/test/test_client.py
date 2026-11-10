# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

import json
import os
import threading
import unittest
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from ptpatronus_plugin_sdk import PTPHostClient, HostError, KVResult
from ptpatronus_plugin_sdk.types import HostContext


def _start_fake_host(token):
    """A minimal Host API server for testing: /log /kv/ /sites /media/search /health."""
    state = {}

    class H(BaseHTTPRequestHandler):
        def _json(self, status, payload):
            raw = json.dumps(payload).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def _authed(self):
            return self.headers.get("Authorization") == f"Bearer {token}"

        def _body(self):
            n = int(self.headers.get("Content-Length", "0") or "0")
            return json.loads(self.rfile.read(n).decode()) if n else {}

        def do_GET(self):
            if self.path == "/health":
                self._json(200, {"ok": True}); return
            if not self._authed():
                self._json(401, {"error": "unauthorized"}); return
            if self.path == "/capabilities":
                self._json(200, {"api_version": "1", "plugin_id": "x", "permissions": ["kv:read"], "host_capabilities": [], "endpoints": []})
            elif self.path.startswith("/kv/"):
                k = urllib.parse.unquote(self.path[len("/kv/"):])
                self._json(200, {"found": k in state, "value": state.get(k)})
            elif self.path == "/sites":
                self._json(200, {"sites": [{"id": 1, "name": "demo", "base_url": "https://demo", "username": "u"}]})
            elif self.path == "/tools":
                self._json(200, {"tools": [{"name": "ping", "permission": "x:read", "category": "test"}]})
            else:
                self._json(404, {"error": "not found"})

        def do_POST(self):
            if not self._authed():
                self._json(401, {"error": "unauthorized"}); return
            b = self._body()
            if self.path == "/log":
                self._json(200, {"ok": True})
            elif self.path == "/media/search":
                self._json(200, {"results": [{"title": b.get("keyword"), "year": b.get("year")}]})
            elif self.path.startswith("/tools/"):
                self._json(200, {"tool": self.path[len("/tools/"):], "got": b.get("input")})
            else:
                self._json(404, {"error": "not found"})

        def do_PUT(self):
            if not self._authed():
                self._json(401, {"error": "unauthorized"}); return
            if self.path.startswith("/kv/"):
                k = urllib.parse.unquote(self.path[len("/kv/"):])
                state[k] = self._body().get("value")
                self._json(200, {"ok": True})

        def log_message(self, *a):
            pass

    srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    return srv, f"http://127.0.0.1:{srv.server_port}"


class HostClientTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.token = "test-token"
        cls.srv, cls.base = _start_fake_host(cls.token)
        cls.client = PTPHostClient(HostContext(base_url=cls.base, token=cls.token, permissions=["kv:read", "kv:write"]))

    @classmethod
    def tearDownClass(cls):
        cls.srv.shutdown(); cls.srv.server_close()

    def test_from_env_missing(self):
        self.assertIsNone(PTPHostClient.from_env({}))

    def test_from_env_present(self):
        c = PTPHostClient.from_env({
            "PTP_HOST_URL": self.base,
            "PTP_HOST_TOKEN": self.token,
            "PTP_PLUGIN_ID": "p",
            "PTP_HOST_PERMISSIONS": "kv:read, downloader:*",
            "PTP_HOST_CAPABILITIES": "host.api.kv.read, host.api.tools",
        })
        self.assertIsNotNone(c)
        self.assertEqual(c.context.plugin_id, "p")
        self.assertTrue(c.has_permission("kv:read"))
        self.assertTrue(c.has_permission("downloader:write"))
        self.assertTrue(c.has_capability("host.api.tools"))

    def test_requires_base_and_token(self):
        with self.assertRaises(ValueError):
            PTPHostClient(HostContext(base_url="", token=""))

    def test_has_permission_scoped(self):
        self.assertTrue(self.client.has_permission("kv:read"))
        self.assertTrue(self.client.has_permission("kv:write"))  # exact
        self.assertFalse(self.client.has_permission("site:cookie"))

    def test_log_and_kv(self):
        self.client.log("info", "evt", "hello")
        self.assertIsNone(self.client.kv_set("k", {"n": 1}))
        r = self.client.kv_get("k")
        self.assertIsInstance(r, KVResult)
        self.assertTrue(r.found)
        self.assertEqual(r.value, {"n": 1})
        miss = self.client.kv_get("missing")
        self.assertFalse(miss.found)

    def test_sites(self):
        sites = self.client.sites()
        self.assertEqual(len(sites), 1)
        self.assertEqual(sites[0].name, "demo")

    def test_media_search(self):
        res = self.client.media_search("drama", 2024)
        self.assertEqual(res[0]["title"], "drama")
        self.assertEqual(res[0]["year"], 2024)

    def test_tools_list(self):
        tools = self.client.tools()
        self.assertTrue(any(t["name"] == "ping" for t in tools))

    def test_call_with_input(self):
        r = self.client.call("ping", {"v": 9})
        self.assertEqual(r["tool"], "ping")
        self.assertEqual(r["got"], {"v": 9})

    def test_call_no_input(self):
        r = self.client.call("system.info")
        self.assertEqual(r["tool"], "system.info")
        self.assertEqual(r["got"], {})  # input 缺省时 call() 传 {}

    def test_convenience_method_routes_to_call(self):
        # 便捷方法本质是 call；fake host 的 /tools/ echo 回显 input。
        r = self.client.torrents_list(3)
        self.assertEqual(r["tool"], "torrents_list")
        self.assertEqual(r["got"], {"downloader_id": 3})

    def test_host_error_on_unknown(self):
        with self.assertRaises(HostError):
            self.client.site_cookie(999)  # 404 -> HostError

    def test_unauthorized_raises(self):
        bad = PTPHostClient(HostContext(base_url=self.base, token="wrong"))
        with self.assertRaises(HostError):
            bad.log("info", "e", "m")


if __name__ == "__main__":
    unittest.main()
