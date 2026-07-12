// Package media 实现影视/音乐元数据匹配：MediaSource 抽象 + TMDB/豆瓣/MusicBrainz
// 多源 + 海报缓存 + NFO 生成。移植现有 PTPatronus RecommendService/TmdbSearch 到 Go。
package media

import (
	"context"
	"errors"
)

// ErrNotConfigured 表示该源未配置（如缺 API key），Chain 会跳过它。
var ErrNotConfigured = errors.New("media source not configured")

// MediaType 媒体类型。
type MediaType string

const (
	TypeMovie MediaType = "movie"
	TypeTV    MediaType = "tv"
	TypeMusic MediaType = "music"
)

// CreditPerson 是演职人员（演员/导演等）。
type CreditPerson struct {
	Name   string `json:"name" binding:"max=128"`
	Role   string `json:"role,omitempty" binding:"max=128"`   // 角色名 / 职务
	Avatar string `json:"avatar,omitempty" binding:"max=512"` // 头像 URL
}

// MediaInfo 是统一的元数据。Search 返回基础字段；Detail 额外填充演职/推荐等富字段。
// 注：binding max 仅供 NFO 端点（nfoReq.ShouldBindJSON）防超长输入；不影响 Marshal/Unmarshal。
type MediaInfo struct {
	Source        string    `json:"source" binding:"max=32"` // tmdb | douban | musicbrainz
	SourceID      string    `json:"source_id" binding:"max=64"`
	Title         string    `json:"title" binding:"max=512"`
	OriginalTitle string    `json:"original_title,omitempty" binding:"max=512"`
	Year          int       `json:"year"`
	Type          MediaType `json:"type" binding:"max=16"`
	Overview      string    `json:"overview,omitempty" binding:"max=10000"`
	PosterURL     string    `json:"poster_url" binding:"max=512"`
	BackdropURL   string    `json:"backdrop_url,omitempty" binding:"max=512"`
	ReleaseDate   string    `json:"release_date,omitempty" binding:"max=32"`
	Rating        float64   `json:"rating,omitempty"`
	Genres        []string  `json:"genres,omitempty" binding:"dive,max=64"`

	// 富字段（Detail 填充，Search 不填）：
	Runtime         int            `json:"runtime,omitempty"` // 分钟
	Countries       []string       `json:"countries,omitempty" binding:"dive,max=64"`
	Directors       []string       `json:"directors,omitempty" binding:"dive,max=64"`
	Cast            []CreditPerson `json:"cast,omitempty" binding:"dive"`
	Recommendations []MediaInfo    `json:"recommendations,omitempty"`
}

// TypeMatches 报告媒体类型 t 是否满足请求的 filter：
//   - ""      全部类型（MatchPoster 等内部调用用，保留音乐海报匹配）
//   - "video" 影视：movie + tv（不含 music）——「影视」搜索默认值
//   - "movie"/"tv"/"music" 精确匹配
//
// 未知 filter 不过滤（保守放行）。
func TypeMatches(t MediaType, filter string) bool {
	switch filter {
	case "":
		return true
	case "video":
		return t == TypeMovie || t == TypeTV
	case "movie":
		return t == TypeMovie
	case "tv":
		return t == TypeTV
	case "music":
		return t == TypeMusic
	}
	return true
}

// TypedSource 可选能力：声明该源产出哪些 MediaType。Chain 用它在过滤口径不含
// 某源产物时直接跳过该源（如「影视」搜 movie/tv 时跳过只产音乐的 MusicBrainz——
// 既省一次必定无用、且常很慢的海外请求，又避免拖慢并发收尾）。
type TypedSource interface {
	Types() []MediaType
}

// SourceMatches 报告一个源（按其 Types()）是否有结果能落入 filter：
// filter="" 全放行；否则要求该源至少一种类型被 TypeMatches 接受。
func SourceMatches(types []MediaType, filter string) bool {
	if filter == "" {
		return true
	}
	for _, t := range types {
		if TypeMatches(t, filter) {
			return true
		}
	}
	return false
}

// CategoryCapable 是可选能力：声明该源 Trending 支持哪些 category。RecommendChain 用它在
// 某分类下跳过不支持该分类的源——例如「短剧」分类只由星光集响应，豆瓣/TMDB/Bangumi 不会
// 各自回退默认热映污染结果；亦修复「anime 让豆瓣回退热映」之类既有回退污染。
// 未实现该接口的源视为支持全部 category（向后兼容）。
type CategoryCapable interface {
	Categories() []string
}

// MediaSource 是一个元数据源。
type MediaSource interface {
	Name() string
	// Search 按关键字（可选年份）搜索。
	Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error)
	// Detail 按 source 内部 id 取详情。
	Detail(ctx context.Context, id string) (*MediaInfo, error)
}

// TrendingCapable 是可选「热门/趋势」能力。实现该接口的源可被 RecommendChain 用来
// 生成推荐信息流（豆瓣热映 / TMDB trending / Bangumi 热门）。category 为自由串
// （movie|tv|anime|coming|popular|calendar），各源自行解释；page 从 1 起。
// 与 *Capable 范式一致：不支持热门的源（如 MusicBrainz）不实现此接口，Chain 跳过。
type TrendingCapable interface {
	Trending(ctx context.Context, category string, page int) ([]MediaInfo, error)
}
