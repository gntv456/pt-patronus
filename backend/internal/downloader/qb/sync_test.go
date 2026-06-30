package qb

import "testing"

func TestSyncStateFullThenIncremental(t *testing.T) {
	s := newSyncState()

	// 全量
	s.merge(syncResponse{
		Rid:        1,
		FullUpdate: true,
		Torrents: map[string]map[string]any{
			"a": {"name": "A", "size": float64(100), "ratio": float64(1.0)},
			"b": {"name": "B", "size": float64(200)},
		},
		Tags: []string{"music"},
	})
	if s.Rid != 1 || len(s.Torrents) != 2 {
		t.Fatalf("full update state = %+v", s)
	}

	// 增量：A 的 ratio 变化（只含变化字段），新增 c
	s.merge(syncResponse{
		Rid: 2,
		Torrents: map[string]map[string]any{
			"a": {"ratio": float64(2.5)},
			"c": {"name": "C", "size": float64(300)},
		},
	})
	if s.Rid != 2 || len(s.Torrents) != 3 {
		t.Fatalf("incremental state = %+v", s)
	}
	if s.Torrents["a"]["ratio"] != float64(2.5) {
		t.Errorf("A ratio not updated: %v", s.Torrents["a"]["ratio"])
	}
	if s.Torrents["a"]["name"] != "A" {
		t.Errorf("A lost unchanged field name: %v", s.Torrents["a"]["name"])
	}

	// 增量删除 b
	s.merge(syncResponse{
		Rid:             3,
		TorrentsRemoved: []string{"b"},
	})
	if _, exists := s.Torrents["b"]; exists {
		t.Error("b should be removed")
	}
	if len(s.Torrents) != 2 {
		t.Errorf("after remove, len = %d", len(s.Torrents))
	}
}

func TestSyncStateStaleDiscarded(t *testing.T) {
	s := newSyncState()
	s.merge(syncResponse{Rid: 10, FullUpdate: true, Torrents: map[string]map[string]any{"a": {"name": "A"}}})
	// 过期响应（rid 更小）应被丢弃
	s.merge(syncResponse{Rid: 5, Torrents: map[string]map[string]any{"a": {"name": "CHANGED"}}})
	if s.Torrents["a"]["name"] != "A" {
		t.Errorf("stale response should be discarded, got %v", s.Torrents["a"]["name"])
	}
	if s.Rid != 10 {
		t.Errorf("rid = %d, want 10", s.Rid)
	}
}
