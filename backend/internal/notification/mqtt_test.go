package notification

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// mqttTestBroker 是一个最小 MQTT 服务端：收 CONNECT → 回 CONNACK(returnCode) → 收 PUBLISH → 抓 topic/payload。
type mqttTestBroker struct {
	ln         net.Listener
	topic      string
	payload    string
	gotConn    bool
	returnCode byte          // CONNACK 返回码（0=接受；5=未授权）
	done       chan struct{} // serve 处理完（连接关闭）后 close，供测试等待避免竞态
}

func newServingBroker(t *testing.T, returnCode byte) *mqttTestBroker {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	b := &mqttTestBroker{ln: ln, returnCode: returnCode, done: make(chan struct{})}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			close(b.done)
			return
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		b.serve(conn)
		close(b.done)
	}()
	return b
}

func (b *mqttTestBroker) addr() string { return b.ln.Addr().String() }
func (b *mqttTestBroker) close()       { b.ln.Close() }

// wait 等待 serve goroutine 处理完毕（最多 2s），避免 Send 返回后立即断言的竞态。
func (b *mqttTestBroker) wait(t *testing.T) {
	t.Helper()
	select {
	case <-b.done:
	case <-time.After(2 * time.Second):
		t.Fatal("broker did not finish in time")
	}
}

func (b *mqttTestBroker) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	// 读 CONNECT 固定头（0x10 + 变长剩余长度）+ body，丢弃。
	typ, err := br.ReadByte()
	if err != nil || typ != 0x10 {
		return
	}
	rl, err := mqttReadLen(br)
	if err != nil {
		return
	}
	if _, err := io.CopyN(io.Discard, br, int64(rl)); err != nil {
		return
	}
	b.gotConn = true
	// 回 CONNACK：0x20 0x02 <ack flags> <return code>。
	if _, err := conn.Write([]byte{0x20, 0x02, 0x00, b.returnCode}); err != nil {
		return
	}
	if b.returnCode != 0 {
		return // 被拒，客户端不发 PUBLISH
	}
	// 读 PUBLISH（QoS0：0x30 + 变长剩余长度 + topic + payload）。
	typ, err = br.ReadByte()
	if err != nil || typ != 0x30 {
		return
	}
	rl, err = mqttReadLen(br)
	if err != nil {
		return
	}
	body := make([]byte, rl)
	if _, err := io.ReadFull(br, body); err != nil {
		return
	}
	if len(body) < 2 {
		return
	}
	tlen := int(binary.BigEndian.Uint16(body[:2]))
	if 2+tlen > len(body) {
		return
	}
	b.topic = string(body[2 : 2+tlen])
	b.payload = string(body[2+tlen:])
}

// TestMQTTSend 验证 QoS0 发布：broker 收到正确 topic + 负载（标题+正文+链接）。
func TestMQTTSend(t *testing.T) {
	broker := newServingBroker(t, 0)
	defer broker.close()

	m := NewMQTT("mqtt://"+broker.addr(), "ptp/notify", "user", "pass", "client-1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Send(ctx, msgOf("Title", "Body", "https://x")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	broker.wait(t)
	if !broker.gotConn {
		t.Error("broker did not receive CONNECT")
	}
	if broker.topic != "ptp/notify" {
		t.Errorf("topic = %q, want ptp/notify", broker.topic)
	}
	if !strings.Contains(broker.payload, "Title") || !strings.Contains(broker.payload, "Body") {
		t.Errorf("payload missing title/body; got %q", broker.payload)
	}
	if !strings.Contains(broker.payload, "https://x") {
		t.Errorf("payload missing link; got %q", broker.payload)
	}
}

// TestMQTTAnonymous 验证无用户名密码（匿名 broker）也能发布。
func TestMQTTAnonymous(t *testing.T) {
	broker := newServingBroker(t, 0)
	defer broker.close()
	m := NewMQTT(broker.addr(), "topic", "", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Send(ctx, msgOf("T", "B", "")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	broker.wait(t)
	if broker.topic != "topic" {
		t.Errorf("topic = %q, want topic", broker.topic)
	}
}

// TestMQTTAuthRejected 验证 broker 返回非 0 返回码时 Send 报错。
func TestMQTTAuthRejected(t *testing.T) {
	broker := newServingBroker(t, 5) // 5 = 未授权
	defer broker.close()
	m := NewMQTT(broker.addr(), "t", "u", "p", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Send(ctx, msgOf("a", "b", "")); err == nil {
		t.Error("expected error on rejected connect")
	}
}

// TestMQTTRequired 验证 broker/topic 为空时报错。
func TestMQTTRequired(t *testing.T) {
	ctx := context.Background()
	if err := NewMQTT("", "t", "", "", "").Send(ctx, msgOf("a", "b", "")); err == nil {
		t.Error("empty broker expected error")
	}
	if err := NewMQTT("h:1883", "", "", "", "").Send(ctx, msgOf("a", "b", "")); err == nil {
		t.Error("empty topic expected error")
	}
}

// TestMQTTParseBroker 验证 broker 地址解析（scheme→TLS、默认端口）。
func TestMQTTParseBroker(t *testing.T) {
	cases := []struct {
		in   string
		host string
		tls  bool
	}{
		{"mqtt://h:1883", "h:1883", false},
		{"mqtts://h:8883", "h:8883", true},
		{"mqtt://h", "h:1883", false}, // 补默认端口
		{"mqtts://h", "h:8883", true},
		{"h:1883", "h:1883", false},
		{"h", "h:1883", false},
	}
	for _, c := range cases {
		host, tls, err := parseMQTTBroker(c.in)
		if err != nil {
			t.Errorf("%s: err %v", c.in, err)
			continue
		}
		if host != c.host || tls != c.tls {
			t.Errorf("%s: got host=%q tls=%v, want host=%q tls=%v", c.in, host, tls, c.host, c.tls)
		}
	}
	if _, _, err := parseMQTTBroker("http://h"); err == nil {
		t.Error("expected error for unsupported scheme")
	}
}
