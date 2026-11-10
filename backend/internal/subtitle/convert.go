// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package subtitle

import (
	"sync"

	"github.com/longbridgeapp/opencc"
)

// 繁简转换用 OpenCC（纯 Go，内嵌字典）。两个 profile 各懒加载一个实例（New 会加载字典，
// 重复构造昂贵；Convert 读字典并发安全）。用互斥保护懒加载而非 sync.Once——前者在首次
// 加载失败时下次调用会重试，后者会把首次错误永久缓存（内嵌字典偶发 FS 抖动致繁简转换永久坏）。
var (
	t2sMu   sync.Mutex
	t2sInst *opencc.OpenCC

	s2tMu   sync.Mutex
	s2tInst *opencc.OpenCC
)

func t2s() (*opencc.OpenCC, error) {
	t2sMu.Lock()
	defer t2sMu.Unlock()
	if t2sInst != nil {
		return t2sInst, nil
	}
	inst, err := opencc.New("t2s")
	if err != nil {
		return nil, err
	}
	t2sInst = inst
	return t2sInst, nil
}

func s2t() (*opencc.OpenCC, error) {
	s2tMu.Lock()
	defer s2tMu.Unlock()
	if s2tInst != nil {
		return s2tInst, nil
	}
	inst, err := opencc.New("s2t")
	if err != nil {
		return nil, err
	}
	s2tInst = inst
	return s2tInst, nil
}

// Convert 繁简转换。target="chs"→繁转简，"cht"→简转繁，""/其他→原样（不转换）。
// 仅对文本字幕有意义（图片字幕 .sup/.sub+idx 不转）。
func Convert(text, target string) (string, error) {
	switch target {
	case "chs":
		cc, err := t2s()
		if err != nil {
			return text, err
		}
		return cc.Convert(text)
	case "cht":
		cc, err := s2t()
		if err != nil {
			return text, err
		}
		return cc.Convert(text)
	}
	return text, nil
}
