package trakt

import (
	"io"
	"net/http"
)

// readBody 读取并关闭响应体（带 64KB 软上限，防异常大响应耗内存）。
func readBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	const limit = 64 * 1024
	data, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	return data
}
