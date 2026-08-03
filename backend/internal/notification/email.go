package notification

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

type Email struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
}

func NewEmail(host string, port int, username, password, from, to string) *Email {
	if port <= 0 {
		port = 25
	}
	recipients := make([]string, 0)
	for _, item := range strings.Split(to, ",") {
		if s := strings.TrimSpace(item); s != "" {
			recipients = append(recipients, s)
		}
	}
	return &Email{host: host, port: port, username: username, password: password, from: from, to: recipients}
}

func (e *Email) Name() string { return "email" }

func (e *Email) Send(ctx context.Context, msg Message) error {
	if e.host == "" || e.from == "" || len(e.to) == 0 {
		return fmt.Errorf("email: host/from/to required")
	}
	addr := net.JoinHostPort(e.host, strconv.Itoa(e.port))
	title, body, link := splitContent(msg)
	// 防邮件头注入：title(Subject) 含 CRLF 可注入额外头（如 Bcc 外泄），清洗为空格。
	title = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(title)
	content := body
	if link != "" {
		content += "\n\n" + link
	}
	raw := strings.Join([]string{
		"From: " + e.from,
		"To: " + strings.Join(e.to, ", "),
		"Subject: " + title,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		content,
	}, "\r\n")
	var auth smtp.Auth
	if e.username != "" || e.password != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.host)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- smtp.SendMail(addr, auth, e.from, e.to, []byte(raw)) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// ctx 取消后 SendMail goroutine 仍在阻塞拨号/握手，无法中断（net/smtp 不吃 ctx）。
		// 这里只把 ctx.Err 返给调用方；goroutine 在 SMTP 拨号超时（Dialer 无显式 timeout 时
		// 走 OS 默认）后自然退出并写入已 buffer 的 errCh，errCh 被 GC 回收，不泄漏。
		return ctx.Err()
	}
}
