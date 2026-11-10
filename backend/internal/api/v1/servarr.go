// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// ServarrHandler 把 PTPatronus 订阅以 Sonarr/Radarr v3 HTTP API 对外暴露，
// 供 Overseerr/Jellyseerr 等 *arr 客户端直接接入（Server URL = http://host:8088/api/v3，API Key = 全局 API Key）。
//
// 映射：Subscription(type=movie) ↔ RadarrMovie；(type=tv) ↔ SonarrSeries。
// hasFile = 该媒体在 TransferHistory 中有成功整理记录。鉴权 = ?apikey= 或 X-Api-Key 头比对全局 API Key。
type ServarrHandler struct {
	subCh   *chain.SubscribeChain
	mediaCh *chain.MediaChain
	hasFile func(source, sourceID string) bool
	apiKey  string
}

// NewServarrHandler 构造。hasFile 为 nil 时 hasFile 恒 false（仅列表/增删可用）。
func NewServarrHandler(subCh *chain.SubscribeChain, mediaCh *chain.MediaChain, hasFile func(string, string) bool, apiKey string) *ServarrHandler {
	return &ServarrHandler{subCh: subCh, mediaCh: mediaCh, hasFile: hasFile, apiKey: apiKey}
}

// authorize 校验 ?apikey= 或 X-Api-Key 头（Servarr 惯例）；未配置 API Key 时一律拒。
func (h *ServarrHandler) authorize(c *gin.Context) bool {
	if strings.TrimSpace(h.apiKey) == "" {
		errcode.Respond(c, errcode.UnauthorizedOp)
		return false
	}
	key := c.Query("apikey")
	if key == "" {
		key = c.GetHeader("X-Api-Key")
	}
	if key == "" || subtleEqual(key, h.apiKey) != 1 {
		errcode.Respond(c, errcode.UnauthorizedOp)
		return false
	}
	return true
}

// systemStatus GET /api/v3/system/status —— Servarr 客户端连通性探测（首请求）。
func (h *ServarrHandler) SystemStatus(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"appName":  "PTPatronus",
		"instance": "PTPatronus",
		"version":  "3.0.0",
		"mode":     "compat",
	})
}

// static GET 端点（Servarr 客户端启动探测，返回静态默认值即可）。
func (h *ServarrHandler) StaticList(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	switch c.Request.URL.Path {
	case "/api/v3/qualityProfile", "/api/v3/profile":
		c.JSON(http.StatusOK, []gin.H{{"id": 1, "name": "默认"}})
	case "/api/v3/rootfolder":
		c.JSON(http.StatusOK, []gin.H{{"id": 1, "path": "/", "accessible": true}})
	case "/api/v3/tag":
		c.JSON(http.StatusOK, []gin.H{{"id": 1, "label": "默认"}})
	case "/api/v3/languageprofile":
		c.JSON(http.StatusOK, []gin.H{{"id": 1, "name": "默认"}})
	default:
		c.JSON(http.StatusOK, []any{})
	}
}

// --- Radarr (movie) ---

// listMovies GET /api/v3/movie
func (h *ServarrHandler) ListMovies(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	subs, err := h.subCh.List()
	if err != nil {
		// Servarr 客户端只认数组形态，失败回空数组 + 服务端日志（不静默吞错）。
		slog.Warn("servarr radarr list failed", "err", err)
		c.JSON(http.StatusInternalServerError, []radarrMovie{})
		return
	}
	out := make([]radarrMovie, 0, len(subs))
	for i := range subs {
		if subs[i].Type != "movie" {
			continue
		}
		out = append(out, h.toRadarrMovie(&subs[i]))
	}
	c.JSON(http.StatusOK, out)
}

// lookupMovies GET /api/v3/movie/lookup?term=tmdb:123
func (h *ServarrHandler) LookupMovies(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	mi := h.lookupByTerm(c.Request.Context(), c.Query("term"), "movie")
	if mi == nil {
		c.JSON(http.StatusOK, []radarrMovie{})
		return
	}
	c.JSON(http.StatusOK, []radarrMovie{h.miToRadarrMovie(mi)})
}

// getMovie GET /api/v3/movie/:id
func (h *ServarrHandler) GetMovie(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	sub, err := h.subCh.Get(parseIDSafe(c.Param("id")))
	if err != nil || sub == nil || sub.Type != "movie" {
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	c.JSON(http.StatusOK, h.toRadarrMovie(sub))
}

// addMovie POST /api/v3/movie {tmdbid,title,year}
func (h *ServarrHandler) AddMovie(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	var in struct {
		TmdbID int    `json:"tmdbid"`
		Title  string `json:"title"`
		Year   int    `json:"year"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.TmdbID == 0 {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	sub := &models.Subscription{Source: "tmdb", SourceID: strconv.Itoa(in.TmdbID), Title: in.Title, Year: in.Year, Type: "movie", Category: "movie"}
	created, err := h.subCh.Create(c.Request.Context(), sub)
	if err != nil {
		// #340 原因要进响应体并挂进日志：原来 `err != nil || created == nil` 之后是
		// 裸 Respond(ServarrCreateFailed)，err 整个丢掉，用户只见「创建失败」。
		errcode.Respond(c, errcode.ServarrCreateFailed.WithDetail(err))
		return
	}
	if created == nil {
		// 无错但没建出来：这不是「链层报错」，没有原因可带，如实说明而不是假装是错误。
		errcode.Respond(c, errcode.ServarrCreateFailed.WithDetail(
			errors.New("订阅未被创建（未给出具体原因）：该条目可能已存在，或来源/类型与现有订阅冲突")))
		return
	}
	c.JSON(http.StatusOK, h.toRadarrMovie(created))
}

// removeMovie DELETE /api/v3/movie/:id
func (h *ServarrHandler) RemoveMovie(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	if err := h.subCh.Delete(parseIDSafe(c.Param("id"))); err != nil {
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	c.Status(http.StatusOK)
}

// --- Sonarr (series) ---

func (h *ServarrHandler) ListSeries(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	subs, err := h.subCh.List()
	if err != nil {
		// Servarr 客户端只认数组形态，失败回空数组 + 服务端日志（不静默吞错）。
		slog.Warn("servarr sonarr list failed", "err", err)
		c.JSON(http.StatusInternalServerError, []sonarrSeries{})
		return
	}
	out := make([]sonarrSeries, 0, len(subs))
	for i := range subs {
		if subs[i].Type != "tv" {
			continue
		}
		out = append(out, h.toSonarrSeries(&subs[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (h *ServarrHandler) LookupSeries(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	mi := h.lookupByTerm(c.Request.Context(), c.Query("term"), "tv")
	if mi == nil {
		c.JSON(http.StatusOK, []sonarrSeries{})
		return
	}
	c.JSON(http.StatusOK, []sonarrSeries{h.miToSonarrSeries(mi)})
}

func (h *ServarrHandler) GetSeries(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	sub, err := h.subCh.Get(parseIDSafe(c.Param("id")))
	if err != nil || sub == nil || sub.Type != "tv" {
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	c.JSON(http.StatusOK, h.toSonarrSeries(sub))
}

func (h *ServarrHandler) AddSeries(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	var in struct {
		TmdbID int    `json:"tmdbid"`
		TvdbID int    `json:"tvdbid"`
		Title  string `json:"title"`
		Year   int    `json:"year"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	source, sourceID := "tmdb", ""
	if in.TmdbID > 0 {
		sourceID = strconv.Itoa(in.TmdbID)
	} else if in.TvdbID > 0 {
		source, sourceID = "tvdb", strconv.Itoa(in.TvdbID)
	}
	if sourceID == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	sub := &models.Subscription{Source: source, SourceID: sourceID, Title: in.Title, Year: in.Year, Type: "tv", Category: "tv"}
	created, err := h.subCh.Create(c.Request.Context(), sub)
	if err != nil {
		// #340 原因要进响应体并挂进日志：原来 `err != nil || created == nil` 之后是
		// 裸 Respond(ServarrCreateFailed)，err 整个丢掉，用户只见「创建失败」。
		errcode.Respond(c, errcode.ServarrCreateFailed.WithDetail(err))
		return
	}
	if created == nil {
		// 无错但没建出来：这不是「链层报错」，没有原因可带，如实说明而不是假装是错误。
		errcode.Respond(c, errcode.ServarrCreateFailed.WithDetail(
			errors.New("订阅未被创建（未给出具体原因）：该条目可能已存在，或来源/类型与现有订阅冲突")))
		return
	}
	c.JSON(http.StatusOK, h.toSonarrSeries(created))
}

// updateSeries PUT /api/v3/series —— 别名：存在则忽略，不存在则建（upsert 简化）。
func (h *ServarrHandler) UpdateSeries(c *gin.Context) { h.AddSeries(c) }

func (h *ServarrHandler) RemoveSeries(c *gin.Context) {
	if !h.authorize(c) {
		return
	}
	if err := h.subCh.Delete(parseIDSafe(c.Param("id"))); err != nil {
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	c.Status(http.StatusOK)
}
