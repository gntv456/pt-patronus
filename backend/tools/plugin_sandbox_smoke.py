# -*- coding: utf-8 -*-
"""Sandbox smoke test for all PTPatronus market plugins.

Starts each plugin via pluginctl's mock host, waits for /health, then invokes
a table of safe actions/events. Run from the repository root:

    python backend/tools/plugin_sandbox_smoke.py [plugin_id ...]

Results: tmp/plugin-deep-results.json ; logs: tmp/plugin-test-logs/<id>.log
"""
import importlib.util, json, os, subprocess, sys, threading, time, traceback
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CTL = ROOT / "backend" / "tools" / "pluginctl.py"
PLUGINS = ROOT / "market-plugins"
LOGDIR = ROOT / "tmp" / "plugin-test-logs"
LOGDIR.mkdir(parents=True, exist_ok=True)

spec = importlib.util.spec_from_file_location("pluginctl", str(CTL))
pc = importlib.util.module_from_spec(spec)
sys.modules["pluginctl"] = pc
spec.loader.exec_module(pc)

# --- test cases: id -> list of (kind, label, payload) ---
A = lambda name, inp=None, cfg=None: ("action", name, {"action": name, "input": inp or {}, "config": cfg or {}})
E = lambda etype, data=None, cfg=None: ("event", etype, {"type": etype, "data": data or {}, "config": cfg or {}})

EXTRA_URL = ""

CASES = {
 "auto-speed": [A("run_speedtest", {}, {"test_url": "{HOST_URL}/health", "timeout_seconds": 5, "notify": False, "allow_private_network": True})],
 "cloudflare-subscribe": [A("preview", {}, {"subscribe_urls": "", "marker": "x", "hosts_path": ""})],
 "cron-helper": [A("next_runs", {"expression": "*/5 * * * *", "count": 3}),
                 A("next_runs", {"expression": "0 9 * * 1", "count": 2}),
                 A("next_runs", {"expression": "bad", "count": 1}),
                 A("next_runs", {"expression": "0 9 * * *", "count": 2, "timezone": "America/New_York"}),
                 A("next_runs", {"expression": "0 0 1 * 1", "count": 3}),
                 A("next_runs", {"expression": "0 0 29 2 *", "count": 2})],
 "event-audit": [A("echo", {"message": "hi"}), A("emit_test_event", {"message": "hi"}),
                 E("plugin.test", {"message": "hello"}), E("webhook", {"a": 1})],
 "group-chat-zone": [A("preview", {}, {"targets": "", "zm_targets": "", "notify": False})],
 "http-probe": [A("probe", {"url": "{HOST_URL}/health", "method": "GET"},
                   {"allow_private_network": "true", "timeout_seconds": 5, "follow_redirects": True, "max_body_bytes": 10000}),
                A("probe", {"url": "http://127.0.0.1:1/x"}, {"allow_private_network": False, "timeout_seconds": 2}),
                A("probe", {"url": "not-a-url"}, {})],
 "json-toolbox": [A("validate", {"json": '{"a":1}'}), A("validate", {"json": "{"}),
                  A("format", {"json": '{"b":2,"a":1}'}, {"indent": 2}),
                  A("minify", {"json": '{"b": 2, "a": 1}'}),
                  A("get", {"json": '{"a":{"b":[1,2,3]}}', "path": "a.b.1"}),
                  A("get", {"json": '[1,2]', "path": "9"}),
                  A("get", {"json": '{"a":1}', "path": "x.y"})],
 "keyword-alert": [A("test_match", {"payload": {"title": "Hello ERROR world"}},
                     {"keywords": "error", "fields": "title", "case_sensitive": False}),
                   A("test_match", {"payload": {"title": "all good"}}, {"keywords": "error"}),
                   E("webhook", {"title": "ERROR happened"}, {"keywords": "error", "event_types": "webhook"})],
 "mp-bridge-cloud-strm": [A("rules"), A("drives"), A("records"), A("health_report")],
 "mp-bridge-media-refresh": [A("servers"), A("scan_all"), A("dup_report")],
 "mp-bridge-site-signin": [A("last_result"), A("fetch_messages", {"mailbox": "inbox"}), A("fetch_freeleech")],
 "mp-bridge-subscribe-enhance": [A("hot_feeds"), A("report"), A("check_all")],
 "mp-bridge-torrent-editor": [A("list_templates"), A("select", {}, {"criteria": ""}), A("apply_template")],
 "mp-bridge-transfer-organize": [A("completed_torrents"), A("history"), A("sweep")],
 "notice-forwarder": [A("send_test_notice", {"title": "t", "body": "b"}),
                      E("plugin.test", {"message": "hello"}, {"event_types": "plugin.test"})],
 "ptp-ai-subtitle": [A("status")],
 "ptp-cd2-assistant": [A("status", {}, {"cd2_confs": ""}), A("info", {}, {"cd2_confs": ""})],
 "ptp-cookiecloud-sync": [A("status")],
 "ptp-custom-command": [A("run", {}, {"commands": ""}),
                         A("run_one", {"index": 0}, {"commands": ""}),
                         A("run", {}, {"commands": "python -c print(123)", "notify": False})],
 "ptp-dirwatch": [A("scan", {}, {"dirs": "", "media_exts": ".mp4"}), A("reset_seen")],
 "ptp-drama-scrape": [A("status"), A("scan", {}, {"scan_paths": ""})],
 "ptp-ffmpeg-thumb": [A("status"), A("scan", {}, {"scan_paths": ""})],
 "ptp-forum-signin": [A("labels", {}, {"sites": ""})],
 "ptp-library-scrape": [A("status"), A("scrape", {}, {"scraper_paths": "", "mode": "all"})],
 "ptp-mcp-server": [A("tools", {}, {"enabled": False, "allow_tools": ""}), A("status")],
 "ptp-miwifi": [A("status", {}, {"base_url": ""}), A("devices", {}, {"base_url": ""})],
 "ptp-playlet-category": [A("status"), A("scan", {}, {"scan_paths": "", "dry_run": True})],
 "ptp-plex-suite": [A("status", {}, {"base_url": ""}), A("libraries", {}, {"base_url": ""})],
 "ptp-tgbot-bridge": [A("status"), A("health")],
 "ptp-trakt-sync": [A("status"), A("auth_status")],
 "text-toolbox": [A("stats", {"text": "a b\nc"}), A("transform", {"text": " hi ", "mode": "upper"}),
                  A("transform", {"text": "x", "mode": "bogus"}), A("replace", {"text": "aab", "old": "a", "new": "X"}),
                  A("split_unique", {"text": "a\nb\na", "sep": "\n"}),
                  A("hash", {"text": "abc", "algo": "sha256"}), A("hash", {"text": "abc", "algo": "md5"})],
 "twofa-helper": [A("list", {"accounts": "Test|JBSWY3DPEHPK3PXP"}),
                  A("generate", {"accounts": "Test|JBSWY3DPEHPK3PXP", "name": "Test"}),
                  A("generate", {})],
 "webhook-transformer": [A("preview", {"payload": {"title": "T", "body": "B"}}, {"mappings": "name=title\ndesc=body"}),
                         E("webhook", {"title": "T"}, {"mappings": "name=title", "send_notice": False})],
 "xiaomi-router": [A("status", {}, {"router_url": ""}), A("port_list", {}, {"router_url": ""})],
}

def run_plugin(pid):
    pdir = PLUGINS / pid
    mf = pdir / "plugin.json"
    manifest = json.loads(mf.read_text("utf-8"))
    entry = manifest.get("entry") or {}
    host_port, plugin_port = pc.free_port(), pc.free_port()
    while host_port == plugin_port:
        plugin_port = pc.free_port()
    host_token, plugin_token = pc.random_token(), pc.random_token()
    host_server = pc.create_mock_host_server("127.0.0.1", host_port, host_token, verbose=False)
    ht = threading.Thread(target=host_server.serve_forever, daemon=True); ht.start()
    host_url = "http://127.0.0.1:%d" % host_port
    plugin_url = "http://127.0.0.1:%d" % plugin_port
    env = os.environ.copy()
    env.update({
        "PTP_PLUGIN_ID": manifest["id"], "PTP_PLUGIN_PORT": str(plugin_port),
        "PTP_PLUGIN_TOKEN": plugin_token, "PTP_PLUGIN_BASE_URL": plugin_url,
        "PTP_PLUGIN_DIR": str(pdir), "PTP_HOST_URL": host_url, "PTP_HOST_TOKEN": host_token,
        "PTP_HOST_PROTOCOL": "1", "PTP_HOST_CAPABILITIES": ",".join(pc.SUPPORTED_HOST_CAPABILITIES),
    })
    for k, v in (entry.get("env") or {}).items():
        env[str(k)] = pc.replace_vars(str(v), plugin_port, plugin_token, str(pdir), manifest["id"])
    cmd = pc.resolve_command(pdir, str(entry.get("command") or ""))
    args = [pc.replace_vars(str(x), plugin_port, plugin_token, str(pdir), manifest["id"]) for x in (entry.get("args") or [])]
    wd = pc.resolve_work_dir(pdir, entry.get("work_dir"))
    lf = open(str(LOGDIR / (pid + ".log")), "w", encoding="utf-8")
    proc = None
    out = {"plugin": pid, "health": None, "cases": [], "stderr": ""}
    try:
        proc = subprocess.Popen([cmd] + args, cwd=wd, env=env, stdout=lf, stderr=subprocess.STDOUT)
        pc.wait_for_health(plugin_url, str(entry.get("health") or "/health"), 15, proc)
        out["health"] = "ok"
        hctx = pc.build_host_context(manifest, host_url, host_token)
        for kind, label, payload in CASES.get(pid, []):
            if kind == "action":
                body = dict(payload)
                body["host"] = hctx
                body["config"] = {k: (v.replace("{HOST_URL}", host_url).replace("{EXTRA_URL}", EXTRA_URL) if isinstance(v, str) else v)
                                  for k, v in (body.get("config") or {}).items()}
                body["input"] = {k: (v.replace("{HOST_URL}", host_url).replace("{EXTRA_URL}", EXTRA_URL) if isinstance(v, str) else v)
                                 for k, v in (body.get("input") or {}).items()}
                resp = pc.plugin_request(plugin_url, plugin_token, "/action", body)
            else:
                body = dict(payload); body["host"] = hctx
                resp = pc.plugin_request(plugin_url, plugin_token, "/event", body)
            out["cases"].append({"label": label, "kind": kind, "resp": resp})
    except Exception as exc:
        out["health"] = "FAIL: " + str(exc)
        out["cases"].append({"label": "harness", "kind": "error", "resp": {"error": traceback.format_exc().splitlines()[-1]}})
    finally:
        if proc is not None and proc.poll() is None:
            proc.terminate()
            try: proc.wait(timeout=3)
            except Exception: proc.kill(); proc.wait(timeout=3)
        host_server.shutdown(); host_server.server_close(); ht.join(timeout=2)
        lf.close()
        try: out["stderr"] = (LOGDIR / (pid + ".log")).read_text("utf-8", "replace")[-1500:]
        except Exception: pass
    return out

def main():
    ids = sorted(p.name for p in PLUGINS.iterdir() if p.is_dir() and (p / "plugin.json").exists())
    if len(sys.argv) > 1: ids = [x for x in ids if x in sys.argv[1:]]
    results = []
    for pid in ids:
        try:
            r = run_plugin(pid)
        except Exception as exc:
            r = {"plugin": pid, "health": "HARNESS_ERROR", "cases": [], "stderr": traceback.format_exc()[-800:]}
        results.append(r)
        h = r.get("health")
        bad = sum(1 for c in r["cases"] if not c["resp"].get("ok", True))
        print("[%s] health=%s cases=%d notok=%d" % (pid, h, len(r["cases"]), bad))
        sys.stdout.flush()
    (ROOT / "tmp" / "plugin-deep-results.json").write_text(json.dumps(results, ensure_ascii=False, indent=2), "utf-8")
    print("DONE results -> tmp/plugin-deep-results.json")

if __name__ == "__main__":
    main()
