package v1

// procRow 对应 ps 输出的一行进程（对齐 1Panel 进程列表）。
type procRow struct {
	PID     uint32  `json:"pid"`
	PPID    uint32  `json:"ppid"`
	User    string  `json:"user"`
	Name    string  `json:"name"`     // 进程名/命令（截断显示）
	Cmd     string  `json:"cmd"`      // 完整命令行
	State   string  `json:"state"`    // R/S/D/Z/T
	CPUPerc float64 `json:"cpu_perc"` // %
	MemPerc float64 `json:"mem_perc"` // %
	RSS     uint64  `json:"rss"`      // KB（常驻内存）
	TTY     string  `json:"tty,omitempty"`
	Time    string  `json:"time,omitempty"` // 累计 CPU 时间
}

// procListResp 进程列表响应。
type procListResp struct {
	Processes []procRow `json:"processes"`
}

// netConnRow 对应 ss/netstat 输出的一条网络连接（对齐 1Panel 网络连接列表）。
type netConnRow struct {
	Proto     string `json:"proto"`      // tcp / tcp6 / udp
	State     string `json:"state"`      // ESTAB / LISTEN / TIME-WAIT ...
	LocalAddr string `json:"local_addr"` // 本地地址:端口
	LocalPort string `json:"local_port"`
	PeerAddr  string `json:"peer_addr,omitempty"`  // 对端地址:端口
	PeerPort  string `json:"peer_port,omitempty"`
	PID       uint32 `json:"pid"`
	Process   string `json:"process,omitempty"` // 进程名
}

// netConnListResp 网络连接列表响应。
type netConnListResp struct {
	Connections []netConnRow `json:"connections"`
}

// procKillRequest 结束进程请求体。
type procKillRequest struct {
	PID     uint `json:"pid" binding:"required"`
	Force   bool `json:"force"`        // true=SIGKILL(false=SIGTERM)
	Confirm bool `json:"confirm"`      // 显式确认
}
