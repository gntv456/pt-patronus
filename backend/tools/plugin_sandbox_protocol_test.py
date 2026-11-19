# -*- coding: utf-8 -*-
"""Phase-3: protocol robustness across all market plugins."""
import importlib.util, json, os, subprocess, sys, threading, urllib.error, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CTL = ROOT / "backend" / "tools" / "pluginctl.py"
PLUGINS = ROOT / "market-plugins"
spec = importlib.util.spec_from_file_location("pluginctl", str(CTL))
pc = importlib.util.module_from_spec(spec); sys.modules["pluginctl"] = pc; spec.loader.exec_module(pc)

def raw(url, token, path, payload=None, method="POST"):
    headers = {}
    if token: headers["Authorization"] = "Bearer " + token
    if payload is not None: headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url + path, data=payload, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=8) as res:
            return res.status, res.read().decode("utf-8", "replace")[:300]
    except urllib.error.HTTPError as e:
        return e.code, (e.read().decode("utf-8", "replace")[:300] if e.fp else "")
    except Exception as e:
        return 0, str(e)

def test_plugin(pid):
    pdir = PLUGINS / pid
    manifest = json.loads((pdir / "plugin.json").read_text("utf-8"))
    entry = manifest.get("entry") or {}
    hp, pp = pc.free_port(), pc.free_port()
    while hp == pp: pp = pc.free_port()
    ht, pt = pc.random_token(), pc.random_token()
    hs = pc.create_mock_host_server("127.0.0.1", hp, ht, verbose=False)
    th = threading.Thread(target=hs.serve_forever, daemon=True); th.start()
    hurl = "http://127.0.0.1:%d" % hp
    purl = "http://127.0.0.1:%d" % pp
    env = os.environ.copy()
    env.update({"PTP_PLUGIN_ID": manifest["id"], "PTP_PLUGIN_PORT": str(pp),
                "PTP_PLUGIN_TOKEN": pt, "PTP_PLUGIN_BASE_URL": purl,
                "PTP_PLUGIN_DIR": str(pdir), "PTP_HOST_URL": hurl, "PTP_HOST_TOKEN": ht,
                "PTP_HOST_PROTOCOL": "1", "PTP_HOST_CAPABILITIES": ",".join(pc.SUPPORTED_HOST_CAPABILITIES)})
    cmd = pc.resolve_command(pdir, str(entry.get("command") or ""))
    args = [pc.replace_vars(str(x), pp, pt, str(pdir), manifest["id"]) for x in (entry.get("args") or [])]
    wd = pc.resolve_work_dir(pdir, entry.get("work_dir"))
    devnull = open(os.devnull, "w")
    proc = None
    out = {"plugin": pid, "checks": [], "alive_after": None}
    try:
        proc = subprocess.Popen([cmd] + args, cwd=wd, env=env, stdout=devnull, stderr=subprocess.STDOUT)
        pc.wait_for_health(purl, "/health", 15, proc)
        checks = out["checks"]
        s, b = raw(purl, None, "/health", method="GET")
        checks.append(("health_no_auth", s, "ok" in b))
        s, b = raw(purl, pt, "/no-such-path", method="GET")
        checks.append(("get_404", s == 404, b[:60]))
        s, b = raw(purl, "wrong-token", "/action", b'{"action":"status"}')
        checks.append(("bad_token_401", s == 401, b[:60]))
        s, b = raw(purl, pt, "/action", b'{')
        checks.append(("malformed_json", s in (200, 400, 500), b[:80]))
        s, b = raw(purl, pt, "/action", b'{"action":"__definitely_unknown__","input":{},"config":{}}')
        checks.append(("unknown_action", s in (200, 400), b[:80]))
        s, b = raw(purl, pt, "/event", b'{"type":"__definitely_unknown__","data":{},"config":{}}')
        checks.append(("unknown_event", s in (200, 400), b[:80]))
        s, b = raw(purl, None, "/health", method="GET")
        out["alive_after"] = (s == 200)
    except Exception as e:
        out["error"] = str(e)
    finally:
        if proc is not None and proc.poll() is None:
            proc.terminate()
            try: proc.wait(timeout=3)
            except Exception: proc.kill(); proc.wait(timeout=3)
        hs.shutdown(); hs.server_close(); th.join(timeout=2); devnull.close()
    return out

results = []
for pid in sorted(p.name for p in PLUGINS.iterdir() if p.is_dir() and (p / "plugin.json").exists()):
    r = test_plugin(pid)
    results.append(r)
    bad = [c for c in r["checks"] if isinstance(c[1], bool) and not c[1]]
    print("[%s] checks=%d bad=%d alive=%s %s" % (pid, len(r["checks"]), len(bad), r["alive_after"], r.get("error", "")), flush=True)
# tmp/ 是本地运行时目录（gitignore），CI 全新 checkout 没有——写结果前先确保存在
(ROOT / "tmp").mkdir(exist_ok=True)
(ROOT / "tmp" / "plugin-phase3-results.json").write_text(json.dumps(results, ensure_ascii=False, indent=2), "utf-8")
print("DONE phase3")
