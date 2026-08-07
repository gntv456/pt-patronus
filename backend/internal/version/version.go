// Package version 持有应用版本号（三端单一权威来源）。
//
// 当前版本 1.02.01。发布构建时可用 ldflags 覆盖：
//
//	go build -ldflags "-X github.com/hxpt/ptpatronus/internal/version.Version=1.2.3" ./cmd/ptpatronus
//
// 前端 Web 通过 /api/v1/system/info 读取本值；移动端与前端各有同值常量兜底。
package version

// Version 是当前应用版本号。var 而非 const，以便 ldflags 在构建期注入。
var Version = "1.03.10"
