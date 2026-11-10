// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	ticketTTL        = 30 * time.Second
	ticketSweepEvery = 60 * time.Second
)

// TicketStore stores short-lived auth tickets.
//
// Single-use tickets are used for websocket and terminal handshakes where the
// token should be consumed exactly once. Scoped tickets are reusable for a
// short time and are suitable for related asset fetches that need multiple
// requests within one session.
type TicketStore struct {
	mu        sync.Mutex
	tickets   map[string]ticket
	ttl       time.Duration
	lastSweep time.Time
}

type ticket struct {
	uid       uint
	exp       time.Time
	scope     string
	singleUse bool
}

func NewTicketStore() *TicketStore {
	return &TicketStore{tickets: map[string]ticket{}, ttl: ticketTTL, lastSweep: time.Now()}
}

// Issue creates a signed single-use ticket for the given user id.
func (s *TicketStore) Issue(uid uint) (string, time.Time, error) {
	return s.issue(uid, "", s.ttl, true)
}

// IssueScoped creates a reusable short-lived ticket for the given scope.
func (s *TicketStore) IssueScoped(uid uint, scope string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = s.ttl
	}
	return s.issue(uid, scope, ttl, false)
}

func (s *TicketStore) issue(uid uint, scope string, ttl time.Duration, singleUse bool) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	tk := base64.RawURLEncoding.EncodeToString(b)
	exp := time.Now().Add(ttl)
	s.mu.Lock()
	s.tickets[tk] = ticket{uid: uid, exp: exp, scope: scope, singleUse: singleUse}
	s.sweepLocked()
	s.mu.Unlock()
	return tk, exp, nil
}

// Consume validates and consumes a single-use ticket.
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

// Validate checks a reusable ticket without consuming it. If scope is
// provided it must match the ticket scope exactly.
func (s *TicketStore) Validate(tk, scope string) (uint, bool) {
	if tk == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.tickets[tk]
	if !ok {
		return 0, false
	}
	s.sweepLocked()
	if time.Now().After(e.exp) {
		delete(s.tickets, tk)
		return 0, false
	}
	if scope != "" && e.scope != scope {
		return 0, false
	}
	if e.singleUse {
		return 0, false
	}
	return e.uid, true
}

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
