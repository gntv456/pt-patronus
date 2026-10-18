package httpclient

import (
	"io"
	"log/slog"
)

// CopyBody 把上游响应体流式写到下游，并保证「上游断流 / 实际写入与声明长度不符」留下痕迹。
//
// 调用点都在这里之前已经 WriteHeader（有的还把上游 Content-Length 一起抄给了客户端），
// 状态码已经改不了：客户端能看到的只有「播放器卡顿一下」或「半张海报」。
// 日志因此是唯一能把症状和原因对上的出口——此前两处调用都写成 `_, _ = io.Copy(...)`，
// 断流被彻底丢弃，服务端零痕迹（排查时只能怀疑播放器、网络或上游本身）。
//
// expected 传上游的 Content-Length（未知为 -1/0）；attrs 由调用方给定位信息（哪个目标/条目）。
func CopyBody(dst io.Writer, src io.Reader, label string, expected int64, attrs ...any) int64 {
	written, err := io.Copy(dst, src)
	if err != nil {
		args := append([]any{"written", written}, attrs...)
		if expected > 0 {
			args = append(args, "expected", expected)
		}
		slog.Warn(label+": 上游流中断（响应头已发出，客户端只会表现为卡顿或半张图）",
			append(args, "err", err)...)
		return written
	}
	if expected > 0 && written != expected {
		args := append([]any{"written", written, "expected", expected}, attrs...)
		slog.Warn(label+": 实际写入长度与上游声明不符", args...)
	}
	return written
}
