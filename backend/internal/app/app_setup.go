package app

import (
	"errors"
	"log/slog"
	"os"
	"strings"

	bundledDefinitions "github.com/hxpt/ptpatronus/definitions"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/assessment"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
	"github.com/hxpt/ptpatronus/internal/site/schemas"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/hxpt/ptpatronus/internal/workflow/nodes"

	"gorm.io/gorm"
)

// newSiteInfra 构造站点基础设施：注册全部 schema 适配器到 registry，挂载到 module 总线，
// 返回 registry / event bus / chain.Base / mteam 适配器。hc 为站点抓取 HTTP 客户端。
func newSiteInfra(hc *httpclient.Client) (*site.Registry, *event.Bus, chain.Base, *schemas.MTeam) {
	registry := site.NewRegistry()
	defsDir := os.Getenv("DEFINITIONS_DIR")
	defsDirExplicit := defsDir != ""
	if defsDir == "" {
		defsDir = defaultDefinitionsDir()
	}
	loadSiteDefinitions(registry, defsDir, defsDirExplicit)
	nexus := schemas.NewNexusPHP(hc)
	registry.RegisterAdapter(nexus)
	mteam := schemas.NewMTeam(hc)
	registry.RegisterAdapter(mteam)
	unit3d := schemas.NewUnit3D(hc)
	registry.RegisterAdapter(unit3d)
	gazelle := schemas.NewGazelle(hc)
	registry.RegisterAdapter(gazelle)
	luminance := schemas.NewLuminance(hc)
	registry.RegisterAdapter(luminance)
	gazelleJSONAPI := schemas.NewGazelleJSONAPI(hc)
	registry.RegisterAdapter(gazelleJSONAPI)
	// Generic HTML（兜底，多 schema 实例）：解锁 "" + 命名 HTML schema 的选择器驱动站。
	genericHTMLs := make([]*schemas.GenericHTML, 0, 12)
	for _, sc := range []string{"", "xbtit", "tcg", "tbsource", "filelist", "discuz", "rartracker", "cgbtsource", "meantorrent", "aidoruonline", "smallhorse"} {
		g := schemas.NewGenericHTML(hc, sc)
		registry.RegisterAdapter(g)
		genericHTMLs = append(genericHTMLs, g)
	}
	// Avistaz 网络（JWT token API，4 站）
	avistaz := schemas.NewAvistaz(hc)
	registry.RegisterAdapter(avistaz)
	// Rousi Pro（Bearer passkey API）
	rousi := schemas.NewRousi(hc)
	registry.RegisterAdapter(rousi)
	// Flarum 论坛社区（invites.fun / pting.club 等 PT 论坛，JSON:API + cookie + 签到）
	flarum := schemas.NewFlarum(hc)
	registry.RegisterAdapter(flarum)
	// Generic JSON（命名 JSON schema）
	genericJSONs := make([]*schemas.GenericJSON, 0, 5)
	for _, sc := range []string{"tnode", "yemapt", "f3nix", "tbdev", "fsm"} {
		gj := schemas.NewGenericJSON(hc, sc)
		registry.RegisterAdapter(gj)
		genericJSONs = append(genericJSONs, gj)
	}

	mm := module.New()
	mm.Register(nexus)
	mm.Register(mteam)
	mm.Register(unit3d)
	mm.Register(gazelle)
	mm.Register(luminance)
	mm.Register(gazelleJSONAPI)
	for _, g := range genericHTMLs {
		mm.Register(g)
	}
	mm.Register(avistaz)
	mm.Register(rousi)
	mm.Register(flarum)
	for _, gj := range genericJSONs {
		mm.Register(gj)
	}
	bus := event.New()
	base := chain.NewBase(mm, bus)
	return registry, bus, base, mteam
}

// newEntertainmentRegistry 注册全部娱乐游戏适配器（好学/财神/LongPT/NovaHD/CBG/UB/PANDA/蟹黄堡 等，
// 含转盘/老虎机/刮刮乐/下注/状态机/红包雨多品类）。
func newEntertainmentRegistry() *entertainment.Registry {
	entReg := entertainment.NewRegistry()
	entReg.Register("hxpt.org", entertainment.JggGame{})
	entReg.Register("hxpt.org", entertainment.MagicScratchGame{})
	entReg.Register("hxpt.org", entertainment.MedalSynthesisGame{})
	for _, h := range []string{"cspt.top", "cspt.cc"} {
		entReg.Register(h, entertainment.CsptScratchGame{})
	}
	// LongPT：幸运大转盘 + 每日抽奖 + 六十四卦勋章（/pt-api/v1/lucky|medals|nexus/*，纯 Cookie 鉴权）
	entReg.Register("longpt.org", entertainment.LongPTWheelGame{})
	entReg.Register("longpt.org", entertainment.LongPTDailyGame{})
	entReg.Register("longpt.org", entertainment.LongPTMedalGame{})
	// 魔力农场（通用 magic_fram.php 插件，好学 hxpt + NovaHD + 包子 + PlayLet 共用；纯 Cookie，货币=seedbonus）
	entReg.Register("hxpt.org", entertainment.MagicFarmGame{})
	entReg.Register("novahd.top", entertainment.MagicFarmGame{})
	entReg.Register("t-baozi.cc", entertainment.MagicFarmGame{})
	entReg.Register("playlet.cc", entertainment.MagicFarmGame{})
	// NovaHD：魔力值刮刮乐 + 幸运抽奖（magic_scratch.php / fortune-wheel-spin.php，纯 Cookie 鉴权）
	entReg.Register("novahd.top", entertainment.NovaHDScratchGame{})
	entReg.Register("novahd.top", entertainment.NovaHDLuckyGame{})
	// ZRPT（zrpt.cc）魔力值刮刮乐：与 NovaHD 同款 magic_scratch.php 插件（?action=buy/scratch 实测一致）
	entReg.Register("zrpt.cc", entertainment.NovaHDScratchGame{})
	// 多站幸运转盘/老虎机/抽奖（批1，纯 Cookie）：CBG(藏宝阁) 转盘+老虎机、UB 转盘、PANDA+蟹黄堡+包子+织梦+DiscFan+自由农场+HDTime+青蛙 LuckyDraw 插件、March 转盘、不可躺抽奖
	entReg.Register("cangbao.ge", entertainment.CbgWheelGame{})
	entReg.Register("cangbao.ge", entertainment.CbgSlotGame{})
	entReg.Register("ubits.club", entertainment.UbWheelGame{})
	entReg.Register("pandapt.net", entertainment.LuckyDrawWheelGame{})
	entReg.Register("crabpt.vip", entertainment.LuckyDrawWheelGame{})
	entReg.Register("t-baozi.cc", entertainment.LuckyDrawWheelGame{})
	entReg.Register("zmpt.cc", entertainment.LuckyDrawWheelGame{})
	// 织梦双色球（omnibot /nodeapi/union-lotto，与麒麟同源；批11 实测 BY0sxZy4.js 前缀）
	// 同批排除：52PT 证券交易所（静态榜单无交易交互）、天枢火星幸运局（WebSocket 房间大厅）、
	// 好学 huihu.php（404）、HDTime bigorsmall.php（404 非 Panda 同源）
	entReg.Register("zmpt.cc", entertainment.ZmptLottoGame{})
	entReg.Register("discfan.net", entertainment.LuckyDrawWheelGame{})
	entReg.Register("pt.0ff.cc", entertainment.LuckyDrawWheelGame{})
	entReg.Register("hdtime.org", entertainment.LuckyDrawWheelGame{})
	entReg.Register("www.qingwapt.com", entertainment.LuckyDrawWheelGame{})
	// 憨憨：幸运大转盘（lucky.php + POST /plugin/lucky-draw，纯 Cookie，货币=憨豆）
	entReg.Register("hhanclub.net", entertainment.HhanLuckyWheelGame{})
	entReg.Register("duckboobee.org", entertainment.MarchWheelGame{})
	entReg.Register("www.tangpt.top", entertainment.TangptLotteryGame{})
	// 不可躺 猜大小（/api/big-small/*：snapshot 快照 + bets 下注 JSON）+ 老虎机（/web/omnibot/slot-machine/*）
	entReg.Register("www.tangpt.top", entertainment.TangptBigSmallGame{})
	entReg.Register("www.tangpt.top", entertainment.TangptSlotGame{})
	// 高清视频 /games/ 大厅（批10）：压大小/老虎机/刮刮乐/HiLo/21点；货币=电影票
	// ddz/poker/zjh/sports/stock/chest 为对战或复杂玩法不接；lucky-draw 302 未开启
	entReg.Register("hdvideo.top", entertainment.HdvideoBigSmallGame{})
	entReg.Register("hdvideo.top", entertainment.HdvideoSlotsGame{})
	entReg.Register("hdvideo.top", entertainment.HdvideoScratchGame{})
	entReg.Register("hdvideo.top", entertainment.HdvideoHiloGame{})
	entReg.Register("hdvideo.top", entertainment.HdvideoBlackjackGame{})
	// 批10.5：签到宝箱（连续 7/15/30 天开箱）+ 股票模拟（market.php A 股行情，持仓只读+显式买卖）
	// 菠菜系统当前无赛事（「暂无可押注的赛事」）backlog；ddz/poker/zjh 对战不接
	entReg.Register("hdvideo.top", entertainment.HdvideoChestGame{})
	entReg.Register("hdvideo.top", entertainment.HdvideoStockGame{})
	// 下注/选号（批2，Bettable）：猜大小/骰宝/双色球/翻面/赛事押注/红包
	entReg.Register("hxpt.org", entertainment.HxptBigSmallGame{})
	entReg.Register("pandapt.net", entertainment.PandaBigSmallGame{})
	entReg.Register("cangbao.ge", entertainment.CbgSicboGame{})
	entReg.Register("crabpt.vip", entertainment.CrabUnionLottoGame{})
	entReg.Register("pandapt.net", entertainment.PandaFlipGame{})
	entReg.Register("ourbits.club", entertainment.OurbitsBetGame{})
	// 同款 NexusPHP「博彩频道」竞猜插件（2026-09 普查：当前均无进行中盘口，Events 空列表验证通过）
	for _, h := range []string{"open.cd", "totheglory.im", "hdsky.me", "nanyangpt.com", "ptchdbits.co", "pt.hdupt.com"} {
		entReg.Register(h, entertainment.OurbitsBetGame{})
	}
	entReg.Register("ubits.club", entertainment.UbitsRedpacketGame{})
	// 状态机/红包雨（批3）：二十一点 + 好学红包雨
	entReg.Register("audiences.me", entertainment.AudiencesBlackjackGame{})
	entReg.Register("hxpt.org", entertainment.HxptRedpacketGame{})
	// NexusPHP 二十一点整页表单插件（2026-09 全站普查：52PT/HDArea/HDU/PTHome/CyanBug/TTG/SSD；
	// 青蛙/KEEPFRDS 房间制或未装不接）
	for _, h := range []string{"52pt.site", "hdarea.club", "pt.hdupt.com", "pthome.net", "cyanbug.net", "totheglory.im", "springsunday.net"} {
		entReg.Register(h, entertainment.NxpBlackjackGame{})
	}
	// CHDBits 21点（game=hit&start=yes 开牌形态，批7 实测页面确认，复用 Nxp 解析）
	entReg.Register("ptchdbits.co", entertainment.NxpBlackjackGame{})
	// OpenCD 21點（对抗制：game=start&bonus 自开局 / gameid 应战；动作 hit/stand/double 同族）
	entReg.Register("open.cd", entertainment.OpenCDBlackjackGame{})
	// 蜂巢五子棋人机挑战（GET/POST /api/gobang JSON，flarum_session cookie 鉴权）
	entReg.Register("pting.club", entertainment.PtingGobangGame{})
	// 13City 彩票中心（lottery.php 每天 20:00 开奖，货币=啤酒瓶；购买走 Bet 显式下单）
	// HDHome 站庆抽奖 2026-11 才开启（无参与按钮）暂缓；OpenCD 乐透上期 2011 年已停摆跳过
	entReg.Register("13city.org", entertainment.City13LotteryGame{})
	// 13City 大转盘 + 幸运骰子（POST action= 协议，页面 JS 实测）
	entReg.Register("13city.org", entertainment.City13WheelGame{})
	entReg.Register("13city.org", entertainment.City13DiceGame{})
	// wof.php 转盘家族：HDKylin（GET wof/ajax_chs.php）+ UBits（POST wof.php + csrf，UB 已有 ub_wheel 之外的第二转盘）
	entReg.Register("www.hdkyl.in", entertainment.WofWheelGame{})
	entReg.Register("ubits.club", entertainment.WofWheelGame{})
	// HDKylin 双色球（union-lotto.php，POST json nums=[6红+1蓝]；GET 返回投注记录）
	entReg.Register("www.hdkyl.in", entertainment.HdkylinLottoGame{})
	// 蟹黄堡快三（/nodeapi/fast-three JSON，角色三选一投注，Bet 显式）
	entReg.Register("crabpt.vip", entertainment.CrabFastThreeGame{})
	// 思齐种菜赚魔力（plant_game.php POST action= 协议；一键收获需验证码不自动）
	entReg.Register("si-qi.xyz", entertainment.SiqiPlantGame{})
	// 思齐钓鱼（siqi_fishing.php：页面内嵌 FISHING_DATA，POST action=perform_fishing+slider_position；每日 30 次免费）
	entReg.Register("si-qi.xyz", entertainment.SiqiFishingGame{})
	// PANDA 好友买卖（ajax.php action=friendTrade* 家族；Play 只读 InitOrHome，claim/redeem/protect 走 Bet）
	entReg.Register("pandapt.net", entertainment.PandaFriendTradeGame{})
	// 天枢星尘农场（ajax.php action+params JSON 协议，Farmable 复用农场 UI）
	entReg.Register("dubhe.site", entertainment.DubheStardustGame{})
	return entReg
}

// newAssessmentRegistry 注册站点考核适配器（MTeam JSON / NexusPHP HTML / TNode / Sunnypt）。
func newAssessmentRegistry() *assessment.Registry {
	assessReg := assessment.NewRegistry()
	assessReg.Register(assessment.NexusPHPAdapter{})
	assessReg.Register(assessment.MTeamAdapter{})
	assessReg.Register(assessment.TNodeAdapter{})
	assessReg.Register(assessment.SunnyptAdapter{})
	return assessReg
}

// newMedalRegistry 注册勋章墙适配器（NexusPHP HTML 双布局 / MTeam / TNode / Unit3D）。
func newMedalRegistry() *medalwall.Registry {
	medalReg := medalwall.NewRegistry()
	medalReg.Register(medalwall.NexusPHPAdapter{})
	medalReg.Register(medalwall.MTeamAdapter{})
	medalReg.Register(medalwall.TNodeAdapter{})
	medalReg.Register(medalwall.Unit3DAdapter{})
	return medalReg
}

func defaultDefinitionsDir() string {
	for _, dir := range []string{"./definitions", "./backend/definitions"} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return "./definitions"
}

func loadSiteDefinitions(registry *site.Registry, defsDir string, externalExplicit bool) {
	if err := registry.LoadDefinitionsFS(bundledDefinitions.FS, "."); err != nil {
		slog.Warn("load bundled site definitions failed", "err", err)
	} else {
		slog.Info("loaded bundled site definitions", "count", len(registry.Definitions()))
	}

	if strings.TrimSpace(defsDir) == "" {
		return
	}
	if err := registry.LoadDefinitions(defsDir); err != nil {
		if !externalExplicit && errors.Is(err, os.ErrNotExist) {
			slog.Info("external site definitions directory not found; using bundled definitions", "dir", defsDir, "count", len(registry.Definitions()))
			return
		}
		slog.Warn("load external site definitions failed; using bundled definitions", "dir", defsDir, "err", err)
		return
	}
	slog.Info("loaded external site definitions", "dir", defsDir, "total", len(registry.Definitions()))
}

// registerWorkflowNodes 注册全部工作流节点（RSS/搜索/过滤/下载/订阅/通知/HTTP/条件/
// 下载管理/站点业务/资源处理/媒体/文件/数据/通知外发/流程控制）到工作流 registry。
func registerWorkflowNodes(
	wfReg *workflow.Registry,
	database *gorm.DB,
	hc *httpclient.Client,
	siteOp *oper.SiteOper,
	siteCh *chain.SiteChain,
	filterEng *filter.Engine,
	dlCh *chain.DownloadChain,
	dlOp *oper.DownloaderOper,
	subCh *chain.SubscribeChain,
	noticeSvc *notice.Service,
	keepSeedCh *chain.KeepSeedChain,
	mediaCh *chain.MediaChain,
	recommendCh *chain.RecommendChain,
	fileCh *chain.FileManagerChain,
	notifCh *chain.NotificationChain,
) {
	wfReg.Register(nodes.NewFetchRSSNode(hc, siteOp))
	wfReg.Register(nodes.NewSearchTorrentsNode(siteCh))
	wfReg.Register(nodes.NewFetchSiteStatsNode(database))
	wfReg.Register(nodes.NewFilterTorrentsNode())
	wfReg.Register(nodes.NewLimitNode())
	wfReg.Register(nodes.NewFilterRulesNode(filterEng))
	wfReg.Register(nodes.NewAddDownloadNode(dlCh, dlOp))
	wfReg.Register(nodes.NewAddSubscribeNode(subCh))
	wfReg.Register(nodes.NewSendMessageNode(noticeSvc))
	wfReg.Register(nodes.NewHTTPRequestNode(hc))
	wfReg.Register(nodes.NewExtractFieldsNode())
	wfReg.Register(nodes.NewCheckinNode(siteCh, siteOp))
	wfReg.Register(nodes.NewReseedNode(keepSeedCh))
	wfReg.Register(nodes.NewConditionNode())
	wfReg.Register(nodes.NewSwitchNode())
	wfReg.Register(nodes.NewWebhookSendNode(hc))
	wfReg.Register(nodes.NewMathNode())
	wfReg.Register(nodes.NewDelayNode())
	wfReg.Register(nodes.NewNoteNode())
	wfReg.Register(nodes.NewSetVariableNode())
	// 下载管理（封装 DownloadChain 进阶操作）
	wfReg.Register(nodes.NewListTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewPauseTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewResumeTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewDeleteTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewRecheckTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewReannounceTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewForceStartNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetCategoryNode(dlCh, dlOp))
	wfReg.Register(nodes.NewAddTagsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetSavePathNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetSpeedLimitNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetTorrentLimitNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetShareLimitNode(dlCh, dlOp))
	wfReg.Register(nodes.NewAddTrackerNode(dlCh, dlOp))
	// 站点业务（封装 SiteChain）
	wfReg.Register(nodes.NewCheckinAllNode(siteCh, siteOp))
	wfReg.Register(nodes.NewFetchFreeleechNode(siteCh, siteOp))
	wfReg.Register(nodes.NewFetchAnnouncementNode(siteCh, siteOp))
	wfReg.Register(nodes.NewFetchMessagesNode(siteCh, siteOp))
	wfReg.Register(nodes.NewCheckHRNode(siteCh, siteOp))
	wfReg.Register(nodes.NewKeepAliveNode(siteCh, siteOp))
	wfReg.Register(nodes.NewSiteUserinfoNode(siteCh, siteOp))
	wfReg.Register(nodes.NewSiteTestNode(siteCh, siteOp))
	wfReg.Register(nodes.NewHaremStatusNode(siteCh, siteOp))
	// 资源处理
	wfReg.Register(nodes.NewSearchAllSitesNode(siteCh))
	wfReg.Register(nodes.NewTorrentDetailNode(siteCh, siteOp))
	wfReg.Register(nodes.NewSortTorrentsNode())
	wfReg.Register(nodes.NewDedupeTorrentsNode())
	wfReg.Register(nodes.NewPickBestNode())
	// 媒体（MediaChain / RecommendChain）
	wfReg.Register(nodes.NewSearchMediaNode(mediaCh))
	wfReg.Register(nodes.NewMediaDetailNode(mediaCh))
	wfReg.Register(nodes.NewRecommendFeedNode(recommendCh))
	// 文件（FileManagerChain）
	wfReg.Register(nodes.NewListFilesNode(fileCh))
	wfReg.Register(nodes.NewDeleteFileNode(fileCh))
	wfReg.Register(nodes.NewMkdirNode(fileCh))
	// 数据
	wfReg.Register(nodes.NewParseJSONNode())
	wfReg.Register(nodes.NewRegexExtractNode())
	// 通知（直发外发渠道）
	wfReg.Register(nodes.NewSendNotificationNode(notifCh))
	// 流程
	wfReg.Register(nodes.NewStopFlowNode())
	wfReg.Register(nodes.NewRandomPickNode())
	wfReg.Register(nodes.NewFormatSizeNode())
}
