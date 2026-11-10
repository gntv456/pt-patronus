# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

from __future__ import annotations

import base64
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("pluginctl.py")
SPEC = importlib.util.spec_from_file_location("pluginctl", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"unable to load {MODULE_PATH}")
pluginctl = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(pluginctl)


class PluginCtlTests(unittest.TestCase):
    def test_create_with_vendor_tools_and_ci(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            plugin_dir = Path(tmp_dir) / "plugin-ci"
            self.run_cli(
                "create",
                "plugin-ci",
                "--directory",
                str(plugin_dir),
                "--vendor-tools",
                "--with-ci",
                "both",
            )

            tool_dir = plugin_dir / "tools" / "ptpatronus"
            github_workflow = plugin_dir / ".github" / "workflows" / "plugin-package.yml"
            gitea_workflow = plugin_dir / ".gitea" / "workflows" / "plugin-package.yml"
            github_text = github_workflow.read_text(encoding="utf-8")
            gitea_text = gitea_workflow.read_text(encoding="utf-8")

            self.assertTrue((tool_dir / "pluginctl.py").exists())
            self.assertTrue((tool_dir / "plugin-manifest.schema.json").exists())
            self.assertTrue(github_workflow.exists())
            self.assertTrue(gitea_workflow.exists())
            self.assertIn("python tools/ptpatronus/pluginctl.py validate .", github_text)
            self.assertIn("gh release create", github_text)
            self.assertIn("--json-out dist/plugin-package.json", github_text)
            self.assertNotIn("gh release create", gitea_text)

    def test_create_typescript_template_with_view(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            plugin_dir = Path(tmp_dir) / "my-ts-plugin"
            self.run_cli(
                "create",
                "my-ts-plugin",
                "--directory",
                str(plugin_dir),
                "--template",
                "typescript",
                "--with-view",
            )

            manifest = json.loads((plugin_dir / "plugin.json").read_text(encoding="utf-8"))
            package_json = json.loads((plugin_dir / "package.json").read_text(encoding="utf-8"))

            self.assertEqual(manifest["entry"]["command"], "node")
            self.assertEqual(manifest["entry"]["args"], ["dist/plugin.js"])
            self.assertEqual(manifest["contributes"]["views"][0]["path"], "index.html")
            self.assertIn("typescript", manifest["keywords"])
            self.assertTrue((plugin_dir / "src" / "plugin.ts").exists())
            self.assertTrue((plugin_dir / "web-src" / "main.ts").exists())
            self.assertTrue((plugin_dir / "scripts" / "build-view.mjs").exists())
            self.assertTrue((plugin_dir / "web" / "index.html").exists())
            self.assertEqual(package_json["scripts"]["build"], "npm run build:server && npm run build:view")
            self.assertEqual(package_json["scripts"]["build:view"], "node scripts/build-view.mjs")
            pluginctl.validate_manifest(plugin_dir)

    def test_vendored_tool_validates_without_host_checkout(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            plugin_dir = Path(tmp_dir) / "standalone-plugin"
            self.run_cli(
                "create",
                "standalone-plugin",
                "--directory",
                str(plugin_dir),
                "--vendor-tools",
            )

            tool_path = plugin_dir / "tools" / "ptpatronus" / "pluginctl.py"
            result = self.run_script(tool_path, "validate", str(plugin_dir))
            self.assertIn("ok: standalone-plugin v0.1.0", result.stdout)

    def test_dev_runs_build_command_before_sync(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            plugin_dir = self.make_plugin(tmp / "build-plugin", "build-plugin")
            target_root = tmp / "plugins"
            build_flag = plugin_dir / "build.txt"

            self.run_cli(
                "dev",
                str(plugin_dir),
                "--plugin-root",
                str(target_root),
                "--build-command",
                f"\"{sys.executable}\" -c \"from pathlib import Path; Path('build.txt').write_text('ok', encoding='utf-8')\"",
                "--once",
            )

            self.assertEqual(build_flag.read_text(encoding="utf-8"), "ok")
            self.assertEqual((target_root / "build-plugin" / "build.txt").read_text(encoding="utf-8"), "ok")

    def test_package_auto_build_infers_package_json_build_script(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            plugin_dir = self.make_plugin(tmp / "auto-build-plugin", "auto-build-plugin")
            out_dir = tmp / "dist"
            npm_dir = tmp / "bin"
            npm_dir.mkdir(parents=True, exist_ok=True)
            build_flag = plugin_dir / "auto-build.txt"

            (plugin_dir / "package.json").write_text(json.dumps({
                "name": "auto-build-plugin",
                "private": True,
                "scripts": {
                    "build": "python -c \"from pathlib import Path; Path('auto-build.txt').write_text('built', encoding='utf-8')\"",
                },
            }, indent=2) + "\n", encoding="utf-8")

            npm_cmd = npm_dir / "npm.cmd"
            npm_cmd.write_text(
                "@echo off\r\n"
                "if \"%1\"==\"run\" if \"%2\"==\"build\" (\r\n"
                f"  \"{sys.executable}\" -c \"from pathlib import Path; Path('auto-build.txt').write_text('built', encoding='utf-8')\"\r\n"
                "  exit /b 0\r\n"
                ")\r\n"
                "exit /b 1\r\n",
                encoding="utf-8",
            )
            env = dict(os.environ)
            env["PATH"] = str(npm_dir) + os.pathsep + os.environ.get("PATH", "")

            result = self.run_cli(
                "package",
                str(plugin_dir),
                "--out-dir",
                str(out_dir),
                env=env,
            )

            self.assertEqual(build_flag.read_text(encoding="utf-8"), "built")
            self.assertIn("auto-build: npm run build", result.stderr)
            archive = out_dir / "auto-build-plugin-0.1.0.zip"
            self.assertTrue(archive.exists())

    def test_ed25519_rfc8032_vector_1(self) -> None:
        seed = bytes.fromhex("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
        want_public = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
        want_signature = (
            "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e06522490155"
            "5fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"
        )
        got_public = pluginctl.ed25519_public_key(seed)
        got_signature = pluginctl.ed25519_sign(seed, b"")
        self.assertEqual(got_public.hex(), want_public)
        self.assertEqual(got_signature.hex(), want_signature)
        self.assertTrue(pluginctl.ed25519_verify(got_public, b"", got_signature))

    def test_package_updates_signed_market(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            plugin_dir = self.make_plugin(tmp / "demo-plugin", "demo-plugin")
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            market = tmp / "market" / "plugin-market.json"
            out_dir = tmp / "market" / "dist"
            json_out = tmp / "market" / "plugin-package.json"

            self.run_cli(
                "keygen",
                "--private-key-out",
                str(private_key),
                "--public-key-out",
                str(public_key),
            )
            result = self.run_cli(
                "package",
                str(plugin_dir),
                "--out-dir",
                str(out_dir),
                "--json-out",
                str(json_out),
                "--private-key",
                str(private_key),
                "--market",
                str(market),
                "--market-name",
                "Test Market",
                "--publisher-name",
                "Test Publisher",
                "--publisher-verified",
                "--changelog",
                "Initial release",
            )

            entry = json.loads(result.stdout)
            entry_from_file = json.loads(json_out.read_text(encoding="utf-8"))
            index = json.loads(market.read_text(encoding="utf-8"))
            archive_path = out_dir / "demo-plugin-0.1.0.zip"

            self.assertEqual(entry["archive"], "dist/demo-plugin-0.1.0.zip")
            self.assertEqual(entry_from_file["archive"], entry["archive"])
            self.assertEqual(index["name"], "Test Market")
            self.assertEqual(index["public_key"], public_key.read_text(encoding="utf-8").strip())
            self.assertEqual(index["plugins"][0]["archive"], entry["archive"])
            self.assertEqual(index["plugins"][0]["publisher"]["name"], "Test Publisher")
            self.assertEqual(index["plugins"][0]["changelog"], ["Initial release"])
            self.assertTrue(
                pluginctl.ed25519_verify(
                    base64.b64decode(index["public_key"]),
                    archive_path.read_bytes(),
                    base64.b64decode(index["plugins"][0]["signature"]),
                )
            )

    def test_signed_market_rejects_unsigned_entry(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            market = tmp / "plugin-market.json"
            out_dir = tmp / "dist"
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            plugin_a = self.make_plugin(tmp / "plugin-a", "plugin-a")
            plugin_b = self.make_plugin(tmp / "plugin-b", "plugin-b")

            self.run_cli(
                "keygen",
                "--private-key-out",
                str(private_key),
                "--public-key-out",
                str(public_key),
            )
            self.run_cli(
                "package",
                str(plugin_a),
                "--out-dir",
                str(out_dir),
                "--private-key",
                str(private_key),
                "--market",
                str(market),
            )
            result = self.run_cli(
                "package",
                str(plugin_b),
                "--out-dir",
                str(out_dir),
                "--market",
                str(market),
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("signed markets require a signature for every plugin entry", result.stderr)

    def test_market_aggregate_merges_signed_packages(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            packages = tmp / "packages"
            packages.mkdir()
            self.run_cli("keygen", "--private-key-out", str(private_key), "--public-key-out", str(public_key))
            for slug in ("alpha-plugin", "beta-plugin"):
                plugin_dir = self.make_plugin(tmp / slug, slug)
                self.run_cli(
                    "package",
                    str(plugin_dir),
                    "--out-dir",
                    str(tmp / "dist"),
                    "--private-key",
                    str(private_key),
                    "--json-out",
                    str(packages / f"{slug}.json"),
                )

            market = tmp / "plugin-market.json"
            result = self.run_cli(
                "market",
                "aggregate",
                "--packages-dir",
                str(packages),
                "--public-key",
                str(public_key),
                "--out",
                str(market),
                "--name",
                "Test Market",
            )

            index = json.loads(market.read_text(encoding="utf-8"))
            self.assertEqual([p["id"] for p in index["plugins"]], ["alpha-plugin", "beta-plugin"])
            self.assertEqual(index["name"], "Test Market")
            self.assertEqual(index["public_key"], public_key.read_text(encoding="utf-8").strip())
            for entry in index["plugins"]:
                self.assertTrue(entry["signature"])
            self.assertIn("aggregated 2 plugin(s)", result.stdout)
            self.assertIn("signed", result.stdout)

    def test_market_aggregate_dedups_latest_version(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            packages = tmp / "packages"
            packages.mkdir()
            plugin_dir = self.make_plugin(tmp / "dup-plugin", "dup-plugin")
            self.run_cli("package", str(plugin_dir), "--out-dir", str(tmp / "dist"), "--json-out", str(packages / "dup-old.json"))
            manifest = json.loads((plugin_dir / "plugin.json").read_text(encoding="utf-8"))
            manifest["version"] = "0.2.0"
            (plugin_dir / "plugin.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
            self.run_cli("package", str(plugin_dir), "--out-dir", str(tmp / "dist"), "--json-out", str(packages / "dup-new.json"))

            market = tmp / "plugin-market.json"
            result = self.run_cli("market", "aggregate", "--packages-dir", str(packages), "--out", str(market))

            index = json.loads(market.read_text(encoding="utf-8"))
            self.assertEqual([p["version"] for p in index["plugins"]], ["0.2.0"])
            self.assertIn("dropped 0.1.0", result.stderr)

    def test_market_aggregate_rejects_partial_signing(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            packages = tmp / "packages"
            packages.mkdir()
            self.run_cli("keygen", "--private-key-out", str(private_key), "--public-key-out", str(public_key))
            signed = self.make_plugin(tmp / "signed-plugin", "signed-plugin")
            self.run_cli("package", str(signed), "--out-dir", str(tmp / "dist"), "--private-key", str(private_key), "--json-out", str(packages / "signed-plugin.json"))
            unsigned = self.make_plugin(tmp / "bare-plugin", "bare-plugin")
            self.run_cli("package", str(unsigned), "--out-dir", str(tmp / "dist"), "--json-out", str(packages / "bare-plugin.json"))

            result = self.run_cli(
                "market",
                "aggregate",
                "--packages-dir",
                str(packages),
                "--public-key",
                str(public_key),
                "--out",
                str(tmp / "plugin-market.json"),
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("requires every plugin entry to be signed", result.stderr)

    def test_market_aggregate_unsigned_market(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            packages = tmp / "packages"
            packages.mkdir()
            plugin_dir = self.make_plugin(tmp / "free-plugin", "free-plugin")
            self.run_cli("package", str(plugin_dir), "--out-dir", str(tmp / "dist"), "--json-out", str(packages / "free-plugin.json"))

            result = self.run_cli("market", "aggregate", "--packages-dir", str(packages), "--out", str(tmp / "plugin-market.json"))
            index = json.loads((tmp / "plugin-market.json").read_text(encoding="utf-8"))

            self.assertNotIn("public_key", index)
            self.assertNotIn("signature", index["plugins"][0])
            self.assertIn("unsigned", result.stdout)

    def test_market_aggregate_rewrites_archive_ref(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            packages = tmp / "packages"
            packages.mkdir()
            self.run_cli("keygen", "--private-key-out", str(private_key), "--public-key-out", str(public_key))
            plugin_dir = self.make_plugin(tmp / "rewrite-plugin", "rewrite-plugin")
            package_json = packages / "rewrite-plugin.json"
            self.run_cli("package", str(plugin_dir), "--out-dir", str(tmp / "dist"), "--private-key", str(private_key), "--json-out", str(package_json))
            original = json.loads(package_json.read_text(encoding="utf-8"))

            self.run_cli(
                "market",
                "aggregate",
                "--package",
                str(package_json),
                "--public-key",
                str(public_key),
                "--release-base",
                "https://github.com/org/market/releases/download",
                "--out",
                str(tmp / "plugin-market.json"),
            )

            index = json.loads((tmp / "plugin-market.json").read_text(encoding="utf-8"))
            entry = index["plugins"][0]
            self.assertEqual(
                entry["archive"],
                "https://github.com/org/market/releases/download/rewrite-plugin-v0.1.0/rewrite-plugin-0.1.0.zip",
            )
            self.assertEqual(entry["sha256"], original["sha256"])
            self.assertEqual(entry["signature"], original["signature"])

    def test_market_aggregate_verifies_archives_dir(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            archives = tmp / "archives"
            archives.mkdir()
            self.run_cli("keygen", "--private-key-out", str(private_key), "--public-key-out", str(public_key))
            plugin_dir = self.make_plugin(tmp / "verify-plugin", "verify-plugin")
            out_dir = tmp / "dist"
            package_json = tmp / "verify-plugin.json"
            self.run_cli("package", str(plugin_dir), "--out-dir", str(out_dir), "--private-key", str(private_key), "--json-out", str(package_json))
            zip_path = out_dir / "verify-plugin-0.1.0.zip"
            archive_copy = archives / zip_path.name
            archive_copy.write_bytes(zip_path.read_bytes())

            # happy path: archive matches sha256 + signature
            self.run_cli(
                "market",
                "aggregate",
                "--package",
                str(package_json),
                "--archives-dir",
                str(archives),
                "--public-key",
                str(public_key),
                "--out",
                str(tmp / "ok.json"),
            )

            # tamper the archive -> sha256 mismatch
            archive_copy.write_bytes(archive_copy.read_bytes() + b"tampered")
            result = self.run_cli(
                "market",
                "aggregate",
                "--package",
                str(package_json),
                "--archives-dir",
                str(archives),
                "--public-key",
                str(public_key),
                "--out",
                str(tmp / "bad.json"),
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("sha256 mismatch", result.stderr)

    def test_market_init_scaffold(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            target = Path(tmp_dir) / "market-repo"
            self.run_cli(
                "market",
                "init",
                str(target),
                "--name",
                "My Market",
                "--generate-keys",
                "--release-base",
                "https://github.com/org/market/releases/download",
            )

            self.assertTrue((target / "README.md").exists())
            self.assertTrue((target / "packages" / ".gitkeep").exists())
            self.assertTrue((target / "plugin-market.json").exists())
            self.assertTrue((target / "keys" / "plugin-market.public.key").exists())
            self.assertTrue((target / "keys" / "plugin-market.private.key").exists())
            self.assertTrue((target / "tools" / "ptpatronus" / "pluginctl.py").exists())
            workflow = (target / ".github" / "workflows" / "aggregate.yml").read_text(encoding="utf-8")
            self.assertIn("market aggregate", workflow)
            self.assertIn("--public-key keys/plugin-market.public.key", workflow)
            self.assertIn("--release-base https://github.com/org/market/releases/download", workflow)
            readme = (target / "README.md").read_text(encoding="utf-8")
            self.assertIn("My Market", readme)
            gitignore = (target / ".gitignore").read_text(encoding="utf-8")
            self.assertIn("private.key", gitignore)

            # the README rebuild command must be valid shell: every line but the last
            # ends with a backslash continuation
            rebuild_block = readme.split("```bash", 1)[1].split("```", 1)[0].strip().splitlines()
            for line in rebuild_block[:-1]:
                self.assertTrue(line.rstrip().endswith("\\"), f"missing continuation in README rebuild line: {line!r}")
            self.assertFalse(rebuild_block[-1].rstrip().endswith("\\"))

    def test_market_aggregate_central_resign(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            packages = tmp / "packages"
            archives = tmp / "archives"
            packages.mkdir()
            archives.mkdir()
            self.run_cli("keygen", "--private-key-out", str(private_key), "--public-key-out", str(public_key))
            for slug in ("resign-alpha", "resign-beta"):
                plugin_dir = self.make_plugin(tmp / slug, slug)
                # package UNSIGNED — authors hand the operator bare zips
                self.run_cli(
                    "package",
                    str(plugin_dir),
                    "--out-dir",
                    str(tmp / "dist"),
                    "--json-out",
                    str(packages / f"{slug}.json"),
                )
                zip_path = tmp / "dist" / f"{slug}-0.1.0.zip"
                (archives / zip_path.name).write_bytes(zip_path.read_bytes())

            market = tmp / "plugin-market.json"
            result = self.run_cli(
                "market",
                "aggregate",
                "--packages-dir",
                str(packages),
                "--private-key",
                str(private_key),
                "--archives-dir",
                str(archives),
                "--out",
                str(market),
            )

            index = json.loads(market.read_text(encoding="utf-8"))
            self.assertEqual(index["public_key"], public_key.read_text(encoding="utf-8").strip())
            self.assertEqual(len(index["plugins"]), 2)
            pub_bytes = base64.b64decode(index["public_key"])
            for entry in index["plugins"]:
                self.assertTrue(entry["signature"])
                local_zip = archives / f"{entry['id']}-0.1.0.zip"
                self.assertTrue(
                    pluginctl.ed25519_verify(pub_bytes, local_zip.read_bytes(), base64.b64decode(entry["signature"]))
                )
            self.assertIn("signed", result.stdout)

    def test_market_aggregate_resign_requires_archives_dir(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            private_key = tmp / "market.private.key"
            public_key = tmp / "market.public.key"
            self.run_cli("keygen", "--private-key-out", str(private_key), "--public-key-out", str(public_key))
            plugin_dir = self.make_plugin(tmp / "lonely", "lonely")
            package_json = tmp / "lonely.json"
            self.run_cli("package", str(plugin_dir), "--out-dir", str(tmp / "dist"), "--json-out", str(package_json))

            result = self.run_cli(
                "market",
                "aggregate",
                "--package",
                str(package_json),
                "--private-key",
                str(private_key),
                "--out",
                str(tmp / "plugin-market.json"),
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("requires --archives-dir", result.stderr)

    def make_plugin(self, path: Path, plugin_id: str) -> Path:
        path.mkdir(parents=True, exist_ok=True)
        manifest = {
            "id": plugin_id,
            "name": plugin_id.replace("-", " ").title(),
            "version": "0.1.0",
            "runtime": "external-http",
            "entry": {
                "command": "python",
                "args": ["plugin.py"],
                "health": "/health",
            },
            "permissions": [],
            "events": [],
            "actions": [],
            "config_schema": [],
        }
        (path / "plugin.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        (path / "plugin.py").write_text("print('ok')\n", encoding="utf-8")
        return path

    def run_cli(self, *args: str, check: bool = True, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
        return self.run_script(MODULE_PATH, *args, check=check, env=env)

    def run_script(self, script_path: Path, *args: str, check: bool = True, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(script_path), *args],
            check=check,
            capture_output=True,
            text=True,
            env=env,
        )


if __name__ == "__main__":
    unittest.main()
