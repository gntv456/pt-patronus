package notification

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

// MQTT 向 MQTT broker 发布一条 QoS0 消息。自实现 MQTT 3.1.1 报文编解码（纯标准库，
// 不引入 paho 依赖）：通知推送只需 CONNECT→CONNACK→PUBLISH→DISCONNECT 一次往返。
// broker 支持 mqtt://host:port / mqtts://host:port / host:port（无 schema=明文 1883）。
type MQTT struct {
	broker   string
	topic    string
	username string
	password string
	clientID string
}

// NewMQTT 构造；username/password 可空（匿名 broker），clientID 空→默认。
func NewMQTT(broker, topic, username, password, clientID string) *MQTT {
	return &MQTT{broker: broker, topic: topic, username: username, password: password, clientID: clientID}
}

func (m *MQTT) Name() string { return "mqtt" }

// Send 建立 TCP/TLS 连接 → CONNECT → PUBLISH(QoS0) → DISCONNECT。
func (m *MQTT) Send(ctx context.Context, msg Message) error {
	if m.broker == "" {
		return fmt.Errorf("mqtt: broker empty")
	}
	if m.topic == "" {
		return fmt.Errorf("mqtt: topic empty")
	}
	host, useTLS, err := parseMQTTBroker(m.broker)
	if err != nil {
		return err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if useTLS {
		c, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("mqtt: tls dial: %w", err)
		}
		conn = c
	} else {
		c, err := dialer.DialContext(ctx, "tcp", host)
		if err != nil {
			return fmt.Errorf("mqtt: dial: %w", err)
		}
		conn = c
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}

	title, body, link := splitContent(msg)
	text := strings.TrimSpace(title + "\n" + body)
	if link != "" {
		text += "\n" + link
	}
	clientID := m.clientID
	if clientID == "" {
		clientID = "ptpatronus-notify"
	}
	br := bufio.NewReader(conn)
	if err := mqttConnect(br, conn, clientID, m.username, m.password); err != nil {
		return err
	}
	if err := mqttPublish(conn, m.topic, text); err != nil {
		return err
	}
	_ = mqttDisconnect(conn)
	return nil
}

// parseMQTTBroker 解析 broker 地址为 host:port + 是否 TLS。
func parseMQTTBroker(s string) (host string, useTLS bool, err error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "://") {
		u, e := url.Parse(s)
		if e != nil {
			return "", false, e
		}
		switch strings.ToLower(u.Scheme) {
		case "mqtt", "tcp":
			useTLS = false
		case "mqtts", "ssl", "tls":
			useTLS = true
		default:
			return "", false, fmt.Errorf("mqtt: unsupported scheme %q", u.Scheme)
		}
		host = u.Host
	} else {
		host = s
	}
	if _, _, perr := net.SplitHostPort(host); perr != nil {
		// 未显式给端口 → 补默认端口
		if useTLS {
			host += ":8883"
		} else {
			host += ":1883"
		}
	}
	return host, useTLS, nil
}

// —— MQTT 3.1.1 报文编解码 ——

// mqttEncodeLen 编码 Remaining Length（base-128 变长）。
func mqttEncodeLen(n int) []byte {
	var out []byte
	for {
		d := n % 128
		n /= 128
		if n > 0 {
			d |= 128
		}
		out = append(out, byte(d))
		if n == 0 {
			break
		}
	}
	return out
}

// mqttWriteString 追加一个 2 字节长度前缀的 UTF-8 字符串（MQTT 字符串编码）。
func mqttWriteString(b *[]byte, s string) {
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(s)))
	*b = append(*b, lb[:]...)
	*b = append(*b, s...)
}

// mqttReadLen 从 bufio 读取变长 Remaining Length（最多 4 字节）。
func mqttReadLen(br *bufio.Reader) (int, error) {
	mult := 1
	value := 0
	for i := 0; i < 4; i++ {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		value += int(b&0x7F) * mult
		if b&0x80 == 0 {
			return value, nil
		}
		mult *= 128
	}
	return 0, fmt.Errorf("mqtt: malformed remaining length")
}

// mqttConnect 发送 CONNECT 并校验 CONNACK 返回码为 0（连接被接受）。
func mqttConnect(br *bufio.Reader, conn net.Conn, clientID, user, pass string) error {
	var vh []byte
	mqttWriteString(&vh, "MQTT") // 协议名
	vh = append(vh, 4)           // 协议级别（3.1.1 = 4）
	var flags byte = 0x02        // Clean Session
	if user != "" {
		flags |= 0x80 // username flag
	}
	if pass != "" {
		flags |= 0x40 // password flag
	}
	vh = append(vh, flags)
	var ka [2]byte
	binary.BigEndian.PutUint16(ka[:], 60) // keep alive 60s
	vh = append(vh, ka[:]...)

	var payload []byte
	mqttWriteString(&payload, clientID)
	if user != "" {
		mqttWriteString(&payload, user)
	}
	if pass != "" {
		mqttWriteString(&payload, pass)
	}
	body := append(vh, payload...)
	pkt := append([]byte{0x10}, mqttEncodeLen(len(body))...) // CONNECT 固定头
	pkt = append(pkt, body...)
	if _, err := conn.Write(pkt); err != nil {
		return fmt.Errorf("mqtt: write connect: %w", err)
	}

	// CONNACK：固定头 0x20 + 变长剩余长度 + 2 字节（ack 标志 + 返回码）。
	typeByte, err := br.ReadByte()
	if err != nil {
		return fmt.Errorf("mqtt: read connack type: %w", err)
	}
	if typeByte != 0x20 {
		return fmt.Errorf("mqtt: expected connack(0x20), got 0x%x", typeByte)
	}
	rl, err := mqttReadLen(br)
	if err != nil {
		return fmt.Errorf("mqtt: read connack len: %w", err)
	}
	if rl < 2 {
		return fmt.Errorf("mqtt: connack payload too short")
	}
	buf := make([]byte, rl)
	if _, err := io.ReadFull(br, buf); err != nil {
		return fmt.Errorf("mqtt: read connack payload: %w", err)
	}
	if rc := buf[1]; rc != 0 {
		return fmt.Errorf("mqtt: connect rejected code %d", rc)
	}
	return nil
}

// mqttPublish 发送 QoS0 PUBLISH（topic + 负载，无包标识）。
func mqttPublish(conn net.Conn, topic, payload string) error {
	var vh []byte
	mqttWriteString(&vh, topic)
	body := append(vh, []byte(payload)...)
	pkt := append([]byte{0x30}, mqttEncodeLen(len(body))...) // PUBLISH QoS0 固定头
	pkt = append(pkt, body...)
	if _, err := conn.Write(pkt); err != nil {
		return fmt.Errorf("mqtt: write publish: %w", err)
	}
	return nil
}

// mqttDisconnect 发送 DISCONNECT（固定头 0xE0 0x00，无负载）。
func mqttDisconnect(conn net.Conn) error {
	_, err := conn.Write([]byte{0xE0, 0x00})
	return err
}
