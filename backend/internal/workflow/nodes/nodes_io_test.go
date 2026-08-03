package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/monitor"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// execExpectErr 用零值 Context 执行节点，断言返回的 error 含 want 片段。
// 用于 I/O 节点「参数校验发生在首次解引用依赖前」的 nil 安全路径。
func execExpectErr(t *testing.T, n workflow.Node, params map[string]any, want string) {
	t.Helper()
	err := n.Execute(context.Background(), params, &workflow.Context{})
	if err == nil {
		t.Fatalf("期望含 %q 的错误，实际返回 nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error=%q，期望含 %q", err.Error(), want)
	}
}

// TestSiteNodesParamValidation 站点类节点：site_id<=0 在 resolveSite 内（siteOp.Get 前）返回，nil 依赖安全。
func TestSiteNodesParamValidation(t *testing.T) {
	siteNodes := []workflow.Node{
		NewCheckinNode(nil, nil),
		NewFetchFreeleechNode(nil, nil),
		NewFetchAnnouncementNode(nil, nil),
		NewFetchMessagesNode(nil, nil),
		NewCheckHRNode(nil, nil),
		NewKeepAliveNode(nil, nil),
		NewSiteUserinfoNode(nil, nil),
		NewSiteTestNode(nil, nil),
		NewHaremStatusNode(nil, nil),
		NewTorrentDetailNode(nil, nil),
	}
	for _, n := range siteNodes {
		execExpectErr(t, n, map[string]any{"site_id": float64(0)}, "未选择站点")
	}
	// reseed：downloader_id<=0（在 keepSeed.Reseed 前）。
	execExpectErr(t, NewReseedNode(nil), map[string]any{"downloader_id": float64(0)}, "未选择下载器")
	// fetch_site_stats：site_id<=0（在 db.Table 前，nil db 安全）。
	execExpectErr(t, NewFetchSiteStatsNode(nil), map[string]any{"site_id": float64(0)}, "未选择站点")
}

// TestDownloadNodesParamValidation 下载类节点：downloader_id>0 绕开 dlOp，
// hashes 空或 category/tags/location 等空在调用 dlCh 前返回。
func TestDownloadNodesParamValidation(t *testing.T) {
	// 12 个节点 hashes 空 → 未提供任务哈希（downloader_id>0 绕开 dlOp.List）。
	hashNodes := []workflow.Node{
		NewPauseTorrentsNode(nil, nil),
		NewResumeTorrentsNode(nil, nil),
		NewDeleteTorrentsNode(nil, nil),
		NewRecheckTorrentsNode(nil, nil),
		NewReannounceTorrentsNode(nil, nil),
		NewForceStartNode(nil, nil),
		NewSetCategoryNode(nil, nil),
		NewAddTagsNode(nil, nil),
		NewSetSavePathNode(nil, nil),
		NewSetTorrentLimitNode(nil, nil),
		NewSetShareLimitNode(nil, nil),
		NewAddTrackerNode(nil, nil),
	}
	for _, n := range hashNodes {
		execExpectErr(t, n, map[string]any{"downloader_id": float64(1), "hashes": ""}, "未提供任务哈希")
	}
	// 次路径：hashes 非空 → 下一层参数校验。
	execExpectErr(t, NewSetCategoryNode(nil, nil), map[string]any{"downloader_id": float64(1), "hashes": "abc", "category": ""}, "分类为空")
	execExpectErr(t, NewAddTagsNode(nil, nil), map[string]any{"downloader_id": float64(1), "hashes": "abc", "tags": ""}, "标签为空")
	execExpectErr(t, NewSetSavePathNode(nil, nil), map[string]any{"downloader_id": float64(1), "hashes": "abc", "location": ""}, "路径为空")
	// add_tracker 用单数 hash 字段。
	execExpectErr(t, NewAddTrackerNode(nil, nil), map[string]any{"downloader_id": float64(1), "hash": "abc", "trackers": ""}, "Tracker 为空")
	// add_subscribe：title 空 + 空 Torrents → 无标题可订阅（nil subCh 安全）。
	execExpectErr(t, NewAddSubscribeNode(nil), map[string]any{"title": "", "type": "movie"}, "无标题可订阅")
}

// TestResourceMediaDataNodesParamValidation 资源/媒体/数据节点：关键字/URL/ID 空校验在依赖调用前。
func TestResourceMediaDataNodesParamValidation(t *testing.T) {
	execExpectErr(t, NewFetchRSSNode(nil, nil), map[string]any{"url": "", "site_id": float64(0)}, "RSS 地址为空")
	execExpectErr(t, NewSearchTorrentsNode(nil), map[string]any{"keyword": "", "category": ""}, "搜索关键字为空")
	execExpectErr(t, NewSearchAllSitesNode(nil), map[string]any{"keyword": "", "category": ""}, "关键词为空")
	execExpectErr(t, NewSearchMediaNode(nil), map[string]any{"keyword": "", "year": float64(0), "filter": "video"}, "关键词为空")
	execExpectErr(t, NewMediaDetailNode(nil), map[string]any{"source": "tmdb", "source_id": ""}, "源内 ID 为空")
	execExpectErr(t, NewWebhookSendNode(nil), map[string]any{"url": "", "method": "POST", "body": "", "headers": map[string]any{}}, "URL 为空")
	execExpectErr(t, NewHTTPRequestNode(nil), map[string]any{"method": "GET", "url": "", "headers": map[string]any{}, "body": "", "cookie": ""}, "URL 为空")
}

// TestFileNodesParamValidation 文件三件套：storage_id==0 在 List/Delete/Mkdir 前；path 空次路径。
// 注意 storage_id 必须恰好 ==0（负数经 uint 转换会绕回大值越过校验）。
func TestFileNodesParamValidation(t *testing.T) {
	execExpectErr(t, NewListFilesNode(nil), map[string]any{"storage_id": float64(0), "path": "/"}, "未选择存储后端")
	execExpectErr(t, NewDeleteFileNode(nil), map[string]any{"storage_id": float64(0), "path": ""}, "未选择存储后端")
	execExpectErr(t, NewMkdirNode(nil), map[string]any{"storage_id": float64(0), "path": ""}, "未选择存储后端")
	// 次路径：storage_id>0 + path 空。
	execExpectErr(t, NewDeleteFileNode(nil), map[string]any{"storage_id": float64(1), "path": ""}, "路径为空")
	execExpectErr(t, NewMkdirNode(nil), map[string]any{"storage_id": float64(1), "path": ""}, "路径为空")
}

// mockSSHRunner 实现 SSHCommandRunner 接口，返回预设结果/错误。
type mockSSHRunner struct {
	result monitor.CommandResult
	err    error
}

func (m mockSSHRunner) RunCommandNow(_ context.Context, _ uint, _ string, _ int) (monitor.CommandResult, error) {
	return m.result, m.err
}

// TestSSHCommandValidationAndExecution ssh_command：参数校验（nil runner / target_id / command）+
// 成功路径（mock runner 写 Variables）+ 失败路径（Success=false / runner err 包装）。
func TestSSHCommandValidationAndExecution(t *testing.T) {
	// nil runner → 未初始化（在所有校验前）。
	execExpectErr(t, NewSSHCommandNode(nil),
		map[string]any{"target_id": float64(0), "command": "x", "timeout_sec": float64(30), "output_var": "ssh_output"},
		"SSH 命令执行器未初始化")

	runner := mockSSHRunner{}
	// 非 nil runner + target_id<=0。
	execExpectErr(t, NewSSHCommandNode(runner),
		map[string]any{"target_id": float64(0), "command": "x", "timeout_sec": float64(30)},
		"未选择服务器")
	// command 空。
	execExpectErr(t, NewSSHCommandNode(runner),
		map[string]any{"target_id": float64(1), "command": "", "timeout_sec": float64(30)},
		"命令不能为空")

	// 成功路径：stdout 写入 Variables[output_var] 与 Content。
	okRunner := mockSSHRunner{result: monitor.CommandResult{Success: true, Stdout: "uptime-ok", TargetName: "host1", DurationMS: 7}}
	ecx := &workflow.Context{}
	if err := NewSSHCommandNode(okRunner).Execute(context.Background(),
		map[string]any{"target_id": float64(1), "command": "uptime", "timeout_sec": float64(30), "output_var": "ssh_output"}, ecx); err != nil {
		t.Fatalf("成功路径不应报错: %v", err)
	}
	if ecx.Variables["ssh_output"] != "uptime-ok" {
		t.Fatalf("ssh_output=%v want uptime-ok", ecx.Variables["ssh_output"])
	}
	if ecx.Content != "uptime-ok" {
		t.Fatalf("Content=%q want uptime-ok", ecx.Content)
	}
	if ecx.Variables["ssh_output_duration_ms"] != int64(7) {
		t.Fatalf("duration_ms=%v want 7", ecx.Variables["ssh_output_duration_ms"])
	}

	// 失败路径：Success=false → SSH 命令失败。
	failRunner := mockSSHRunner{result: monitor.CommandResult{Success: false, Error: "exit status 1"}}
	err := NewSSHCommandNode(failRunner).Execute(context.Background(),
		map[string]any{"target_id": float64(1), "command": "bad"}, &workflow.Context{})
	if err == nil || !strings.Contains(err.Error(), "SSH 命令失败") {
		t.Fatalf("Success=false 应报 SSH 命令失败，got %v", err)
	}

	// runner 返回 error → 应包装为「SSH 命令执行失败」（修复前未包装）。
	errRunner := mockSSHRunner{err: errors.New("connection refused")}
	err = NewSSHCommandNode(errRunner).Execute(context.Background(),
		map[string]any{"target_id": float64(1), "command": "x"}, &workflow.Context{})
	if err == nil || !strings.Contains(err.Error(), "SSH 命令执行失败") {
		t.Fatalf("runner err 应包装，got %v", err)
	}
}

// TestNilSafeNodesReturnSuccess 两个 nil 守卫/空输入节点：不 panic 且返回 nil。
func TestNilSafeNodesReturnSuccess(t *testing.T) {
	// add_download：downloader_id>0 绕开 dlOp，空 Torrents 不触发 dlCh.Add → 成功无副作用。
	ecx := &workflow.Context{}
	if err := NewAddDownloadNode(nil, nil).Execute(context.Background(),
		map[string]any{"downloader_id": float64(1)}, ecx); err != nil {
		t.Fatalf("add_download 空 Torrents 应成功: %v", err)
	}

	// send_message：notice==nil 守卫，恒返回 nil。
	ecx2 := &workflow.Context{}
	if err := NewSendMessageNode(nil).Execute(context.Background(),
		map[string]any{"title": "", "body": "x", "level": "info"}, ecx2); err != nil {
		t.Fatalf("send_message nil notice 应成功: %v", err)
	}
}

// TestNilDepNodesGuarded 5 个原「nil 必 panic」节点现已补 nil 守卫：返回中文 error 而非 panic。
// 回归：修复前这 5 个节点无参数校验/nil 守卫，第一动作即解引用依赖，nil 下必 panic。
func TestNilDepNodesGuarded(t *testing.T) {
	cases := []struct {
		name  string
		node  workflow.Node
		param map[string]any
		want  string
	}{
		{"checkin_all", NewCheckinAllNode(nil, nil), map[string]any{}, "站点服务未初始化"},
		{"list_torrents", NewListTorrentsNode(nil, nil), map[string]any{"downloader_id": float64(1)}, "下载器服务未初始化"},
		{"set_speed_limit", NewSetSpeedLimitNode(nil, nil), map[string]any{"downloader_id": float64(1)}, "下载器服务未初始化"},
		{"recommend_feed", NewRecommendFeedNode(nil), map[string]any{}, "推荐服务未初始化"},
		{"send_notification", NewSendNotificationNode(nil), map[string]any{"title": "t", "body": "b", "level": "info"}, "通知服务未初始化"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.node.Execute(context.Background(), tc.param, &workflow.Context{})
			if err == nil {
				t.Fatalf("%s nil 依赖应返回错误（已补守卫）", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s error=%q，期望含 %q", tc.name, err.Error(), tc.want)
			}
		})
	}
}

// TestDlMgmtNodesGuardedOnNilDep dlMgmt 骨架节点的 nil 守卫：downloader_id>0 绕开 dlOp.List 后，
// 在调用 dlCh.* 前由 runEarlyValidate 拦下空参数（与 list_torrents 等已有守卫一致）。
// 回归：修复前 dlMgmtNode.Execute 直接解引用 nil dlCh，全部 panic。
func TestDlMgmtNodesGuardedOnNilDep(t *testing.T) {
	// downloader_id=0 让 resolveDLID 走 defaultDownloaderID(nil) → 返回 0 → 报「未配置下载器」。
	for _, n := range []workflow.Node{
		NewPauseTorrentsNode(nil, nil),
		NewResumeTorrentsNode(nil, nil),
		NewDeleteTorrentsNode(nil, nil),
		NewRecheckTorrentsNode(nil, nil),
		NewReannounceTorrentsNode(nil, nil),
		NewForceStartNode(nil, nil),
		NewSetCategoryNode(nil, nil),
		NewAddTagsNode(nil, nil),
		NewSetSavePathNode(nil, nil),
		NewSetTorrentLimitNode(nil, nil),
		NewSetShareLimitNode(nil, nil),
	} {
		// 合法 hashes + 必填字段：先经 runEarlyValidate 通过；再到 nil 依赖守卫报「下载器服务未初始化」。
		err := n.Execute(context.Background(),
			map[string]any{"downloader_id": float64(1), "hashes": "abc", "category": "c", "tags": "t", "location": "/x"},
			&workflow.Context{})
		if err == nil || !strings.Contains(err.Error(), "下载器服务未初始化") {
			t.Fatalf("%s nil dlCh/dlOp 应报「下载器服务未初始化」, got %v", n.Type(), err)
		}
	}
	// add_tracker 用单数 hash 字段，独立验证。
	err := NewAddTrackerNode(nil, nil).Execute(context.Background(),
		map[string]any{"downloader_id": float64(1), "hash": "abc", "trackers": "http://t.example/x"},
		&workflow.Context{})
	if err == nil || !strings.Contains(err.Error(), "下载器服务未初始化") {
		t.Fatalf("add_tracker nil dlCh/dlOp 应报「下载器服务未初始化」, got %v", err)
	}
}

