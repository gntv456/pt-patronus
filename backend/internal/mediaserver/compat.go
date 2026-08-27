package mediaserver

import (
	"context"
	"fmt"
)

// embyCompatClient 包装 embyJellyfinClient，为国产 NAS 影视服务
// （绿影视 ugos / 飞牛影视 fnos / 极空间极影视 zspace）提供 Emby 兼容层适配：
//   - Test 消息标注兼容网关来源（便于 UI 区分）；
//   - 其余能力（Libraries/Scan/Refresh/Notify/RecentItems/Image/Users/Played/
//     LibraryItems/EpisodeCount）直接继承——三家的 Emby 兼容层实现了同构 REST。
//   - Notify 若兼容层未实现会返回错误（继承 emby 行为），调用方已按 best-effort 处理。
type embyCompatClient struct {
	embyJellyfinClient
}

func (c *embyCompatClient) Test(ctx context.Context) (string, error) {
	info, err := c.embyJellyfinClient.Test(ctx)
	if err != nil {
		return "", err
	}
	switch c.server.Type {
	case "ugos":
		return fmt.Sprintf("绿影视（Emby 兼容）· %s", info), nil
	case "fnos":
		return fmt.Sprintf("飞牛影视（Emby 兼容）· %s", info), nil
	case "zspace":
		return fmt.Sprintf("极空间极影视（Emby 兼容）· %s", info), nil
	}
	return info, nil
}

var _ Client = (*embyCompatClient)(nil)
