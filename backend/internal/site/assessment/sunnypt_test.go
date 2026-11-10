// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package assessment

import "testing"

func TestParseSunnyptAssessmentsEmpty(t *testing.T) {
	body := []byte(`{"code":0,"data":{"list":[],"total":0},"msg":"ok"}`)
	tasks := mustParseSunnypt(t, body)
	if len(tasks) != 0 {
		t.Fatalf("want 0, got %d", len(tasks))
	}
}

func TestParseSunnyptAssessmentsActive(t *testing.T) {
	body := []byte(`{"code":0,"data":{"list":[{"name":"新手考核","description":"完成指标","status":"in_progress","success_reward_bonus":100,"metrics":{"uploaded_bytes":{"value":1000,"target":5000,"completed":false},"seed_time_seconds":{"value":3600,"target":3600,"completed":true}}}]},"msg":"ok"}`)
	tasks := mustParseSunnypt(t, body)
	if len(tasks) != 1 {
		t.Fatalf("want 1, got %d", len(tasks))
	}
	if tasks[0].Name != "新手考核" {
		t.Errorf("name=%q", tasks[0].Name)
	}
	if tasks[0].Status != StatusActive {
		t.Errorf("status=%s", tasks[0].Status)
	}
	if len(tasks[0].Metrics) != 2 {
		t.Fatalf("metrics=%d", len(tasks[0].Metrics))
	}
}
