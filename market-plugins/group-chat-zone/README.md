# group-chat-zone

按站点配置定时发送群聊/喊话消息（开源市场通用版）。

Former PTPatronus built-in, now open-source market plugin.

## 安全说明

`targets` / `zm_targets` 每行格式为 `name|post_url|cookie|message`，Cookie 以**明文**形式存放在插件配置中，并以 HTTP Header 发送到 post_url。请仅使用可信站点与专用账号 Cookie，避免把主力账号 Cookie 填入此处；同时注意 post_url 将被插件直接 POST，配置权限等同于可向该地址发送消息。

