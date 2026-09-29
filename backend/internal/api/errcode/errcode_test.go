package errcode

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// allCodes 注册表全量清单（新增码时同步加一行，防漏测）。
func allCodes() []*Error {
	return []*Error{
		Internal,
		AdminRequired, TokenMismatch, InvalidToken, NotActivated, NotVIP, VIPNotVerified, VIPUnknown, VIPExpired,
		ChallengeMode, OIDCDeprecated, ConfirmRequired,
		RiskConfirmationRequired,
		InvalidInput, NotFound, BadGateway, Conflict, Unavailable,
		ForbiddenSQL, WriteSQLForbidden, MultiStatement, DuplicateSpend, SiteAuthRejected,
		AuthServiceUnavailable, AuthServerBusy, AuthTicketFailed, AuthCaptchaFailed,
		AuthBadCredentials, AuthCookieRequired, DemoDisabled, APIKeyNoRefresh, APIKeyNoLogout,
		InvalidKey, ValueTooLarge, ValueInvalidJSON, VIPCheckDisabled,
		OIDCNotConfigured, OIDCStateMismatch, SaveFailed,
		WebhookNotFound, WebhookInvalid, WebhookBadPayload, WebhookBadSignature, WebhookNameRequired,
		SiteNotFound, SiteInvalidID, SiteDuplicate,
		DownloaderNotFound, DownloaderInvalidInput, DownloaderConnFailed, DownloaderPushFailed,
		BackupPasswordRequired, BackupPasswordIncorrect,
		AgentNameRequired, AgentTypeInvalid, AgentNotFound, AgentMsgRequired, AgentConfirmGone,
		DockerLocalNotReady,
		DockerContainerNameRequired, ResourceNotFound, AuthTokenFailed, TicketInvalidOrExpired, SubscriptionNotFound,
		DiskInvalidMountpoint, TorrentHashRequired, MediaLibraryRequired, UrlRequired, ServarrCreateFailed,
		BackupReadFailed, BrushflowTaskNotFound, TransferTaskNotFound, RssSubscriptionGone, WorkflowNotFound,
		MediaKeywordRequired, MediaSourceIDRequired, DockerImageRefRequired, ComposeDirInvalid, ComposeFileNotFound, ComposeWriteFailed,
		BackupFileInvalid, BackupFileTooLarge, NameRequired, InvalidID, InvalidRequest, FieldRequired,
		ResourceStateInvalid, DependencyMissing, UnauthorizedOp, UpstreamFailed, OperationForbidden,
	}
}

// TestCodesUnique 码值全局唯一 + 非空 + 格式合法（小写/下划线/字母数字）。
func TestCodesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range allCodes() {
		if e.Code == "" {
			t.Fatalf("空 code: %+v", e)
		}
		if seen[e.Code] {
			t.Fatalf("重复 code: %s", e.Code)
		}
		seen[e.Code] = true
		for _, r := range e.Code {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
				t.Fatalf("code %q 含非法字符 %q（仅允许小写字母/数字/下划线）", e.Code, r)
			}
		}
		if e.Message == "" {
			t.Fatalf("code %s 缺少中文默认文案", e.Code)
		}
	}
}

// TestCodesHTTPStatus HTTP 状态码合法性：4xx/5xx 且与语义一致。
func TestCodesHTTPStatus(t *testing.T) {
	for _, e := range allCodes() {
		if e.HTTP < 400 || e.HTTP > 599 {
			t.Fatalf("code %s 的 HTTP=%d 不在 4xx/5xx 范围", e.Code, e.HTTP)
		}
	}
	if Internal.HTTP != http.StatusInternalServerError {
		t.Fatalf("internal 应为 500，got %d", Internal.HTTP)
	}
}

// TestErrorErrorString Error() 文案：无底层错误返 Message，有则附加细节。
func TestErrorErrorString(t *testing.T) {
	e := NotVIP
	if got := e.Error(); got != e.Message {
		t.Fatalf("无底层错误应返 Message，got %q", got)
	}
	wrapped := VIPExpired.WithErr(errors.New("upstream 503"))
	if !strings.Contains(wrapped.Error(), "upstream 503") {
		t.Fatalf("应包含底层细节: %q", wrapped.Error())
	}
	if !errors.Is(wrapped, VIPExpired) {
		t.Fatal("Unwrap 应支持 errors.Is 链")
	}
	var target *Error
	if !errors.As(wrapped, &target) || target.Code != VIPExpired.Code {
		t.Fatal("Unwrap 应支持 errors.As 解出 *Error")
	}
}

// TestFromError 归一：*Error 原样（含 wrapped）、普通 error 包装为 Internal 且细节保留在 Err。
func TestFromError(t *testing.T) {
	if got := FromError(AdminRequired); got != AdminRequired {
		t.Fatalf("*Error 应原样返回，got %+v", got)
	}
	wrapped := errors.Join(RiskConfirmationRequired, errors.New("extra"))
	if got := FromError(wrapped); got.Code != RiskConfirmationRequired.Code {
		t.Fatalf("wrapped 链应解出 *Error，got code=%s", got.Code)
	}
	sentinel := errors.New("db: connection refused")
	got := FromError(sentinel)
	if got.Code != Internal.Code || got.HTTP != 500 {
		t.Fatalf("普通 error 应归一 internal/500，got %s/%d", got.Code, got.HTTP)
	}
	if !errors.Is(got, sentinel) {
		t.Fatal("底层错误应可通过 errors.Is 追溯")
	}
	if FromError(nil) != nil {
		t.Fatal("nil 应返回 nil")
	}
}

// TestRespondEnvelope 信封形态：{code, message, error}，error 与 message 同值，状态码透传。
func TestRespondEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/vip", func(c *gin.Context) { Respond(c, VIPNotVerified) })
	r.GET("/boom", func(c *gin.Context) { Respond(c, errors.New("secret detail")) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/vip", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("应 403，got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"code":"vip_not_verified"`, `"message":`, `"error":`} {
		if !strings.Contains(body, want) {
			t.Fatalf("信封应含 %s:\n%s", want, body)
		}
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest("GET", "/boom", nil))
	if rec2.Code != 500 {
		t.Fatalf("普通 error 应 500，got %d", rec2.Code)
	}
	if strings.Contains(rec2.Body.String(), "secret detail") {
		t.Fatal("内部细节不应进响应体")
	}
	if !strings.Contains(rec2.Body.String(), `"code":"internal"`) {
		t.Fatalf("应归一 internal:\n%s", rec2.Body.String())
	}
}
