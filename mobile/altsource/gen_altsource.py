#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

"""生成/更新 AltSource.json（iOS AltStore / SideStore 侧载源）。

私密分发：ipa 放网盘，不走 GitHub Release API（
本项目闭源 + 私密，改成手动传参）。每次发 iOS 版后跑一次：

  python3 mobile/altsource/gen_altsource.py \\
      --version 0.70.01 \\
      --url https://网盘直链/ptpatronus-0.70.01.ipa \\
      --size 13500000 \\
      --notes "本次更新内容..."

然后把生成的 AltSource.json 推到公开 URL（gist / 轻量仓库 raw / 静态站），
用户在 AltStore 里「添加源」订阅该 URL，即可自动检测并安装/更新 ipa。

首次使用：复制 config.json.example 为 config.json 并填 app_id / icon_url 等。
"""
import argparse
import json
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).parent


def main() -> None:
    ap = argparse.ArgumentParser(description="生成/更新 AltSource.json")
    ap.add_argument("--version", required=True, help="版本号，如 0.70.01")
    ap.add_argument("--url", required=True, help="ipa 下载直链（网盘直链）")
    ap.add_argument("--size", type=int, default=0, help="ipa 字节数（sha256sum/wc -c 取）")
    ap.add_argument("--notes", default="", help="更新日志（纯文本）")
    ap.add_argument("--config", default=str(HERE / "config.json"))
    ap.add_argument("--out", default=str(HERE / "AltSource.json"))
    args = ap.parse_args()

    cfg = json.loads(Path(args.config).read_text(encoding="utf-8"))
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    out_path = Path(args.out)
    if out_path.exists():
        data = json.loads(out_path.read_text(encoding="utf-8"))
    else:
        data = {
            "name": cfg["app_name"],
            "identifier": cfg["source_id"],
            "sourceURL": "",
            "website": cfg["website"],
            "iconURL": cfg["icon_url"],
            "subtitle": cfg["subtitle"],
            "tintColor": cfg["tint_colour"],
            "apps": [],
            "news": [],
        }

    entry = {
        "version": args.version,
        "date": now,
        "localizedDescription": args.notes,
        "downloadURL": args.url,
        "size": args.size,
    }

    apps = data.setdefault("apps", [])
    app = next((a for a in apps if a.get("bundleIdentifier") == cfg["app_id"]), None)
    if app is None:
        app = {
            "beta": False,
            "name": cfg["app_name"],
            "bundleIdentifier": cfg["app_id"],
            "developerName": cfg["developer_name"],
            "subtitle": cfg["subtitle"],
            "localizedDescription": cfg["localized_description"],
            "iconURL": cfg["icon_url"],
            "tintColor": cfg["tint_colour"],
            "category": cfg.get("category", "utilities"),
            "versions": [],
            "appPermissions": {"entitlements": [], "privacy": {}},
        }
        apps.insert(0, app)

    # 更新当前版本指针 + 在版本历史头部插入本次
    app["version"] = args.version
    app["versionDate"] = now
    app["versionDescription"] = args.notes
    app["downloadURL"] = args.url
    app["size"] = args.size
    versions = app.setdefault("versions", [])
    if not any(v.get("version") == args.version for v in versions):
        versions.insert(0, entry)

    # news 动态
    data.setdefault("news", []).insert(0, {
        "appID": cfg["app_id"],
        "title": f"{args.version} - {datetime.now(timezone.utc).strftime('%d %b')}",
        "identifier": f"release-{args.version}",
        "caption": f"{cfg['app_name']} {args.version} 已发布",
        "date": now,
        "tintColor": cfg["tint_colour"],
        "imageURL": cfg["icon_url"],
        "notify": True,
        "url": args.url,
    })

    out_path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"[gen] 已更新 {out_path}：版本 {args.version}，共 {len(app['versions'])} 个历史版本")


if __name__ == "__main__":
    main()
