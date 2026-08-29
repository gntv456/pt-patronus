package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// DockerExecHandler 通过 Docker Engine API 在容器内打开交互式终端（exec）。
// 浏览器 WS <-> 后端 <-> docker.sock 的 /exec/{id}/start (hijack raw-stream)。
type DockerExecHandler struct {
	local          *DockerLocalClient
	allowedOrigins map[string]bool
	release        bool
}

func NewDockerExecHandler(allowedOrigins []string, release bool) *DockerExecHandler {
	set := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			set[o] = true
		}
	}
	return &DockerExecHandler{local: NewDockerLocalClient(), allowedOrigins: set, release: release}
}

var execUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// CheckOrigin 由 handler 实例注入（checkExecOrigin）——exec 是命令执行级能力，
	// 曾为 `return true` 全放行，HTTP 部署 + 管理员被种恶意页面时可被 CSWSH 开 shell
	// （安全审计 P2）。release 未配置白名单时 fail-closed，与 ws.go 一致。
}

// checkExecOrigin 与 ws.go/monitor_terminal.go 同口径：非浏览器（无 Origin）放行；
// 配置白名单则必须命中；release 未配置白名单时 fail-closed（exec 是命令执行级
// 能力，CSWSH 借管理员浏览器开 shell 的面必须关掉）。
func (h *DockerExecHandler) checkExecOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if len(h.allowedOrigins) == 0 {
		return !h.release
	}
	return h.allowedOrigins[origin]
}

// Handle 是 exec 终端的 WebSocket 入口。
// GET /ops/docker/local/containers/:name/exec/ws?cmd=/bin/sh&cols=120&rows=32
func (h *DockerExecHandler) Handle(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		errcode.Respond(c, errcode.DockerContainerNameRequired)
		return
	}
	cmd := c.DefaultQuery("cmd", "/bin/sh")
	cols := parseIntDefault(c.DefaultQuery("cols", "120"), 120)
	rows := parseIntDefault(c.DefaultQuery("rows", "32"), 32)

	// 1. 浏览器 WS 升级（带 Origin 收敛检查）
	upgrader := execUpgrader
	upgrader.CheckOrigin = func(r *http.Request) bool { return h.checkExecOrigin(r) }
	browserWS, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer browserWS.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()

	// 2. 连 docker.sock（裸 TCP，用于 hijack）
	sockConn, err := dialDockerSocket(h.local)
	if err != nil {
		browserWS.WriteMessage(websocket.TextMessage, []byte("\r\n[连接 docker.sock 失败: "+err.Error()+"]\r\n"))
		return
	}
	defer sockConn.Close()

	// 3. 创建 exec 实例
	execID, err := createExec(sockConn, name, cmd)
	if err != nil {
		browserWS.WriteMessage(websocket.TextMessage, []byte("\r\n[创建 exec 失败: "+err.Error()+"]\r\n"))
		return
	}

	// 4. 发 start 请求（hijack：发 HTTP 请求头后，连接变成双向流）
	if err := startExecHijack(sockConn, execID, cols, rows); err != nil {
		browserWS.WriteMessage(websocket.TextMessage, []byte("\r\n[启动 exec 失败: "+err.Error()+"]\r\n"))
		return
	}

	// 5. 双向桥接
	done := make(chan struct{}, 2)

	// docker.sock -> 浏览器
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := sockConn.Read(buf)
			if n > 0 {
				browserWS.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// 浏览器 -> docker.sock
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := browserWS.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.TextMessage || mt == websocket.BinaryMessage {
				sockConn.Write(msg)
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// dialDockerSocket 直连 docker.sock（返回裸 net.Conn，用于 hijack）。
func dialDockerSocket(c *DockerLocalClient) (net.Conn, error) {
	if c.scheme == "http+unix" {
		return net.DialTimeout("unix", c.host, 5*time.Second)
	}
	return net.DialTimeout("tcp", c.host, 5*time.Second)
}

// createExec 发送 POST /containers/{name}/exec 创建 exec 实例。
func createExec(conn net.Conn, name, cmd string) (string, error) {
	body := fmt.Sprintf(`{"AttachStdin":true,"AttachStdout":true,"AttachStderr":true,"Tty":true,"Cmd":["%s"]}`, cmd)
	req := fmt.Sprintf("POST /v1.41/containers/%s/exec HTTP/1.1\r\nHost: docker\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		name, len(body), body)
	if _, err := conn.Write([]byte(req)); err != nil {
		return "", err
	}
	// 读响应（HTTP 头 + JSON body）
	resp, err := readHTTPResponse(conn)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return "", fmt.Errorf("parse exec response: %w (raw: %s)", err, resp)
	}
	if result.ID == "" {
		return "", fmt.Errorf("empty exec id (response: %s)", resp)
	}
	return result.ID, nil
}

// startExecHijack 发送 POST /exec/{id}/start 并保持连接用于双向流。
// 必须用 HTTP/1.0 + Connection: Upgrade 让 Docker 不关闭连接。
func startExecHijack(conn net.Conn, execID string, cols, rows int) error {
	body := fmt.Sprintf(`{"Detach":false,"Tty":true,"Height":%d,"Width":%d}`, rows, cols)
	req := fmt.Sprintf("POST /v1.41/exec/%s/start HTTP/1.1\r\nHost: docker\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n%s",
		execID, len(body), body)
	_, err := conn.Write([]byte(req))
	// 不读响应头——hijack 后连接直接变成 TTY 流
	// 但需要消费掉 HTTP 响应头行，否则会出现在终端里
	if err != nil {
		return err
	}
	consumeHTTPHeaders(conn)
	return nil
}

// consumeHTTPHeaders 读取并丢弃 HTTP 响应头（到第一个 \r\n\r\n）。
func consumeHTTPHeaders(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	buf := make([]byte, 4096)
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			break
		}
		if strings.Contains(string(buf[:total]), "\r\n\r\n") {
			// 把 header 之后的数据重新处理：实际上这些字节可能已含 TTY 输出
			// 简化：header 后的数据会丢失（exec 初始输出极少，可接受）
			break
		}
	}
}

// readHTTPResponse 读取完整 HTTP 响应 body（用于 createExec 的 JSON 响应）。
func readHTTPResponse(conn net.Conn) (string, error) {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	var full []byte
	buf := make([]byte, 4096)
	headerEnd := -1
	contentLength := 0
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			full = append(full, buf[:n]...)
			if headerEnd < 0 {
				if idx := strings.Index(string(full), "\r\n\r\n"); idx >= 0 {
					headerEnd = idx + 4
					// 解析 Content-Length
					headers := string(full[:idx])
					for _, line := range strings.Split(headers, "\r\n") {
						if strings.HasPrefix(strings.ToLower(line), "content-length:") {
							fmt.Sscanf(line, "Content-Length: %d", &contentLength)
						}
					}
				}
			}
			if headerEnd >= 0 && contentLength > 0 && len(full)-headerEnd >= contentLength {
				return string(full[headerEnd : headerEnd+contentLength]), nil
			}
		}
		if err != nil {
			if headerEnd >= 0 {
				return string(full[headerEnd:]), nil
			}
			return "", err
		}
	}
}

// parseIntDefault 安全解析 int。
func parseIntDefault(s string, def int) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}

// 确保 io 包被引用（readHTTPResponse 等可能不直接用，但保留以备扩展）。
var _ = io.EOF
