package errcode

// #344 「按 id 定位的记录没了」这一族错误码里，有 9 条还是光秃秃的「X 不存在」。
//
// 这一族的语义是确定的：记录已被删除 / id 失效 / 从没建过，下一步永远是
// 「刷新对应的列表重新选」。仓库里已有 18 条写成这个形状（对照组）：
//
//	MonitorAlertNotFound  告警不存在，可能已被清理，请刷新告警列表
//	FilterRuleNotFound    过滤规则不存在，可能已被删除，请刷新规则列表
//	CloudDriveNotFound    云盘账号不存在，可能已被删除，请重新选择云盘
//	…
//
// 剩下 9 条没跟上（站点/下载器/媒体服务器/订阅/刷流任务/整理任务/RSS 订阅/
// 工作流/webhook/被控端/通用资源）。用户点进去看到的是「站点不存在」，
// 既不知道是「我删了」还是「从没有过」，也不知道该刷新列表——
// 而列表页往往就是他要找的地方。
//
// 只改 NotFound 这一族（语义同构、下一步唯一），不顺手改其它 60 多条干巴文案：
// 那些语义各不相同（口令错误 / 签名失败 / 挂载点非法），要逐条确认才对，
// 混进来会让这一族改不干净。

import (
	"strings"
	"testing"
)

// notfoundFamily 该族码：语义都是「按 id 定位的记录没了」，下一步是刷新对应列表。
var notfoundFamily = []string{
	"NotFound", "WebhookNotFound", "SiteNotFound", "DownloaderNotFound",
	"MediaServerNotFound", "AgentNotFound", "ResourceNotFound",
	"SubscriptionNotFound", "BrushflowTaskNotFound", "TransferTaskNotFound",
	"RssSubscriptionGone", "WorkflowNotFound", "ComposeFileNotFound",
}

// TestNotFoundFamilyIsActionable 该族每条都要说清「可能已被删除」并给出下一步。
func TestNotFoundFamilyIsActionable(t *testing.T) {
	byVar := map[string]*Error{
		"NotFound": NotFound, "WebhookNotFound": WebhookNotFound, "SiteNotFound": SiteNotFound,
		"DownloaderNotFound": DownloaderNotFound, "MediaServerNotFound": MediaServerNotFound,
		"AgentNotFound": AgentNotFound, "ResourceNotFound": ResourceNotFound,
		"SubscriptionNotFound": SubscriptionNotFound, "BrushflowTaskNotFound": BrushflowTaskNotFound,
		"TransferTaskNotFound": TransferTaskNotFound, "RssSubscriptionGone": RssSubscriptionGone,
		"WorkflowNotFound": WorkflowNotFound, "ComposeFileNotFound": ComposeFileNotFound,
	}
	dry := []string{}
	for _, v := range notfoundFamily {
		e := byVar[v]
		if e == nil {
			t.Fatalf("判据里列了 %s，但注册表里没有（码被改名或删了，判据要同步）", v)
		}
		// 下一步：给得出一个能走的动作即可（刷新 / 重新选择 / 重新建 / 换目录…）。
		// 不限定用哪几个词——ComposeFileNotFound 说的是「请确认目录选对，或先在此目录
		// 新建 compose 文件」，那是正确的下一步，只是没用「刷新」这类词。
		hasNext := strings.Contains(e.Message, "请") ||
			strings.Contains(e.Message, "重新") ||
			strings.Contains(e.Message, "或先")
		if !hasNext {
			dry = append(dry, v+"（"+e.Message+"）")
		}
	}
	if len(dry) > 0 {
		t.Errorf("这一族里这些码没给可走的下一步：\n  %s\n"+
			"（同族其余码的形状是「X 不存在，可能已被删除，请刷新 XX 列表」）", strings.Join(dry, "\n  "))
	}
}

// TestNotFoundFamilyKeepsObject 说清是「什么」没了——用户至少知道在找什么。
func TestNotFoundFamilyKeepsObject(t *testing.T) {
	for _, c := range []struct {
		e    *Error
		want string
	}{
		{SiteNotFound, "站点"},
		{DownloaderNotFound, "下载器"},
		{MediaServerNotFound, "媒体服务器"},
		{SubscriptionNotFound, "订阅"},
		{WorkflowNotFound, "工作流"},
		{RssSubscriptionGone, "RSS"},
		{BrushflowTaskNotFound, "刷流"},
		{TransferTaskNotFound, "整理"},
	} {
		if !strings.Contains(c.e.Message, c.want) {
			t.Errorf("%s 的文案没说清是什么没了（应含 %q）：%q", c.e.Code, c.want, c.e.Message)
		}
	}
}
