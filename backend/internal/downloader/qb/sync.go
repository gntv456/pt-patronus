package qb

// syncResponse 是 qB /api/v2/sync/maindata 的响应（全量或增量）。
type syncResponse struct {
	Rid               int64                     `json:"rid"`
	FullUpdate        bool                      `json:"full_update"`
	Torrents          map[string]map[string]any `json:"torrents"`
	TorrentsRemoved   []string                  `json:"torrents_removed"`
	ServerState       map[string]any            `json:"server_state"`
	Categories        map[string]map[string]any `json:"categories"`
	CategoriesRemoved []string                  `json:"categories_removed"`
	Tags              []string                  `json:"tags"`
	TagsRemoved       []string                  `json:"tags_removed"`
}

// SyncState 是可增量合并的同步状态（每下载器实例一份）。
type SyncState struct {
	Rid         int64
	Torrents    map[string]map[string]any
	ServerState map[string]any
	Categories  map[string]map[string]any
	Tags        []string
}

func newSyncState() *SyncState {
	return &SyncState{
		Torrents:    map[string]map[string]any{},
		ServerState: map[string]any{},
		Categories:  map[string]map[string]any{},
		Tags:        []string{},
	}
}

// merge 把一次响应合并进状态。语义（移植自 qBitController MainData.merge）：
//   - full_update=true：用响应整体替换
//   - 响应 rid < 当前 rid：过期响应，丢弃
//   - 否则：字段级合并增量 + 按 *_removed 删除
func (s *SyncState) merge(r syncResponse) {
	if r.FullUpdate {
		s.Rid = r.Rid
		s.Torrents = nonNilMap2(r.Torrents)
		s.ServerState = nonNilMap1(r.ServerState)
		s.Categories = nonNilMap2(r.Categories)
		s.Tags = r.Tags
		if s.Tags == nil {
			s.Tags = []string{}
		}
		return
	}
	if r.Rid < s.Rid {
		return // 过期
	}
	s.Rid = r.Rid
	for h, fields := range r.Torrents {
		if s.Torrents[h] == nil {
			s.Torrents[h] = map[string]any{}
		}
		for k, v := range fields {
			s.Torrents[h][k] = v
		}
	}
	for _, h := range r.TorrentsRemoved {
		delete(s.Torrents, h)
	}
	for k, v := range r.ServerState {
		s.ServerState[k] = v
	}
	for c, fields := range r.Categories {
		if s.Categories[c] == nil {
			s.Categories[c] = map[string]any{}
		}
		for k, v := range fields {
			s.Categories[c][k] = v
		}
	}
	for _, c := range r.CategoriesRemoved {
		delete(s.Categories, c)
	}
	if r.Tags != nil {
		s.Tags = r.Tags
	}
}

func nonNilMap1(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nonNilMap2(m map[string]map[string]any) map[string]map[string]any {
	if m == nil {
		return map[string]map[string]any{}
	}
	return m
}
