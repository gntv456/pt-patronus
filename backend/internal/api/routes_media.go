package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
)

// registerMediaRoutes 注册媒体搜索/推荐与保种查找路由。
func registerMediaRoutes(guarded *gin.RouterGroup, d *Deps) {
	mediaH := v1.NewMediaHandler(d.MediaCh, d.Recommend)
	md := guarded.Group("/media")
	{
		md.GET("/search", mediaH.Search)
		md.GET("/recommend", mediaH.Recommend)
		md.GET("/recommend/stream", mediaH.RecommendStream)
		md.GET("/discover", mediaH.Discover)
		md.GET("/poster", mediaH.Poster)
		md.GET("/detail", mediaH.Detail)
		md.GET("/celebrity", mediaH.Celebrity)
		md.GET("/image", mediaH.Image)
		md.POST("/nfo", mediaH.NFO)
		md.GET("/config", mediaH.GetConfig)
		md.PUT("/config", middleware.RequireAdmin(), mediaH.SetConfig)
		md.GET("/quality", mediaH.GetQuality)
		md.PUT("/quality", middleware.RequireAdmin(), mediaH.SetQuality)
		md.GET("/rating", mediaH.Rating)
	}

	ksH := v1.NewKeepSeedHandler(d.KeepSeed)
	guarded.POST("/keepseed/find", ksH.Find)
	ks := guarded.Group("/keepseed")
	{
		ks.GET("/config", ksH.GetConfig)
		ks.PUT("/config", ksH.SetConfig)
		ks.POST("/reseed", ksH.Reseed)
		ks.POST("/missing", ksH.Missing)
		ks.GET("/tasks", ksH.Tasks)
		ks.POST("/tasks/:id/resolve", ksH.Resolve)
		ks.GET("/setting", ksH.GetSetting)
		ks.PUT("/setting", ksH.SetSetting)
	}
}
