package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// RegistrationHandler 处理注册开放检测的监控目标 CRUD 与手动检测。
type RegistrationHandler struct {
	op    *oper.RegistrationOper
	regCh *chain.RegistrationChain
}

// NewRegistrationHandler 构造。
func NewRegistrationHandler(op *oper.RegistrationOper, regCh *chain.RegistrationChain) *RegistrationHandler {
	return &RegistrationHandler{op: op, regCh: regCh}
}

// List 列出全部监控目标。
func (h *RegistrationHandler) List(c *gin.Context) {
	ws, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"registrations": ws})
}

// Create 添加监控目标。
func (h *RegistrationHandler) Create(c *gin.Context) {
	var in oper.RegistrationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.URL == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if in.Name == "" {
		in.Name = in.URL
	}
	w, err := h.op.Create(in)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, w)
}

// Update 更新监控目标。
func (h *RegistrationHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.RegistrationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	w, err := h.op.Update(id, in)
	if err != nil {
		// oper.Update 先 Get 再 Save：目标已被删除时抛的是 record not found，
		// 报 500 会让用户以为服务端坏了，其实是列表里的行早没了。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			errcode.Respond(c, errcode.RegistrationNotFound)
			return
		}
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, w)
}

// Delete 删除监控目标。
// respondRegistrationWriteErr 注册检测域写失败的出口（#396）。
//
// 三个写出口语义不同，但都归同一档「写不进去」：
//
//	Delete        纯 DB 写；失败＝什么都没删（幂等，重试安全）。
//	TouchChecked  只刷新「最近查过」时间戳、刻意不动 last_open；失败＝站点开/关结论不受影响。
//	UpdateStatus  回写「是否开放」这个业务结论；失败＝这一轮检测白跑了，
//	               下一轮还会读到旧值（链层那条路径有日志并 continue）。
//
// 旧形状三处都是 Internal.WithErr ⇒ 500「服务内部错误，请稍后重试」，而这条路径
// **零测试覆盖**（delete_missing_more_test.go 只测 Delete）、读者又主要是 cron 而非界面
// ⇒ 失败时没有提示、只有 slog，运维只看到「注册状态不更新」，查不出是磁盘满还是连接断。
// 所以文案落在「原因可查」上（而不是「用户该点哪里」——这条路径没有界面）。
func respondRegistrationWriteErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.RegistrationWriteFailed.WithDetailKeepMessage(err))
}

func (h *RegistrationHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.op.Get, id, errcode.RegistrationNotFound) {
		return
	}
	if err := h.op.Delete(id); err != nil {
		respondRegistrationWriteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Check 手动触发单个目标的注册检测，回写状态并返回结果。
func (h *RegistrationHandler) Check(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	w, err := h.op.Get(id)
	if err != nil {
		errcode.Respond(c, errcode.RegistrationNotFound)
		return
	}
	open, decided, err := h.regCh.CheckOne(c.Request.Context(), w)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "open": false, "error": userErrText(err)})
		return
	}
	if !decided {
		// 无法判定（被拦截 / 页面认不出标记）：保留原状态，别把它写成「关闭」。
		if err := h.op.TouchChecked(id); err != nil {
			respondRegistrationWriteErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "decided": false})
		return
	}
	if _, err := h.op.UpdateStatus(id, open); err != nil {
		respondRegistrationWriteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "open": open, "decided": true})
}
