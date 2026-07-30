package ws

import (
	"time"

	"github.com/gorilla/websocket"
)

// 读写/心跳时序常量（pingPeriod 必须 < pongWait）。
const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	sendBuf    = 32
)

// Client 是单个 WS 连接的读写句柄。
type Client struct {
	conn *websocket.Conn
	send chan []byte
	inc  bool // 增量协议客户端（收 torrents_diff；否则收全量 torrents/torrents_full）
}

func newClient(conn *websocket.Conn, inc bool) *Client {
	return &Client{conn: conn, send: make(chan []byte, sendBuf), inc: inc}
}

// readPump 持续读客户端消息。本服务为「服务端推送」模型，客户端不发业务消息，
// 此循环主要为检测断开 + 在 pong 处理器里续读超时。
func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.unregister(c)
		c.conn.Close()
	}()
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// writePump 消费 send 通道写广播消息，并按 pingPeriod 发心跳。
// send 被关闭（unregister）或写失败时退出并关连接。
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
