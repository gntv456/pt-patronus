package api

import (
	"strings"

	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/automation"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"github.com/hxpt/ptpatronus/internal/savept"
	"github.com/hxpt/ptpatronus/internal/site"
	subauto "github.com/hxpt/ptpatronus/internal/subtitle/automation"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/ws"
	"gorm.io/gorm"
)

// Deps 聚合路由装配所需的全部依赖。
type Deps struct {
	Cfg                *config.Config
	DB                 *gorm.DB
	JWT                *security.JWT
	Act                *auth.Activation
	Crypto             *security.Crypto
	TVCache            *auth.TokenVersionCache
	Tickets            *auth.TicketStore
	OIDC               *auth.OIDCService
	VIPGraceDays       int
	SiteOp             *oper.SiteOper
	SiteCh             *chain.SiteChain
	Reg                *site.Registry
	SiteHC             *httpclient.Client
	DLOp               *oper.DownloaderOper
	DLCh               *chain.DownloadChain
	DLMgr              *manager.Manager
	Notif              *chain.NotificationChain
	NotifMgr           *notification.Manager
	NotifProvider      *chain.NotificationSettingsProvider
	CfgOp              *oper.SystemConfigOper
	MediaCh            *chain.MediaChain
	Recommend          *chain.RecommendChain
	FileOp             *oper.FileStorageOper
	FileCh             *chain.FileManagerChain
	FileMgr            *filemanager.Manager
	KeepSeed           *chain.KeepSeedChain
	Backup             *backup.Service
	Entertainment      *chain.EntertainmentChain
	Dashboard          *chain.DashboardChain
	WS                 *ws.Broadcaster
	RegOp              *oper.RegistrationOper
	RegCh              *chain.RegistrationChain
	Subscribe          *chain.SubscribeChain
	Workflow           *chain.WorkflowChain
	Rss                *chain.RssChain
	BrushFlow          *chain.BrushFlowChain
	Contract           *chain.ContractChain
	MediaServer        *chain.MediaServerChain
	Transfer           *chain.TransferChain
	AgentChat          *v1.AgentChatHandler
	LLMGateway         *v1.LLMGatewayHandler
	Doctor             *v1.DoctorHandler
	TgBot              *v1.TgBotHandler
	Servarr            *v1.ServarrHandler
	Cloud              *chain.CloudChain
	Subtitle           *chain.SubtitleChain
	SubtitleAutomation *subauto.Service
	Trakt              *chain.TraktChain
	CookieCloud        *chain.CookieCloudChain
	MCP                *mcp.Server
	Webhook            *v1.WebhookHandler
	MonOp              *oper.MonitorOper
	MonSvc             *monitor.Service
	RTSPProxy          *monitor.RTSPProxy
	SSHHostKeyOp       *oper.SSHHostKeyOper
	AgentOp            *oper.AgentOper
	AgentHub           *monitor.AgentWSHub
	Savept             *savept.Service
	Assessment         *chain.AssessmentChain
	Medal              *chain.MedalChain
	Notice             *notice.Service
	Update             *update.Service
	Filter             *filter.Store
	FilterEng          *filter.Engine
	Plugins            *plugins.Manager
	Automation         *automation.Service
}

// splitOrigins 把逗号分隔的 CORS 配置拆为来源切片（空切片表示开发全开）。
func splitOrigins(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// loginRPS 把「每分钟上限」换算为每秒令牌速率，默认 10/min。
func loginRPS(cfg *config.Config) float64 {
	m := cfg.LoginRatePerMin
	if m <= 0 {
		m = 10
	}
	return float64(m) / 60.0
}
