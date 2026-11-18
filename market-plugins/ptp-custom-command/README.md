# ptp-custom-command

按行配置本地命令并定时/手动执行（工作目录与超时可配）。

安全说明：命令直接以 argv 方式执行（不使用 shell），因此 `|`、`&&`、`>`、变量展开等 shell 语法**不受支持**，会被当作普通参数。若需要管道等能力，请显式配置 `bash -c`/`cmd /c` 并自行承担相应风险。每条命令执行都会写入审计日志。

Former PTPatronus built-in, now open-source market plugin.
