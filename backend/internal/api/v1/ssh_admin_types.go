package v1

// sshServiceStatus SSH 服务运行状态（对齐 1Panel SSH 服务配置页）。
type sshServiceStatus struct {
	Active     bool   `json:"active"`       // 是否正在运行
	State      string `json:"state"`        // active/inactive/failed
	Enabled    bool   `json:"enabled"`      // 开机自启
	ServiceName string `json:"service_name"` // sshd / ssh
	Port       string `json:"port"`         // 监听端口（从 sshd_config 解析）
	ListenAddr string `json:"listen_addr"`  // 监听地址
	Version    string `json:"version"`      // OpenSSH 版本
}

// sshConfigRequest 修改 SSH 服务配置请求体。
type sshConfigRequest struct {
	Port       string `json:"port"`        // 新端口
	ListenAddr string `json:"listen_addr"` // 新监听地址（0.0.0.0 / 127.0.0.1）
	Confirm    bool   `json:"confirm"`     // 显式确认
}

// sshSessionRow 一条活跃 SSH 会话（who/w 输出）。
type sshSessionRow struct {
	User   string `json:"user"`
	TTY    string `json:"tty"`     // pts/0
	From   string `json:"from"`    // 来源 IP
	Login  string `json:"login"`   // 登录时间
	Idle   string `json:"idle"`    // 空闲时长
	PID    uint32 `json:"pid"`     // 会话进程 PID（用于断开）
	What   string `json:"what"`    // 当前正在做什么
}

// sshSessionListResp 会话列表响应。
type sshSessionListResp struct {
	Sessions []sshSessionRow `json:"sessions"`
}

// sshLogRow 一条 SSH 登录日志。
type sshLogRow struct {
	Time    string `json:"time"`     // 时间
	Type    string `json:"type"`     // Accepted/Failed
	User    string `json:"user"`     // 用户
	From    string `json:"from"`     // 来源 IP
	Port    string `json:"port"`     // 源端口
	Method string `json:"method"`    // password/publickey
}

// sshLogListResp 登录日志列表响应。
type sshLogListResp struct {
	Logs []sshLogRow `json:"logs"`
}

// sshActionRequest SSH 服务操作请求（start/stop/restart/reload）。
type sshActionRequest struct {
	Action  string `json:"action"`  // start | stop | restart | reload
	Confirm bool   `json:"confirm"`
}
