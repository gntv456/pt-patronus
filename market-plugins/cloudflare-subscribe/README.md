# cloudflare-subscribe

Fetch subscription sources on schedule and update local hosts entries.

安全说明：`hosts_path` 只允许系统 hosts 文件（`/etc/hosts`、`C:\Windows\System32\drivers\etc\hosts`）、插件自身目录，以及 `allowed_roots` 中显式授权的目录；其他路径会被拒绝。写入前会自动生成 `.ptp-bak` 备份。

Former PTPatronus built-in, now open-source market plugin.
