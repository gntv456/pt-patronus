package v1

import (
	"context"
	"crypto/subtle"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/media"
)

// Servarr 模型转换与词条解析（从 servarr.go 拆出：该文件触行数门禁，转换层自成一体）。

// --- 模型转换 ---

type radarrMovie struct {
	ID               uint   `json:"id"`
	Title            string `json:"title"`
	Year             int    `json:"year"`
	IsAvailable      bool   `json:"isAvailable"`
	Monitored        bool   `json:"monitored"`
	HasFile          bool   `json:"hasFile"`
	TmdbID           int    `json:"tmdbId"`
	ImdbID           string `json:"imdbId"`
	ProfileID        int    `json:"profileId"`
	QualityProfileID int    `json:"qualityProfileId"`
}

type sonarrSeries struct {
	ID                uint    `json:"id"`
	Title             string  `json:"title"`
	SeasonCount       int     `json:"seasonCount"`
	Seasons           []gin.H `json:"seasons"`
	Year              int     `json:"year"`
	TmdbID            int     `json:"tmdbId"`
	TvdbID            int     `json:"tvdbId"`
	ImdbID            string  `json:"imdbId"`
	ProfileID         int     `json:"profileId"`
	LanguageProfileID int     `json:"languageProfileId"`
	QualityProfileID  int     `json:"qualityProfileId"`
	Monitored         bool    `json:"monitored"`
	HasFile           bool    `json:"hasFile"`
}

func (h *ServarrHandler) toRadarrMovie(s *models.Subscription) radarrMovie {
	m := radarrMovie{
		ID: s.ID, Title: s.Title, Year: s.Year, IsAvailable: true,
		Monitored: s.Status == "subscribed" || s.Status == "",
		ProfileID: 1, QualityProfileID: 1,
	}
	if s.Source == "tmdb" {
		m.TmdbID, _ = strconv.Atoi(s.SourceID)
	}
	m.HasFile = h.mediaExists(s)
	return m
}

func (h *ServarrHandler) toSonarrSeries(s *models.Subscription) sonarrSeries {
	return sonarrSeries{
		ID: s.ID, Title: s.Title, Year: s.Year, SeasonCount: 1,
		Seasons:   []gin.H{{"seasonNumber": 1, "monitored": true}},
		Monitored: s.Status == "subscribed" || s.Status == "",
		ProfileID: 1, LanguageProfileID: 1, QualityProfileID: 1,
		HasFile: h.mediaExists(s),
	}
}

func (h *ServarrHandler) miToRadarrMovie(mi *media.MediaInfo) radarrMovie {
	m := radarrMovie{
		Title: mi.Title, Year: mi.Year, IsAvailable: true, Monitored: true,
		ProfileID: 1, QualityProfileID: 1,
	}
	if mi.Source == "tmdb" {
		m.TmdbID, _ = strconv.Atoi(mi.SourceID)
	}
	return m
}

func (h *ServarrHandler) miToSonarrSeries(mi *media.MediaInfo) sonarrSeries {
	s := sonarrSeries{
		Title: mi.Title, Year: mi.Year, SeasonCount: 1, Monitored: true,
		Seasons:   []gin.H{{"seasonNumber": 1, "monitored": true}},
		ProfileID: 1, LanguageProfileID: 1, QualityProfileID: 1,
	}
	return s
}

// mediaExists 该媒体是否已在库（TransferHistory 成功记录）；hasFile 注入为 nil 时恒 false。
func (h *ServarrHandler) mediaExists(s *models.Subscription) bool {
	if h.hasFile == nil || s.Source == "" || s.SourceID == "" {
		return false
	}
	return h.hasFile(s.Source, s.SourceID)
}

// lookupByTerm 解析 "tmdb:123"/"tvdb:456" 并查 MediaChain.Detail 富化（失败返 nil）。
// ctx 传请求级 context：客户端断开即取消外呼，不占满上游配额。
func (h *ServarrHandler) lookupByTerm(ctx context.Context, term, typ string) *media.MediaInfo {
	if h.mediaCh == nil || strings.TrimSpace(term) == "" {
		return nil
	}
	source, id := parseTerm(term)
	if source == "" || id == "" {
		return nil
	}
	mi, err := h.mediaCh.Detail(ctx, source, id)
	if err != nil || mi == nil {
		return nil
	}
	if typ == "movie" && mi.Type != media.TypeMovie {
		return nil
	}
	if typ == "tv" && mi.Type != media.TypeTV {
		return nil
	}
	return mi
}

// parseTerm 拆 "tmdb:123" → ("tmdb","123")；无前缀默认 tmdb。
func parseTerm(term string) (string, string) {
	term = strings.TrimSpace(term)
	if i := strings.Index(term, ":"); i > 0 {
		return strings.ToLower(strings.TrimSpace(term[:i])), strings.TrimSpace(term[i+1:])
	}
	if term != "" {
		return "tmdb", term
	}
	return "", ""
}

// parseIDSafe 路由 :id → uint（非法返 0）。
func parseIDSafe(s string) uint {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

// subtleEqual 常数时间字符串比较（防时序侧信道）。
func subtleEqual(a, b string) int {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b))
}
