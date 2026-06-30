package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	ticketTTL        = 30 * time.Second // ticket 有效期：握手短时
	ticketSweepEvery = 60 * time.Second // 惰性清扫过期项的间隔
)

// TicketStore 是 WS/RTSP 的一次性短时 ticket 存储（单实例，与 TokenVersionCache 同约束；多副本需上 Redis）。
//
// 流程：客户端先用 header-JWT 调 /auth/ws-ticket（guarded 内签发，含 TokenVersion 校验）换 ticket，
// 再用 ?ticket= 连 WS/RTSP——JWT 不再落 URL。ticket 单次消费（Consume 删除）、ticketTTL 过期。
type TicketStore struct {
	mu        sync.Mutex
	tickets   map[string]ticket
	ttl       time.Duration
	lastSweep time.Time
}

type ticket struct {
	uid uint
	exp time.Time
}

// NewTicketStore 构造（默认 ticketTTL）。
func NewTicketStore() *TicketStore {
	return &TicketStore{tickets: map[string]ticket{}, ttl: ticketTTL, lastSweep: time.Now()}
}

// Issue 为 uid 签发一个一次性 ticket。
func (s *TicketStore) Issue(uid uint) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	tk := base64.RawURLEncoding.EncodeToString(b)
	exp := time.Now().Add(s.ttl)
	s.mu.Lock()
	s.tickets[tk] = ticket{uid: uid, exp: exp}
	s.sweepLocked()
	s.mu.Unlock()
	return tk, exp, nil
}

// Consume 校验并消费（删除）一个 ticket；返回 uid 与是否有效。单次有效。
func (s *TicketStore) Consume(tk string) (uint, bool) {
	if tk == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.tickets[tk]
	if !ok {
		return 0, false
	}
	delete(s.tickets, tk)
	s.sweepLocked()
	if time.Now().After(e.exp) {
		return 0, false
	}
	return e.uid, true
}

// sweepLocked 惰性清扫过期项（调用方持锁）。在 Issue/Consume 时顺带触发，无需独立定时器。
func (s *TicketStore) sweepLocked() {
	if time.Since(s.lastSweep) < ticketSweepEvery {
		return
	}
	s.lastSweep = time.Now()
	now := time.Now()
	for k, e := range s.tickets {
		if now.After(e.exp) {
			delete(s.tickets, k)
		}
	}
}
