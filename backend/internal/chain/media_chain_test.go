package chain

import (
	"context"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/media"
)

// mediaFake 测试用媒体源：按 delay 睡眠后返回 items（模拟慢/快源），实现 TypedSource。
type mediaFake struct {
	name    string
	delay   time.Duration
	items   []media.MediaInfo
	types   []media.MediaType
	called  bool
}

func (f *mediaFake) Name() string             { return f.name }
func (f *mediaFake) Types() []media.MediaType { return f.types }
func (f *mediaFake) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }
func (f *mediaFake) Search(ctx context.Context, _ string, _ int) ([]media.MediaInfo, error) {
	f.called = true
	select {
	case <-time.After(f.delay):
		return f.items, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// 证明「并发 + 预算」：慢源(5s)被 context 在预算内砍掉，快源(20ms)正常返回，
// 总时长 ≈ 预算而非各源累加。这是修「串行跑出 31s 触发前端 15s 超时」的关键回归保护。
func TestMediaChainSearchConcurrentBudget(t *testing.T) {
	fast := &mediaFake{name: "fast", delay: 20 * time.Millisecond, types: []media.MediaType{media.TypeMovie},
		items: []media.MediaInfo{{Source: "fast", Title: "A", Type: media.TypeMovie}}}
	slow := &mediaFake{name: "slow", delay: 5 * time.Second, types: []media.MediaType{media.TypeMovie},
		items: []media.MediaInfo{{Source: "slow", Title: "B", Type: media.TypeMovie}}}
	c := NewMediaChain(Base{}, &MediaSourceProvider{sources: []media.MediaSource{fast, slow}}, nil) // nil cache

	// 父 ctx 给 300ms 短预算；Search 内部再套 8s → 有效 deadline = 300ms。
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := c.Search(ctx, "kw", 0, "video")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("并发+预算应 ~300ms 而非 5s 累加，实际 %v", elapsed)
	}
	if len(res) != 1 || res[0].Title != "A" {
		t.Errorf("慢源被砍、应只含快源结果，got %+v", res)
	}
}

// 证明按 TypedSource 跳过：video 搜时只产音乐的源（MusicBrainz）不被调用，结果为空。
func TestMediaChainSearchSkipsMusicForVideo(t *testing.T) {
	mb := &mediaFake{name: "mb", types: []media.MediaType{media.TypeMusic},
		items: []media.MediaInfo{{Source: "mb", Title: "song", Type: media.TypeMusic}}}
	c := NewMediaChain(Base{}, &MediaSourceProvider{sources: []media.MediaSource{mb}}, nil)

	res, err := c.Search(context.Background(), "kw", 0, "video")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if mb.called {
		t.Error("只产音乐的源在 video 搜时不应被调用")
	}
	if len(res) != 0 {
		t.Errorf("video 搜跳过音乐源后应无结果，got %+v", res)
	}
}
