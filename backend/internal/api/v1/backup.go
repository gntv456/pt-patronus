package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// BackupHandler 处理配置导出/导入。
type BackupHandler struct {
	svc    *backup.Service
	notice *notice.Service
}

// NewBackupHandler 构造。ns 为站内通知中心（备份完成/导入成功发通知；nil 跳过）。
func NewBackupHandler(svc *backup.Service, ns *notice.Service) *BackupHandler {
	return &BackupHandler{svc: svc, notice: ns}
}

// Export 导出自身格式备份。默认明文 JSON（旧行为，前端引导用户自存）；带
// X-Backup-Password 头时口令加密（PBKDF2+AES-GCM 信封，后端加密审计项）——
// 备份文件含全量明文 cookie/passkey/下载器密码，落盘即应密文。
func (h *BackupHandler) Export(c *gin.Context) {
	data, err := h.svc.Export(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	enc := false
	if pw := c.GetHeader("X-Backup-Password"); pw != "" {
		data, err = backup.Encrypt(data, pw)
		if err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
		enc = true
	}
	c.Header("Content-Disposition", `attachment; filename="ptpatronus-backup.json"`)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if enc {
		c.Data(http.StatusOK, "application/json", data)
		if h.notice != nil {
			_ = h.notice.Notice(c.Request.Context(), notice.Input{
				Source: notice.SourceBackupDone,
				Title:  "备份导出完成",
				Body:   "配置已导出为口令加密文件",
				Level:  "success",
			}, false)
		}
		return
	}
	c.Data(http.StatusOK, "application/json", data)
	if h.notice != nil {
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceBackupDone,
			Title:  "备份导出完成",
			Body:   "配置已导出为 JSON 文件（未加密，请妥善保管）",
			Level:  "success",
		}, false)
	}
}

// Import 导入自身格式备份。加密信封必须带 X-Backup-Password；
// 口令错误回 backup_password_incorrect，缺口令回 backup_password_required。
// respondBackupDecryptErr 备份解密失败的三档归类。
//
// #375 Decrypt 的三条解析失败（encrypt.go 的 bad salt / bad iv / bad data）说的都是
// **文件本身坏了**，而从前一律回 InvalidInput.WithErr —— 英文原因进日志、界面只显示
// 「请求参数有误」，用户于是反复点导入、反复检查自己从没改过的表单。
// 文件损坏不可恢复，文案必须点明「重新导出」，否则用户会当成程序出错。
//
// 两档哨兵各归各位，不动它们的既有码：
//
//	ErrBadPassword      → BackupPasswordIncorrect（口令错，可重试）
//	ErrPasswordRequired → BackupPasswordRequired（压根没给口令）
//
// 其余（bad salt/iv/data）→ BackupFileCorrupt + 带上底层原因供定位。
func respondBackupDecryptErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, backup.ErrBadPassword):
		// GCM 认证失败在密码学上分不清「口令错」与「密文被损坏」，文案必须同时给出
		// 两条出路；只说「口令错误」时，文件坏掉（下载截断/误改）的用户会反复重试口令。
		errcode.Respond(c, errcode.BackupPasswordIncorrect.WithDetail(
			errors.New("解密失败：口令不正确，或备份文件已损坏。请先确认口令；若口令无误，请重新导出备份后再导入")))
	case errors.Is(err, backup.ErrPasswordRequired):
		errcode.Respond(c, errcode.BackupPasswordRequired)
	default:
		// WithErr 而不是 WithDetail：WithDetail 无条件把 Message 换成 err.Error()
		// （errcode.go），会把码自带的中文冲掉、退回英文原文（实测 got "bad salt: …"）。
		// 码的文案已经点明「重新导出」，底层英文只留给日志定位。
		errcode.Respond(c, errcode.BackupFileCorrupt.WithErr(err))
	}
}
func (h *BackupHandler) Import(c *gin.Context) {
	data, err := readBody(c)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if backup.NeedsPassword(data) {
		pw := c.GetHeader("X-Backup-Password")
		if pw == "" {
			errcode.Respond(c, errcode.BackupPasswordRequired)
			return
		}
		data, err = backup.Decrypt(data, pw)
		if err != nil {
			// #375 三档分开：口令错 / 没给口令 / 文件本身坏了。
			// 最后一档从前是 InvalidInput（400「没能指出具体是哪一项」）+ 一句英文
			// `bad salt` / `bad iv` / `bad data` 只进日志 —— 而真相是用户手上的文件坏了，
			// 他该重新导出一次。让他去查表单参数是白费功夫（同 #335 的形状）。
			respondBackupDecryptErr(c, err)
			return
		}
	}
	res, err := h.svc.Import(c.Request.Context(), data)
	if err != nil {
		// 「不是备份文件」「版本太新」是用户选错文件，要说清下一步，不能只甩一句参数有误。
		switch {
		case errors.Is(err, backup.ErrNotBackupFile):
			errcode.Respond(c, errcode.BackupNotOurFile.WithErr(err))
		case errors.Is(err, backup.ErrBackupVersionTooNew):
			errcode.Respond(c, errcode.BackupVersionTooNew.WithErr(err))
		default:
			// 走到这里的是批量写库 / 加密失败：这批站点在一个事务里 ⇒ 一个都没落库，
			// 说成「请求参数有误」会让人去改一个本来没问题的文件反复重试（#285）。
			// oper 自产的输入原因（如某个 base_url 不合法）仍由 respondRecordError 带回原话。
			respondRecordError(c, err, nil, errcode.InvalidInput, errcode.BackupWriteFailed)
		}
		return
	}
	empty := !shouldAnnounceImport(res.Sites, res.Downloaders)
	// 一条都没导入时不发「导入成功」通知：修复前空备份/选错文件也会留下一条成功记录。
	if h.notice != nil && shouldAnnounceImport(res.Sites, res.Downloaders) {
		body := fmt.Sprintf("已导入 %d 个站点、%d 个下载器", res.Sites, res.Downloaders)
		if res.SitesSkipped > 0 || res.DownloadersSkipped > 0 || res.DownloadersFailed > 0 {
			body += fmt.Sprintf("；跳过 %d 个站点、%d 个已存在的下载器、失败 %d 个下载器",
				res.SitesSkipped, res.DownloadersSkipped, res.DownloadersFailed)
		}
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceBackupImport,
			Title:  "备份导入成功",
			Body:   body,
			Level:  "success",
		}, false)
	}
	c.JSON(http.StatusOK, gin.H{
		"empty":                empty,
		"imported_sites":       res.Sites,
		"imported_downloaders": res.Downloaders,
		"sites_skipped":        res.SitesSkipped,
		"downloaders_skipped":  res.DownloadersSkipped,
		"downloaders_failed":   res.DownloadersFailed,
		"failures":             res.Failures,
	})
}

// shouldAnnounceImport 判断这次导入值不值得发「成功」通知。
// 一条都没导入时不发：选错文件/空备份/格式不匹配都会返回 0，而通知标题写的是「导入成功」——
// 收件箱里留下一条说谎的 success 记录，用户以为恢复已完成（自有格式早有 empty 判定，
// 外部格式 .ptd 漏了；判定只留这一份，三条导入路径共用）。
func shouldAnnounceImport(sites, downloaders int) bool { return sites+downloaders > 0 }

// ImportMoviePilot 导入 MoviePilot 站点 JSON。
func (h *BackupHandler) ImportMoviePilot(c *gin.Context) {
	data, err := readBody(c)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	res, err := h.svc.ImportMoviePilot(c.Request.Context(), data)
	if err != nil {
		// #257 给 .ptd 与自有格式各留了「该换个文件」的判据，这一路一条都没有：
		// 形状不对和库写不进都回 400「参数有误」，两件事被说成同一件（#285）。
		if errors.Is(err, backup.ErrNotMoviePilotFile) || errors.Is(err, backup.ErrNotBackupFile) {
			// 只用自己的中文文案，json 的英文原文留在日志里（WithErr）。
			errcode.Respond(c, errcode.BackupNotMoviePilotFile.WithErr(err))
			return
		}
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.BackupWriteFailed)
		return
	}
	// empty / sites_skipped 必须回出来：MP 导出里缺 name 的条目、以及注册域重复的站点
	// 都不在成功数里，只报「已导入 N 个站点」会把部分失败说成全部成功（#257）。
	c.JSON(http.StatusOK, gin.H{
		"imported_sites": res.Sites,
		"sites_skipped":  res.SitesSkipped,
		"empty":          !shouldAnnounceImport(res.Sites, 0),
	})
}

// ImportPTD 导入 PT-Depiler (.ptd) 备份（multipart 文件上传，field=file）。
// .ptd 是 ZIP（含 manifest/metadata/cookies），后端解压转换，重复站点跳过。
func (h *BackupHandler) ImportPTD(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		errcode.Respond(c, errcode.BackupFileInvalid)
		return
	}
	if fh.Size > maxBackupSize {
		errcode.Respond(c, errcode.BackupFileTooLarge)
		return
	}
	f, err := fh.Open()
	if err != nil {
		errcode.Respond(c, errcode.BackupReadFailed)
		return
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, f); err != nil {
		errcode.Respond(c, errcode.BackupReadFailed)
		return
	}
	res, err := h.svc.ImportPTD(c.Request.Context(), buf.Bytes())
	if err != nil {
		// 这些原因（缺 manifest / 已加密 / 非 ZIP / 无可用站点）都是写给用户看的，
		// 走统一分类器把原话带回响应；从前一律 InvalidInput.WithErr ⇒ 界面只剩
		// 「请求参数有误，请检查后重试」，用户不知道该去掉加密还是该换个文件（#257）。
		respondRecordError(c, err, nil, errcode.InvalidInput, errcode.InvalidInput)
		return
	}
	// 一条都没导入时不发「导入成功」通知：选错 .ptd / 空包也会留下一条成功记录，
	// 于是收件箱里「已导入 0 个站点」被当成成功（同自有格式 Import 的 empty 判定）。
	if h.notice != nil && shouldAnnounceImport(res.Sites, 0) {
		body := fmt.Sprintf("已从 .ptd 导入 %d 个站点", res.Sites)
		// 跳过数同样要进通知：只报成功数就是把「包里 20 条只进来 12 条」说成全部完成（#257）。
		if res.SitesSkipped > 0 {
			body += fmt.Sprintf("；跳过 %d 个站点（无可用 URL 或与现有站点重复）", res.SitesSkipped)
		}
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceBackupImport,
			Title:  "PT-Depiler 备份导入成功",
			Body:   body,
			Level:  "success",
		}, false)
	}
	c.JSON(http.StatusOK, gin.H{
		"imported_sites": res.Sites,
		"sites_skipped":  res.SitesSkipped,
		"empty":          !shouldAnnounceImport(res.Sites, 0),
	})
}

// maxBackupSize 限制备份导入请求体大小（备份 JSON 正常 <几 MB，防认证用户 POST 超大 body OOM）。
const maxBackupSize = 64 << 20

func readBody(c *gin.Context) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackupSize)
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}
