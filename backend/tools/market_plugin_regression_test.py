# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors
"""Regression tests for defects found during deep market-plugin testing.

Run: python -m unittest discover -s backend/tools -p '*regression*test.py' -v
"""
from __future__ import annotations

import importlib.util
import json
import os
import shlex
import sys
import tempfile
import unittest
import urllib.request
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
PLUGINS = ROOT / "market-plugins"


def load_plugin_module(name: str, plugin_id: str):
    path = PLUGINS / plugin_id / "plugin.py"
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class ManifestTests(unittest.TestCase):
    def test_all_manifests_are_consistent(self) -> None:
        for plugin_dir in sorted(p for p in PLUGINS.iterdir() if p.is_dir()):
            manifest_path = plugin_dir / "plugin.json"
            if not manifest_path.exists():
                continue
            data = json.loads(manifest_path.read_text(encoding="utf-8"))
            self.assertEqual(data.get("id"), plugin_dir.name, plugin_dir.name)
            self.assertEqual(data.get("runtime"), "external-http", plugin_dir.name)
            names = [a["name"] for a in data.get("actions", [])]
            self.assertEqual(len(names), len(set(names)), f"duplicate actions in {plugin_dir.name}")
            for spec in data.get("schedule", []):
                self.assertIn(spec["action"], names, f"{plugin_dir.name}: schedule action not declared")

    def test_no_regressed_source_patterns(self) -> None:
        for plugin_dir in sorted(p for p in PLUGINS.iterdir() if p.is_dir()):
            source = plugin_dir / "plugin.py"
            if not source.exists():
                continue
            text = source.read_text(encoding="utf-8")
            self.assertNotIn("shell=True", text, f"{plugin_dir.name}: shell execution regressed")
            self.assertNotIn(".removeprefix(", text, f"{plugin_dir.name}: Python 3.9+ API regressed")
            if plugin_dir.name != "ptp-ai-subtitle":
                self.assertNotIn('os.environ.get("PORT"', text, f"{plugin_dir.name}: wrong port variable")


class CronHelperTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.m = load_plugin_module("reg_cron_helper", "cron-helper")

    def runs(self, expr: str, count: int = 3, tz: str = "Asia/Shanghai"):
        return self.m.handle_action("next_runs", {"expression": expr, "count": count, "timezone": tz}, {}, {})["next_runs"]

    def test_timezone_is_applied(self) -> None:
        from datetime import datetime, timedelta
        utc_now = datetime(2026, 11, 18, 12, 0, 0)
        self.assertEqual(self.m._zone_offset(utc_now, "Asia/Shanghai"), timedelta(hours=8))
        self.assertEqual(self.m._zone_offset(utc_now, "America/New_York"), timedelta(hours=-5))
        self.assertEqual(self.m._zone_offset(utc_now, "UTC"), timedelta(0))
        self.assertTrue(self.runs("0 9 * * *", 1, "Asia/Shanghai"))
        self.assertTrue(self.runs("0 9 * * *", 1, "America/New_York"))

    def test_dom_dow_or_semantics(self) -> None:
        runs = self.runs("0 0 1 * 1", 5)
        self.assertTrue(any(r.endswith("-01 00:00:00") for r in runs))
        self.assertTrue(any(not r[8:10] == "01" for r in runs))

    def test_leap_day_is_found(self) -> None:
        self.assertTrue(any(r.endswith("02-29 00:00:00") for r in self.runs("0 0 29 2 *", 3)))

    def test_invalid_field_fails_fast(self) -> None:
        with self.assertRaises(ValueError):
            self.runs("61 * * * *", 1)


class TextToolboxTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.m = load_plugin_module("reg_text_toolbox", "text-toolbox")

    def test_hash_algorithms(self) -> None:
        out = self.m.handle_action("hash", {"text": "abc", "algo": "sha256"}, {}, {})
        self.assertEqual(out["hex"], "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
        out = self.m.handle_action("hash", {"text": "abc", "algo": "sha512"}, {}, {})
        self.assertEqual(out["hex"], "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f")

    def test_unknown_hash_algorithm_raises(self) -> None:
        with self.assertRaises(ValueError):
            self.m.handle_action("hash", {"text": "abc", "algo": "md5sum"}, {}, {})


class HttpProbeRedirectTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.m = load_plugin_module("reg_http_probe", "http-probe")

    def test_redirect_to_private_host_is_blocked(self) -> None:
        class Resp:
            status = 302
            headers = {"Location": "http://127.0.0.1:8088/admin"}
            def geturl(self): return "http://public.example/start"
            def read(self): return b""
            def __enter__(self): return self
            def __exit__(self, *args): return False

        opened = []
        class Opener:
            def open(self, req, timeout=None):
                opened.append(req.full_url)
                return Resp()

        with mock.patch.object(urllib.request, "build_opener", lambda *a, **k: Opener()):
            out = self.m.handle_action("probe", {"url": "http://public.example/start"},
                                       {"follow_redirects": True, "allow_private_network": False}, {})
        self.assertFalse(out["ok"])
        self.assertIn("内网", out["error"] or "")
        self.assertEqual(len(opened), 1)


class CloudflarePathTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.m = load_plugin_module("reg_cloudflare", "cloudflare-subscribe")

    def test_path_whitelist(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            with mock.patch.dict(os.environ, {"PTP_PLUGIN_DIR": tmp}):
                allowed = self.m._resolve_hosts_path("hosts.txt", {})
                self.assertTrue(str(allowed).startswith(tmp))
                with self.assertRaises(ValueError):
                    self.m._resolve_hosts_path(str(Path(tmp) / ".." / "secret" / "passwd"), {})


class CustomCommandTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.m = load_plugin_module("reg_custom_command", "ptp-custom-command")

    def test_command_runs_without_shell(self) -> None:
        cmd = '"%s" -c print(123)' % sys.executable
        out = self.m._run_cmd(cmd, None, 10)
        self.assertEqual(out["code"], 0)
        self.assertIn("123", out["stdout"])


class NetworkGuardTests(unittest.TestCase):
    def _check_admin_url_guard(self, plugin_id: str) -> None:
        m = load_plugin_module("reg_guard_" + plugin_id.replace("-", "_"), plugin_id)
        with self.assertRaises(ValueError):
            m._guard_target_url("http://127.0.0.1:8088/admin", {})
        self.assertIn("127.0.0.1", m._guard_target_url(
            "http://127.0.0.1:8088/admin", {"allow_private_network": True}))
        with self.assertRaises(ValueError):
            m._guard_target_url("file:///etc/passwd", {"allow_private_network": True})

    def test_auto_speed_guard(self) -> None:
        self._check_admin_url_guard("auto-speed")

    def test_cloudflare_guard(self) -> None:
        self._check_admin_url_guard("cloudflare-subscribe")

    def test_plex_guard(self) -> None:
        self._check_admin_url_guard("ptp-plex-suite")

    def test_router_guards(self) -> None:
        for plugin_id in ("ptp-miwifi", "xiaomi-router"):
            m = load_plugin_module("reg_router_" + plugin_id.replace("-", "_"), plugin_id)
            self.assertIn("192.168.1.1", m._assert_router_target("http://192.168.1.1/"))
            with self.assertRaises(ValueError):
                m._assert_router_target("http://169.254.169.254/latest/meta-data")
            with self.assertRaises(ValueError):
                m._assert_router_target("ftp://example.com")


class ProtocolTests(unittest.TestCase):
    def test_malformed_body_does_not_drop_connection(self) -> None:
        for plugin_dir in sorted(p for p in PLUGINS.iterdir() if p.is_dir()):
            source = plugin_dir / "plugin.py"
            if not source.exists():
                continue
            text = source.read_text(encoding="utf-8")
            if plugin_dir.name != "ptp-ai-subtitle":
                self.assertIn("_safe_json_body", text, f"{plugin_dir.name}: safe body parser missing")
            self.assertNotIn("return json.loads(self.rfile.read", text,
                             f"{plugin_dir.name}: raw body parse regressed")

    def test_ai_subtitle_implements_event_endpoint(self) -> None:
        text = (PLUGINS / "ptp-ai-subtitle" / "plugin.py").read_text(encoding="utf-8")
        self.assertIn('self.path == "/event"', text)


if __name__ == "__main__":
    unittest.main(verbosity=2)
