package v1

import (
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
	"gorm.io/gorm"
)

type PluginHandler struct {
	manager *plugins.Manager
	tickets *auth.TicketStore
}

const pluginAssetTicketTTL = 2 * time.Minute
const pluginAssetTicketCookieName = "ptp_plugin_asset_ticket"

func NewPluginHandler(manager *plugins.Manager, tickets *auth.TicketStore) *PluginHandler {
	return &PluginHandler{manager: manager, tickets: tickets}
}

type pluginConfigReq struct {
	Config map[string]any `json:"config"`
}

type pluginActionReq struct {
	Action string         `json:"action" binding:"max=64"`
	Input  map[string]any `json:"input"`
}

type pluginEventReq struct {
	Type string         `json:"type" binding:"max=64"`
	Data map[string]any `json:"data"`
}

type pluginBridgeReq struct {
	Method string         `json:"method" binding:"max=16"`
	Params map[string]any `json:"params"`
}

type pluginPermissionReq struct {
	AcceptedPermissions []string `json:"accepted_permissions" binding:"dive,max=64"`
}

type pluginRollbackReq struct {
	ReleaseID uint `json:"release_id" binding:"min=1"`
}

func pluginAssetScope(id string) string {
	return "plugin-assets:" + id
}

func (h *PluginHandler) List(c *gin.Context) {
	ps, err := h.manager.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugins": ps})
}

func (h *PluginHandler) Get(c *gin.Context) {
	p, err := h.manager.Get(c.Param("id"))
	if err != nil {
		errcode.Respond(c, errcode.NotFound.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *PluginHandler) Enable(c *gin.Context) {
	if err := h.validateAcceptedPermissions(c); err != nil {
		errcode.Respond(c, pluginStateError(err))
		return
	}
	if err := h.manager.SetEnabled(c.Param("id"), true); err != nil {
		errcode.Respond(c, pluginStateError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}

func (h *PluginHandler) Disable(c *gin.Context) {
	if err := h.manager.SetEnabled(c.Param("id"), false); err != nil {
		errcode.Respond(c, pluginStateError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": false})
}

func (h *PluginHandler) SaveConfig(c *gin.Context) {
	var in pluginConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	p, err := h.manager.SaveConfig(c.Param("id"), in.Config)
	if err != nil {
		// 「插件没启用」「设置读不出来」不该和「配置值不合插件心意」挤在同一句 400 里（#287）。
		errcode.Respond(c, pluginActionError(err))
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *PluginHandler) Action(c *gin.Context) {
	var in pluginActionReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.Input == nil {
		in.Input = map[string]any{}
	}
	out, err := h.manager.ExecuteAction(c.Request.Context(), c.Param("id"), in.Action, in.Input)
	if err != nil {
		// 插件动作跑挂不是用户的错：兜底给 plugin_action_failed（502，并把去处指向插件日志），
		// 不再回「请求参数有误，请检查后重试」——那句话把插件崩溃、进程掉线、超时全推给输入框。
		// 「动作名写错」由 ErrUnknownAction 单独留在 400（改输入有用），SaveConfig 仍按原口径兜底 400。
		errcode.Respond(c, pluginActionErrorWith(err, errcode.PluginActionFailed))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
}

func (h *PluginHandler) Logs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	pid := c.Query("plugin_id")
	limit = oper.NormalizePluginLogLimit(limit)
	logs, err := h.manager.Logs(pid, limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// total/truncated 与清单同批给出；计数读不出时保守报截断并省略 total，不编 0。
	total, cntErr := h.manager.CountLogs(pid)
	truncated := cntErr != nil || total > int64(len(logs))
	if cntErr != nil {
		slog.Warn("plugin log count failed", "plugin_id", pid, "err", cntErr)
	}
	resp := gin.H{"logs": logs, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

func (h *PluginHandler) Asset(c *gin.Context) {
	if _, ok := c.Get("uid"); !ok {
		if h.tickets == nil {
			errcode.Respond(c, errcode.UnauthorizedOp)
			return
		}
		ticket := c.Query("ticket")
		if ticket == "" {
			if cookie, err := c.Cookie(pluginAssetTicketCookieName); err == nil {
				ticket = cookie
			}
		}
		if _, ok := h.tickets.Validate(ticket, pluginAssetScope(c.Param("id"))); !ok {
			errcode.Respond(c, errcode.TicketInvalidOrExpired)
			return
		}
	}
	asset, err := h.manager.ReadAsset(c.Param("id"), c.Param("path"))
	if err != nil {
		errcode.Respond(c, errcode.NotFound.WithErr(err))
		return
	}
	c.Data(http.StatusOK, asset.ContentType, asset.Data)
}

func (h *PluginHandler) AssetTicket(c *gin.Context) {
	if h.tickets == nil {
		errcode.Respond(c, errcode.DependencyMissing)
		return
	}
	tk, exp, err := h.tickets.IssueScoped(c.GetUint("uid"), pluginAssetScope(c.Param("id")), pluginAssetTicketTTL)
	if err != nil {
		// #346 不是上游故障：IssueScoped 是纯进程内操作（生成随机字节→写内存 map→
		// 清过期项），唯一会失败的是 rand.Read（本机随机源）。此前报 UpstreamFailed
		// （502「上游服务不可用」）把用户指向网络/第三方服务，方向完全错；
		// 且 err 未带，日志里也查不到。这里回本机故障码并挂上原因。
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		pluginAssetTicketCookieName,
		tk,
		int(time.Until(exp).Seconds()),
		"/api/v1/plugins/"+c.Param("id")+"/assets/",
		"",
		false,
		true,
	)
	c.JSON(http.StatusOK, gin.H{
		"ticket":     tk,
		"expires_at": exp,
		"expires_in": int(time.Until(exp).Seconds()),
	})
}

func (h *PluginHandler) Releases(c *gin.Context) {
	releases, err := h.manager.ListReleases(c.Param("id"), 50)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"releases": releases})
}

func (h *PluginHandler) Bridge(c *gin.Context) {
	var in pluginBridgeReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	out, err := h.manager.Bridge(c.Request.Context(), c.Param("id"), in.Method, in.Params)
	if err != nil {
		// 插件动作跑挂不是「请求参数有误」：那是插件自己的故障，重试同一个动作不会变好（#287）。
		errcode.Respond(c, pluginActionErrorWith(err, errcode.PluginActionFailed))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": out})
}

// pluginRollbackFailure 把回滚失败分成五种真实处境。从前一律 400「请求参数有误」，
// 而回滚是破坏性动作——每次点都会先停插件、再把当前目录改名出去：
// 「版本记录没了 / 快照文件没了 / 没配插件目录 / 旧目录没能放回 / 账面没写进去」
// 没有一种能靠改输入框解决，被说成参数有误只会换来反复重试。
func pluginRollbackFailure(err error) *errcode.Error {
	switch {
	case errors.Is(err, plugins.ErrRollbackRestoreFailed):
		// WithDetail 会拿底层 Go 错误顶掉注册表里的中文文案（响应体只给 Message），
		// 这里要留中文、把路径与原始错误交给日志（Respond 对 5xx 记 WARN）。
		return errcode.PluginRollbackRestoreFailed.WithErr(err)
	case errors.Is(err, plugins.ErrRollbackLedgerNotWritten):
		return errcode.PluginRollbackLedgerUnwritten.WithErr(err)
	case errors.Is(err, plugins.ErrPluginRootUnconfigured):
		return errcode.PluginRootUnconfigured
	case errors.Is(err, plugins.ErrReleaseNotForPlugin):
		return errcode.PluginReleaseMismatch
	case errors.Is(err, gorm.ErrRecordNotFound):
		return errcode.PluginReleaseNotFound
	case errors.Is(err, fs.ErrNotExist):
		return errcode.PluginRollbackNoSnapshot
	default:
		return errcode.Internal.WithDetail(err)
	}
}

func (h *PluginHandler) Rollback(c *gin.Context) {
	var in pluginRollbackReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	out, err := h.manager.Rollback(c.Request.Context(), c.Param("id"), in.ReleaseID)
	if err != nil {
		errcode.Respond(c, pluginRollbackFailure(err))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *PluginHandler) PublishEvent(c *gin.Context) {
	var in pluginEventReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.Type == "" {
		in.Type = "plugin.test"
	}
	if in.Data == nil {
		in.Data = map[string]any{"source": "api"}
	}
	h.manager.PublishEvent(c.Request.Context(), in.Type, in.Data)
	c.JSON(http.StatusOK, gin.H{"published": true, "type": in.Type})
}

func (h *PluginHandler) Reload(c *gin.Context) {
	out, err := h.manager.ReloadExternal(c.Param("id"))
	if err != nil {
		// 内置插件没有磁盘目录，重载必然撞 os 的 not-exist：说清「不用重载」，
		// 别让用户以为是自己的操作参数有问题。
		if errors.Is(err, fs.ErrNotExist) {
			errcode.Respond(c, errcode.PluginNotReloadable.WithErr(err))
			return
		}
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *PluginHandler) Uninstall(c *gin.Context) {
	if err := h.manager.Uninstall(c.Param("id")); err != nil {
		if errors.Is(err, plugins.ErrPluginStateNotCleared) {
			// 中文原因走 Message/WithDetail：WithErr 只进日志不进响应（#214 同族）。
			errcode.Respond(c, errcode.PluginStateNotCleared.WithDetail(err))
			return
		}
		if errors.Is(err, plugins.ErrPluginNotInstalled) {
			errcode.Respond(c, errcode.PluginNotInstalled)
			return
		}
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"uninstalled": c.Param("id")})
}

// pluginActionError 把插件动作的失败分成本来就该不同的四类。从前一律 400「请求参数有误」：
// 「该插件没启用」「本机没有这个插件」「设置读不出」都被推给用户去改输入，
// 而这三件事没有一件能靠改输入框解决（真机实测：not found 被报成参数有误）。
// pluginActionErrorWith 是同一条判据的可换兜底版本：已知的三档（设置读不出 / 未启用 / 没装）
// 各归各码，剩下的交给调用方那一句——因为「保存设置失败」和「插件动作跑挂」不是同一件事，
// 前者多半是用户改的配置值不合插件心意（400），后者是插件自己的故障（502，重试无用）。
func pluginActionErrorWith(err error, fallback *errcode.Error) *errcode.Error {
	switch {
	case errors.Is(err, plugins.ErrPluginConfigUnreadable):
		// 哨兵原文是英文（"plugin config unreadable"），WithDetail 会把它整句送进界面；
		// 这里要的是注册好的那句中文（去哪儿重新保存）。#287 的用例当场抓到。
		return errcode.PluginConfigUnreadable.WithErr(err)
	case errors.Is(err, plugins.ErrPluginDisabled):
		return errcode.PluginDisabled.WithErr(err)
	case errors.Is(err, plugins.ErrPluginNotRegistered), errors.Is(err, plugins.ErrPluginNotInstalled):
		return errcode.PluginNotInstalled.WithErr(err)
	case errors.Is(err, plugins.ErrUnknownAction):
		// 「这个插件没实现这个动作」确实是改输入有用的 400，但要把动作名和下一步给出来；
		// 链层那句是自产中文（带「可在插件面板查看它支持的动作」），所以 WithDetail 直送界面。
		return errcode.InvalidInput.WithDetail(err)
	default:
		return fallback.WithErr(err)
	}
}

func pluginActionError(err error) *errcode.Error {
	return pluginActionErrorWith(err, errcode.InvalidInput)
}

// pluginStateError 是「启用 / 停用」两个出口共用的判据（同一条判定不在两处各抄一遍）。
// 从前两者一律 InvalidInput：插件被别的已启用插件依赖、依赖没满足、权限清单没确认、
// 甚至本机根本没这个插件，全被说成「请求参数有误，请检查后重试」——而这几件事
// 没有一件能靠改输入框解决，用户只会反复点同一个开关。
func pluginStateError(err error) *errcode.Error {
	switch {
	case errors.Is(err, errAcceptedPermissionsBad):
		// body 真的读不出形状：这一档才该让用户检查请求内容。
		return errcode.InvalidInput.WithErr(err)
	case errors.Is(err, plugins.ErrPluginPermissionsPending):
		return errcode.PluginPermissionsPending.WithDetail(err)
	case errors.Is(err, plugins.ErrPluginStateRejected):
		return errcode.PluginStateChangeRejected.WithDetail(err)
	default:
		return pluginActionErrorWith(err, errcode.PluginStateChangeRejected)
	}
}
