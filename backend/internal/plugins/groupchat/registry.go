package groupchat

// registry 是站点 handler 注册表(顺序决定 Dispatch 优先级：特殊站优先，NexusPHP 兜底最后)。
var registry []Handler

func register(h Handler) { registry = append(registry, h) }

// Dispatch 返回首个 Match 命中的 handler；无匹配返回 nil。
// 引擎层调用此函数完成"站 → handler"派发(对标原版 get_site_handler 线性扫描)。
func Dispatch(s SiteRef) Handler {
	for _, h := range registry {
		if h.Match(s) {
			return h
		}
	}
	return nil
}

func init() {
	// 特殊站按需注册(站名子串互斥，顺序不敏感)；NexusPHP 兜底必须最后。
	register(zmHandler{})           // 织梦
	register(qingwaHandler{})       // 青蛙
	register(vicomoHandler{})       // 象站
	register(luckptHandler{})       // 幸运
	register(cangbaoHandler{})      // 藏宝阁
	register(dubheHandler{})        // 天枢
	register(hxptHandler{})         // 好学
	register(longPTHandler{})       // LongPT
	register(momentHandler{})       // Moment
	register(ptlgsHandler{})        // PTLGS
	register(ptskitHandler{})       // PTS
	register(thirteenCityHandler{}) // 13City
	register(nexusPHPHandler{})     // 兜底：所有未被特殊站认领的 NexusPHP 站
}
