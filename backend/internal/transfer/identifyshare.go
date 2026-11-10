// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ---- 识别词云端共享（对标 MSaber「本地自定义 + 云端协作共享」） ----
//
// 模型：一个或多个「共享仓库」（JSON URL，如 GitHub raw / jsDelivr / 自建 gist）。
// 仓库格式：{"words":[{"kind":"replace","from":"...","to":"...","note":"..."}]}。
// 同步 = 拉取仓库合并进本地词表（from+kind 去重，本地优先），一键导入，不做自动推送
//（避免用户词表被远端静默覆盖；与 MSaber 云端协作同只读语义）。

const identifyRepoKey = "transfer.identify_words_repos"

// SharedRepo 一个共享词库仓库。
type SharedRepo struct {
	Name string `json:"name"`
	URL  string `json:"url"` // 返回 {"words":[...]} 的 JSON 地址
}

// SharedRepoWords 仓库响应。
type SharedRepoWords struct {
	Words []IdentifyWord `json:"words"`
}

// LoadSharedRepos 已配置的共享仓库列表。
func LoadSharedRepos(kv KVStore) []SharedRepo {
	if kv == nil {
		return nil
	}
	raw, err := kv.Get(identifyRepoKey)
	if err != nil || raw == "" {
		return nil
	}
	var rs []SharedRepo
	if json.Unmarshal([]byte(raw), &rs) != nil {
		return nil
	}
	return rs
}

// SaveSharedRepos 保存共享仓库列表。
func SaveSharedRepos(kv KVStore, rs []SharedRepo) error {
	b, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return kv.Set(identifyRepoKey, string(b))
}

// FetchSharedRepo 拉取单个仓库词表（8s 超时；2MB 上限防滥用）。
func FetchSharedRepo(ctx context.Context, repoURL string) ([]IdentifyWord, error) {
	if repoURL == "" {
		return nil, fmt.Errorf("仓库地址为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repoURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("仓库 HTTP %d", resp.StatusCode)
	}
	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if len(body) > 2<<20 {
			return nil, fmt.Errorf("仓库超过 2MB 上限")
		}
		if rerr != nil {
			break
		}
	}
	var out SharedRepoWords
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("仓库格式错误: %v", err)
	}
	// 只收合法 kind（replace/trim），防仓库注入垃圾项。
	valid := make([]IdentifyWord, 0, len(out.Words))
	for _, w := range out.Words {
		if (w.Kind == "replace" || w.Kind == "trim") && w.From != "" {
			valid = append(valid, w)
		}
	}
	return valid, nil
}

// MergeWords 合并词表：按 kind+from 去重，existing 优先（云端不覆盖本地自定义）。
// 返回合并后的新表与新增条数。
func MergeWords(existing, incoming []IdentifyWord) ([]IdentifyWord, int) {
	seen := make(map[string]bool, len(existing))
	for _, w := range existing {
		seen[w.Kind+"\x00"+w.From] = true
	}
	out := make([]IdentifyWord, len(existing))
	copy(out, existing)
	added := 0
	for _, w := range incoming {
		key := w.Kind + "\x00" + w.From
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, w)
		added++
	}
	return out, added
}
