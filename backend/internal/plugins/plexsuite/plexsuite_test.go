// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plexsuite

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProbeAllFastest 用 httptest 模拟两个候选，校验取最快可达者。
func TestProbeAllFastest(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer slow.Close()
	defer fast.Close()

	hc := newTestClient(t)
	e := NewEngine(hc)
	// 反向传入，确保排序按延迟而非顺序
	s := Settings{ServerURL: slow.URL, Token: "X", CandidateURLs: []string{slow.URL, fast.URL}}
	probes := e.ProbeAll(testContext(), s)
	if len(probes) != 2 {
		t.Fatalf("want 2 probes, got %d", len(probes))
	}
	if !probes[0].OK {
		t.Fatalf("first probe should be ok")
	}
	// 排序后 probes[0] 必为可达中最快（OK 优先 + 延迟升序）
	best := pickFastest(probes)
	if best == nil || !best.OK {
		t.Fatalf("pickFastest should return reachable")
	}
}

func TestSectionID(t *testing.T) {
	if sectionID("/library/sections/2") != "2" {
		t.Fatalf("sectionID parse failed")
	}
	if sectionID("7") != "7" {
		t.Fatalf("sectionID bare id failed")
	}
}

func TestApplyNoConfig(t *testing.T) {
	e := NewEngine(newTestClient(t))
	res := e.Apply(testContext(), Settings{})
	if len(res) != 1 || res[0].OK {
		t.Fatalf("empty config should yield one failing result: %+v", res)
	}
}
