package v1

// 插件市场·安装面：市场源与签名信任配置、市场浏览、从市场安装（含失败分档映射）。
// 从 plugin.go 拆出以守 400 行门禁；同域的运营面（百鸟朝凤/收藏/实例）在 plugin_market.go。

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

type pluginInstallReq struct {
	Source                      string                          `json:"source" binding:"max=256"`
	ID                          string                          `json:"id" binding:"max=128"`
	AcceptedPermissions         []string                        `json:"accepted_permissions" binding:"dive,max=64"`
	AcceptedPermissionsByPlugin []pluginInstallPermissionAccept `json:"accepted_permissions_by_plugin" binding:"dive"`
}

type pluginInstallPermissionAccept struct {
	ID                  string   `json:"id" binding:"max=128"`
	AcceptedPermissions []string `json:"accepted_permissions" binding:"dive,max=64"`
}

// MarketSources 返回 UI 持久化的插件市场源列表（GitHub 仓库地址或任意 URL）。
// official 为官方市场仓库地址，供前端打「官方」标签与「恢复官方源」。
func (h *PluginHandler) MarketSources(c *gin.Context) {
	sources, err := h.manager.MarketSourcesSnapshot()
	if err != nil {
		errcode.Respond(c, errcode.PluginMarketSourcesUnreadable.WithErr(err))
		return
	}
	if sources == nil {
		// 空的真的没配过也要回 []：回 null 会让前端 marketSources.value.map 直接炸。
		sources = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"sources": sources, "official": plugins.OfficialMarketSource})
}

type pluginMarketSourcesReq struct {
	Sources []string `json:"sources" binding:"dive,max=256"`
}

// SetMarketSources 持久化插件市场源列表。
func (h *PluginHandler) SetMarketSources(c *gin.Context) {
	var req pluginMarketSourcesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.manager.SetMarketSources(req.Sources); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	sources, err := h.manager.MarketSourcesSnapshot()
	if err != nil {
		// 刚写完却读不回来：如实说出来，别让面板显示一份其实没生效的列表。
		errcode.Respond(c, errcode.PluginMarketSourcesUnreadable.WithErr(err))
		return
	}
	if sources == nil {
		sources = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"sources": sources, "official": plugins.OfficialMarketSource})
}

// TrustConfig 返回插件签名信任配置（官方只读公钥 + 用户受信公钥 + allow_unsigned）。
func (h *PluginHandler) TrustConfig(c *gin.Context) {
	cfg, err := h.manager.TrustConfigSnapshot()
	if err != nil {
		errcode.Respond(c, errcode.PluginTrustConfigUnreadable.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, cfg)
}

type pluginTrustConfigReq struct {
	TrustedPublicKeys []string `json:"trusted_public_keys" binding:"dive,max=2048"` // ed25519 base64 公钥约 68 字符，给宽上界
	AllowUnsigned     bool     `json:"allow_unsigned"`
}

// SetTrustConfig 持久化插件签名信任配置（受信公钥 + 是否允许未签名）。官方公钥只读不可改。
func (h *PluginHandler) SetTrustConfig(c *gin.Context) {
	var req pluginTrustConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.manager.SetTrustConfig(req.TrustedPublicKeys, req.AllowUnsigned); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	cfg, err := h.manager.TrustConfigSnapshot()
	if err != nil {
		// 这两条记录是分开写的，写完却读不回来时不能照旧回一份「看起来保存成功」的配置。
		errcode.Respond(c, errcode.PluginTrustConfigUnreadable.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *PluginHandler) Market(c *gin.Context) {
	var (
		idx *plugins.MarketIndex
		err error
	)
	if q := c.Query("source"); q != "" {
		// 单源覆盖（向后兼容）
		idx, err = h.manager.FetchMarket(c.Request.Context(), q)
	} else {
		// 聚合全部已配置源
		idx, err = h.manager.FetchMarketMulti(c.Request.Context(), nil)
	}
	if err != nil {
		// 市场列表取不到是上游问题，不是用户填错了参数。
		errcode.Respond(c, errcode.PluginMarketUnreachable.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, idx)
}

// pluginInstallFailure 按失败语义挑状态码：市场侧、包侧、宿主侧、本机侧四类各有各的下一步动作。
// 从前这里一律 InvalidInput（400「请求参数有误」），于是「包被改过」这种安全事件和
// 「市场源挂了」都被说成用户手误：前者让人以为填错了表而不去核对源，后者让人拿同一个坏包反复重试。
func pluginInstallFailure(err error) *errcode.Error {
	switch {
	case errors.Is(err, plugins.ErrUntrustedPackage):
		return errcode.PluginPackageUntrusted
	case errors.Is(err, plugins.ErrPackageInvalid):
		return errcode.PluginPackageInvalid
	case errors.Is(err, plugins.ErrMarketUnreachable):
		return errcode.PluginMarketUnreachable
	case errors.Is(err, plugins.ErrPluginNotInMarket):
		return errcode.PluginNotInMarket
	case errors.Is(err, plugins.ErrPluginNotInstallable):
		return errcode.PluginNotInstallable
	case errors.Is(err, plugins.ErrInstallWriteFailed):
		return errcode.PluginInstallFailed
	default:
		// 其余是请求侧问题（缺 id、权限未确认）——这一档确实是 400。
		return errcode.InvalidInput
	}
}

func (h *PluginHandler) Install(c *gin.Context) {
	var in pluginInstallReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	options := plugins.MarketInstallOptions{
		AcceptedPermissions:         in.AcceptedPermissions,
		AcceptedPermissionsByPlugin: make(map[string][]string, len(in.AcceptedPermissionsByPlugin)),
	}
	for _, item := range in.AcceptedPermissionsByPlugin {
		if item.ID == "" {
			continue
		}
		options.AcceptedPermissionsByPlugin[item.ID] = item.AcceptedPermissions
	}
	if len(options.AcceptedPermissionsByPlugin) == 0 {
		options.AcceptedPermissionsByPlugin = nil
	}
	out, err := h.manager.InstallFromMarketWithOptions(c.Request.Context(), in.Source, in.ID, options)
	if err != nil {
		errcode.Respond(c, pluginInstallFailure(err).WithErr(err))
		return
	}
	c.JSON(http.StatusOK, out)
}

// errAcceptedPermissionsBad 只用来标「请求体本身读不出」这一档，
// 免得它和「用户还没确认权限」在界面变成同一句话。
var errAcceptedPermissionsBad = errors.New("bad accepted_permissions body")

func (h *PluginHandler) validateAcceptedPermissions(c *gin.Context) error {
	if c.Request.ContentLength == 0 {
		return nil
	}
	var in pluginPermissionReq
	if err := c.ShouldBindJSON(&in); err != nil {
		// 真的是 body 不合形状（改请求有用），单独一档，不与「没确认权限」混在一起。
		return fmt.Errorf("%w（请求体里的 accepted_permissions 无法解析）: %w", errAcceptedPermissionsBad, err)
	}
	if in.AcceptedPermissions == nil {
		return fmt.Errorf("%w：这个插件声明了权限清单，启用前需要在弹窗里确认（界面上发 accepted_permissions 数组，可为空数组）；请重新打开插件页后再启用", plugins.ErrPluginPermissionsPending)
	}
	p, err := h.manager.Get(c.Param("id"))
	if err != nil {
		return err
	}
	if !plugins.PermissionSetEqual(in.AcceptedPermissions, p.Permissions) {
		return fmt.Errorf("%w：清单与插件当前声明的权限不一致（插件可能刚更新过），请重新打开插件页再启用", plugins.ErrPluginPermissionsPending)
	}
	return nil
}
