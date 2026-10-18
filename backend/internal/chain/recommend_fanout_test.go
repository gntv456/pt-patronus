package chain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/media"
)

// TestRecommendPartialFanoutIsNotCached 有源报错时不得把部分结果钉进 30min 内存缓存。
// 修复前 fetchRecommend 的 complete 只看「预算有没有砍」，报错的源不算不完整 ⇒
// 缺席的那一源整半小时都不出现；流式面早就有 len(sum.Failed)==0 的规矩，两面在此漂移。
func TestRecommendPartialFanoutIsNotCached(t *testing.T) {
	ok := &fakeSrc{name: "ok", items: []media.MediaInfo{{Title: "A"}}}
	bad := &errSrc{name: "bad"}
	rc := NewRecommendChain(NewBase(module.New(), event.New()), &MediaSourceProvider{sources: []media.MediaSource{ok, bad}})

	rc.RecommendFeeds(context.Background(), "", "movie", 1, true)
	if ok.calls != 1 {
		t.Fatalf("前提：第一轮各源应各调一次，got ok.calls=%d", ok.calls)
	}
	rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if ok.calls != 2 {
		t.Errorf("有源失败时不得缓存部分结果（第二轮必须重新扇出），got ok.calls=%d", ok.calls)
	}
}

// TestRecommendStreamCountsPanickingSource 崩掉的源必须进 done 帧的 Failed 账面：
// 否则「2 个源、0 个失败」与「少了一源的内容」同时成立，操作者无从发现。
func TestRecommendStreamCountsPanickingSource(t *testing.T) {
	ok := &fakeSrc{name: "ok", items: []media.MediaInfo{{Title: "A"}}}
	boom := &fakeSrc{name: "boom", panicTrending: true}
	rc := NewRecommendChain(NewBase(module.New(), event.New()), &MediaSourceProvider{sources: []media.MediaSource{ok, boom}})

	var mu sync.Mutex
	var batches [][]media.MediaInfo
	sum := rc.RecommendFeedsStream(context.Background(), "", "movie", 1, true, func(items []media.MediaInfo) {
		mu.Lock()
		batches = append(batches, items)
		mu.Unlock()
	})
	if sum.Sources != 2 {
		t.Fatalf("参与源数应为 2，got %+v", sum)
	}
	if len(sum.Failed) != 1 || !strings.Contains(sum.Failed[0], "boom") || !strings.Contains(sum.Failed[0], "malformed trending response") {
		t.Errorf("崩源要带源名与 panic 载荷进 Failed，got %+v", sum.Failed)
	}
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Errorf("正常源的结果仍要推出，got %d 批 %+v", len(batches), batches)
	}
}

// TestRecommendFanoutPanickingSourceIsNotCached JSON 面同一轴：崩掉的源既不留内容也不算「完整」，
// 部分结果不得进缓存（否则这一小时的推荐流永远缺那一源）。
func TestRecommendFanoutPanickingSourceIsNotCached(t *testing.T) {
	ok := &fakeSrc{name: "ok", items: []media.MediaInfo{{Title: "A"}}}
	boom := &fakeSrc{name: "boom", panicTrending: true}
	rc := NewRecommendChain(NewBase(module.New(), event.New()), &MediaSourceProvider{sources: []media.MediaSource{ok, boom}})

	res := rc.RecommendFeeds(context.Background(), "", "movie", 1, true)
	if len(res) != 1 || ok.calls != 1 {
		t.Fatalf("前提：正常源应出 1 条且只调一次，got res=%d calls=%d", len(res), ok.calls)
	}
	rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if ok.calls != 2 {
		t.Errorf("有源崩溃时不得缓存部分结果，got ok.calls=%d", ok.calls)
	}
}
