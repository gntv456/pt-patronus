package v1

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

type MonitorTerminalHandler struct {
	svc            *monitor.Service
	tickets        *auth.TicketStore
	db             *gorm.DB
	allowedOrigins map[string]bool
}

func NewMonitorTerminalHandler(svc *monitor.Service, tickets *auth.TicketStore, db *gorm.DB, allowedOrigins []string) *MonitorTerminalHandler {
	set := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			set[o] = true
		}
	}
	return &MonitorTerminalHandler{svc: svc, tickets: tickets, db: db, allowedOrigins: set}
}

func (h *MonitorTerminalHandler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 非浏览器客户端无 Origin，不适用 CSWSH
	}
	if len(h.allowedOrigins) == 0 {
		// 未配置白名单：debug 保持旧行为（全放行）；release fail-closed——
		// 与 ws.go checkOrigin 一致（终端是命令执行级能力，不能默认敞开）。
		return gin.Mode() != gin.ReleaseMode
	}
	return h.allowedOrigins[origin]
}

func (h *MonitorTerminalHandler) Handle(c *gin.Context) {
	uid, ok := h.tickets.Consume(c.Query("ticket"))
	if !ok {
		errcode.Respond(c, errcode.TicketInvalidOrExpired)
		return
	}
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil || !user.IsAdmin {
		errcode.Respond(c, errcode.UnauthorizedOp)
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     h.checkOrigin,
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	io := &terminalWSIO{conn: conn}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	cols, _ := strconv.Atoi(c.DefaultQuery("cols", "120"))
	rows, _ := strconv.Atoi(c.DefaultQuery("rows", "32"))
	err = h.svc.RunTerminalNow(ctx, uint(id64), io, io, cols, rows)
	if err != nil && err != context.Canceled && err != io.closedErr {
		_, _ = io.Write([]byte(fmt.Sprintf("\r\n[终端已断开：%v]\r\n", err)))
	}
}

type terminalWSIO struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	readBuf   []byte
	closedErr error
}

func (w *terminalWSIO) Read(p []byte) (int, error) {
	for len(w.readBuf) == 0 {
		mt, msg, err := w.conn.ReadMessage()
		if err != nil {
			w.closedErr = err
			return 0, err
		}
		if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
			continue
		}
		w.readBuf = msg
	}
	n := copy(p, w.readBuf)
	w.readBuf = w.readBuf[n:]
	return n, nil
}

func (w *terminalWSIO) Write(p []byte) (int, error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if err := w.conn.WriteMessage(websocket.TextMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

var _ io.ReadWriter = (*terminalWSIO)(nil)
