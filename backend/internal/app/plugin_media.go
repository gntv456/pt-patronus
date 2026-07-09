package app

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// mediaSearchAdapter 把 chain.MediaChain 适配为 plugins.MediaSearcher，
// 供插件 Host API POST /media/search 复用宿主已有的 douban/tmdb/寸光集 元数据源，
// 让刮削类插件不必各自内嵌 TMDB client + key。
type mediaSearchAdapter struct{ c *chain.MediaChain }

func (a mediaSearchAdapter) Search(ctx context.Context, keyword string, year int) ([]plugins.MediaHit, error) {
	mis, err := a.c.Search(ctx, keyword, year, "")
	if err != nil {
		return nil, err
	}
	out := make([]plugins.MediaHit, 0, len(mis))
	for _, mi := range mis {
		out = append(out, plugins.MediaHit{
			Source:        mi.Source,
			SourceID:      mi.SourceID,
			Title:         mi.Title,
			OriginalTitle: mi.OriginalTitle,
			Year:          mi.Year,
			Type:          string(mi.Type),
			Overview:      mi.Overview,
			PosterURL:     mi.PosterURL,
			BackdropURL:   mi.BackdropURL,
			ReleaseDate:   mi.ReleaseDate,
			Rating:        mi.Rating,
			Genres:        mi.Genres,
		})
	}
	return out, nil
}
