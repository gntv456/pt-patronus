package forumsignin

// byKey 是 key → handler 注册表；order 保留注册顺序（决定结果列表稳定排序）。
var (
	byKey  = map[string]Handler{}
	order  []string
)

func register(h Handler) {
	if _, ok := byKey[h.Key()]; ok {
		return
	}
	byKey[h.Key()] = h
	order = append(order, h.Key())
}

// Lookup 返回 key 对应 handler；无匹配返回 nil。
func Lookup(key string) Handler { return byKey[key] }

// Labels 返回全部已注册站点的 key→label（供前端展示可配置站点列表）。
func Labels() map[string]string {
	out := make(map[string]string, len(order))
	for _, k := range order {
		out[k] = byKey[k].Label()
	}
	return out
}

// SiteEntry 是单个支持站点的 key/label。
type SiteEntry struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Supported 按注册顺序返回全部支持站点（顺序稳定，供配置帮助与 labels action）。
func Supported() []SiteEntry {
	out := make([]SiteEntry, 0, len(order))
	for _, k := range order {
		out = append(out, SiteEntry{Key: k, Label: byKey[k].Label()})
	}
	return out
}

func init() {
	// 特殊站（自有 API/JSON 契约）优先注册。
	register(nodeseekHandler{})
	register(pojieHandler{}) // 吾爱破解（Discuz! dsu_paulsign 两步流：GET formhash → POST）
	// NexusPHP 系论坛区签到（共享 signin.php 路径与关键词解析）。
	register(nexusSignin{key: "fengchao", label: "蜂巢"})
	register(nexusSignin{key: "feiniu", label: "飞牛"})
	register(nexusSignin{key: "ningmeng", label: "柠檬"})
	register(nexusSignin{key: "guoyu", label: "国语视界"})
	register(nexusSignin{key: "ssdforum", label: "SSDForum"})
}
