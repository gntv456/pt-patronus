package v1

// #370 插件市场的收藏/榜单出口把「市场运营层没装配」说成 400「请求参数有误」。
//
// 同一个包里 ErrMarketOperUnavailable 早就被正确处理过了 —— instanceErr
// （plugin_market.go:69）把它归到 DependencyMissing，注释也写明「该去开配置」。
// 但 FavoritePlugin / UnfavoritePlugin / MarketCharts / MarketFavorites 这几条
// 压根没走那个分档：
//
//	FavoritePlugin     → InvalidInput.WithErr(err)   400
//	UnfavoritePlugin   → InvalidInput.WithErr(err)   400
//	MarketCharts       → BadGateway.WithErr(err)      502「上游服务不可用，请稍后重试」
//	MarketFavorites    → BadGateway.WithErr(err)      502
//
// 两种错法都指错方向：收藏失败说「参数有误」⇒ 用户去改那个根本没得改的插件 id；
// 榜单拉不到说「上游服务不可用，稍后重试」⇒ 市场运营层压根没装配（不是没装插件市场，
// 是市场那一层的数据存储没接上），重试一万次也一样。
//
// 判据钉「这些出口必须认 ErrMarketOperUnavailable」，复用既有的 instanceErr 分档，
// 不新造一套 —— 这正是仓库里 pluginInstallFailure 的做法。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// TestMarketOperUnavailableIsClassified 这一档必须有归属：跟建分身那条同口径。
func TestMarketOperUnavailableIsClassified(t *testing.T) {
	got := instanceErr(plugins.ErrMarketOperUnavailable)
	if got.Code != errcode.DependencyMissing.Code {
		t.Fatalf("市场运营层未装配应归 %q（跟 instanceErr 一致），got %q",
			errcode.DependencyMissing.Code, got.Code)
	}
}

// TestMarketOutletsMustClassifyOperUnavailable 四个运营出口都必须走分档。
func TestMarketOutletsMustClassifyOperUnavailable(t *testing.T) {
	src, err := os.ReadFile("plugin_market.go")
	if err != nil {
		t.Fatalf("读不到 plugin_market.go：%v", err)
	}
	// 剥整行注释：注释里会引用旧形状的字面文本。
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")

	for _, fn := range []string{
		"MarketCharts", "MarketFavorites", "FavoritePlugin", "UnfavoritePlugin",
	} {
		at := strings.Index(code, "func (h *PluginHandler) "+fn+"(")
		if at < 0 {
			t.Errorf("找不到 %s（判据没有落点）", fn)
			continue
		}
		tail := code[at:]
		if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
			tail = tail[:1+n]
		}
		// 不许直接拿一个通用码把原因丢进日志。
		for _, generic := range []string{"errcode.InvalidInput.WithErr(err)", "errcode.BadGateway.WithErr(err)"} {
			if strings.Contains(tail, generic) {
				t.Errorf("%s 仍用 %q 一把罩住：市场运营层没装配、参数有误、市场源不可达是三件事"+
					"（instanceErr 已经给这一档分好类了）", fn, generic)
			}
		}
		if !strings.Contains(tail, "marketOperErr(c, err)") {
			t.Errorf("%s 必须走 marketOperErr 分档（与 instanceErr 同口径）", fn)
		}
	}
}

// 反面对照：市场源不可达（网络问题）仍应是 502，不能被并进「运营层没装配」。
func TestMarketOperUnavailableKeepsUnreachableSeparate(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	marketOperErr(c, plugins.ErrMarketUnreachable)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("市场源不可达该 502，got %d", rec.Code)
	}
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if env["code"] == errcode.DependencyMissing.Code {
		t.Errorf("市场源不可达不许说成「依赖没装配」：%v", env)
	}
}

// 反面对照：真的参数错（缺 id）仍应 400。
func TestMarketOperUnavailableKeepsInputSeparate(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	marketOperErr(c, errors.New("plugin id required"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("真的参数错该 400，got %d", rec.Code)
	}
}
