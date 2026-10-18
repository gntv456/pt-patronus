package tgbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// TalkFunc 由 app.go 注入：按 chatKey 取/建会话并跑 agent，返回事件流（tgbot 不触 aiagent/oper）。
// 第二个返回值是「会话都没建起来」这类硬失败：调用方必须走错误出口，不能退回空回复渲染
// （历史缺陷：建会话失败 → 空流 → 回复「（已执行，无文本输出）」，把失败说成成功）。
type TalkFunc func(ctx context.Context, chatKey, text string) (<-chan llm.Event, error)

// ClearFunc 清空某 chatKey 的会话上下文（/clear 用）。
type ClearFunc func(chatKey string)

// Bot 入站 Telegram bot。
type Bot struct {
	token      string
	apiURL     string // 空走官方 api.telegram.org
	client     *http.Client
	allowed    map[int64]bool
	allowGroup bool
	talk       TalkFunc
	clear      ClearFunc
	offset     int64
}

// New 构造。token 为解密后的 BotFather token；talk 必须非 nil；clear 可为 nil。
func New(token, apiURL string, allowedUserIDs []int64, allowGroup bool, talk TalkFunc, clear ClearFunc) *Bot {
	allowed := map[int64]bool{}
	for _, id := range allowedUserIDs {
		allowed[id] = true
	}
	return &Bot{
		token:      token,
		apiURL:     strings.TrimSpace(apiURL),
		client:     &http.Client{Timeout: 65 * time.Second}, // 长轮询 timeout=30，留余量
		allowed:    allowed,
		allowGroup: allowGroup,
		talk:       talk,
		clear:      clear,
	}
}

const helpText = "我是 PTPatronus 助手，可直接用自然语言下指令（搜影视、订阅、整理、查站点…）。\n命令：\n/help - 帮助\n/clear - 清空对话上下文\n\n示例：帮我搜索 Inception 并订阅 1080p。"

// Run 启动长轮询（阻塞，直至 ctx 取消）。
func (b *Bot) Run(ctx context.Context) {
	slog.Info("tgbot: 长轮询启动")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		upds, err := b.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("tgbot: getUpdates 失败，5s 后重试", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		for _, u := range upds {
			b.offset = u.UpdateID + 1
			b.handleUpdate(ctx, u)
		}
	}
}

func (b *Bot) base() string {
	if b.apiURL != "" {
		return strings.TrimRight(b.apiURL, "/")
	}
	return "https://api.telegram.org"
}

// ===== Bot API =====

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
}
type tgMessage struct {
	MessageID int64  `json:"message_id"`
	Chat      tgChat `json:"chat"`
	From      tgUser `json:"from"`
	Text      string `json:"text"`
}
type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // private | group | supergroup
}
type tgUser struct {
	ID int64 `json:"id"`
}
type apiResp struct {
	OK          bool       `json:"ok"`
	Result      []tgUpdate `json:"result"`
	ErrorCode   int        `json:"error_code"`
	Description string     `json:"description"`
}

func (b *Bot) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	u := b.base() + "/bot" + b.token + "/getUpdates?timeout=30&offset=" + strconv.FormatInt(b.offset, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if !r.OK {
		return nil, fmt.Errorf("getUpdates !ok: %d %s", r.ErrorCode, r.Description)
	}
	return r.Result, nil
}

func (b *Bot) sendMessage(chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", text)
	return b.postAPI("/bot"+b.token+"/sendMessage", form)
}

// sendSplit 超长按 4096 分段（Telegram 单条上限）。按 rune 边界切，
// 避免 CJK 等多字节字符被拦腰截断产生非法 UTF-8 半字符。
func (b *Bot) sendSplit(chatID int64, text string) {
	const limit = 4096
	for len(text) > 0 {
		chunk := text
		if len(chunk) > limit {
			chunk = chunk[:limit]
			// 回退到完整 rune 边界：UTF-8 续字节 0b10xxxxxx 属于前一字符
			for len(chunk) > 0 && chunk[len(chunk)-1]&0xC0 == 0x80 {
				chunk = chunk[:len(chunk)-1]
			}
			if len(chunk) == 0 { // 理论不可达（首字节必在 4096 窗口内），防御兜底
				chunk = text[:limit]
			}
		}
		if err := b.sendMessage(chatID, chunk); err != nil {
			slog.Warn("tgbot: sendMessage 失败", "err", err)
			return
		}
		text = text[len(chunk):]
	}
}

func (b *Bot) sendChatAction(chatID int64, action string) {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("action", action)
	_ = b.postAPI("/bot"+b.token+"/sendChatAction", form)
}

func (b *Bot) postAPI(path string, form url.Values) error {
	u := b.base() + path
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tg api %s HTTP %d: %s", path, resp.StatusCode, truncate(string(b), 200))
	}
	return nil
}

// ===== 消息处理 =====

func (b *Bot) handleUpdate(ctx context.Context, u tgUpdate) {
	if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
		return
	}
	uid := u.Message.From.ID
	chatID := u.Message.Chat.ID
	text := u.Message.Text

	// 权限：白名单非空时仅放行白名单用户。
	if len(b.allowed) > 0 && !b.allowed[uid] {
		return
	}
	// 群聊：默认不响应（避免噪音）；仅 AllowGroupChat 时响应。
	if u.Message.Chat.Type != "private" && !b.allowGroup {
		return
	}
	chatKey := "tg:" + strconv.FormatInt(chatID, 10)

	// 斜杠命令
	if strings.HasPrefix(text, "/") {
		cmd := strings.Fields(text)[0]
		switch cmd {
		case "/start", "/help":
			_ = b.sendMessage(chatID, helpText)
			return
		case "/clear":
			if b.clear != nil {
				b.clear(chatKey)
			}
			_ = b.sendMessage(chatID, "已清空对话上下文。")
			return
		}
	}

	// 自然语言 → agent
	b.sendChatAction(chatID, "typing")
	var sb strings.Builder
	stream, err := b.talk(ctx, chatKey, text)
	if err != nil {
		_ = b.sendMessage(chatID, "⚠️ "+err.Error())
		return
	}
	sawEvent := false
	for ev := range stream {
		sawEvent = true
		switch ev.Type {
		case llm.EventText:
			sb.WriteString(ev.Text)
		case llm.EventError:
			_ = b.sendMessage(chatID, "⚠️ "+ev.Error)
			return
		}
	}
	reply := strings.TrimSpace(sb.String())
	if reply == "" {
		if sawEvent {
			reply = "（已执行，无文本输出）"
		} else {
			// 零事件 = agent 侧在产出前就崩了（safego 静默 recover），不能渲染成「已执行」。
			reply = "⚠️ 助手没有返回任何结果，请重试"
		}
	}
	b.sendSplit(chatID, reply)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
