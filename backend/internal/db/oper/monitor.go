package oper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// MonitorOper 封装监控目标/快照/告警的 CRUD 与凭据加解密。
// 同时隐式实现 monitor.Store 接口（供 monitor.Service 依赖，避免循环引用）。
type MonitorOper struct {
	db *gorm.DB
	c  *security.Crypto
}

// NewMonitorOper 构造。
func NewMonitorOper(db *gorm.DB, c *security.Crypto) *MonitorOper {
	return &MonitorOper{db: db, c: c}
}

// WithTx 返回绑定到指定事务的副本（保留 Crypto 句柄），用于跨表原子写。
// 不改变原 oper；调用方负责 tx 的提交/回滚。
func (o *MonitorOper) WithTx(tx *gorm.DB) *MonitorOper {
	return &MonitorOper{db: tx, c: o.c}
}

// TargetInput 创建/更新监控目标入参（凭据明文，由 Oper 加密）。
type TargetInput struct {
	Name         string `json:"name" binding:"max=64"`
	Type         string `json:"type" binding:"max=16"` // host | nas | camera
	Host         string `json:"host" binding:"max=255"`
	Port         int    `json:"port"`
	Vendor       string `json:"vendor" binding:"max=32"`
	Protocol     string `json:"protocol" binding:"max=16"`
	CredentialID uint   `json:"credential_id"`
	RTSPURL      string `json:"rtsp_url" binding:"max=512"`
	IntervalSec  int    `json:"interval_sec"`
	TimeoutSec   int    `json:"timeout_sec"`
	Username     string `json:"username" binding:"max=256"`
	Password     string `json:"password" binding:"max=512"`
	PrivateKey   string `json:"private_key" binding:"max=8192"` // SSH 私钥 PEM（明文，Oper 加密；空=不改/不设）
	Thresholds   string `json:"thresholds" binding:"max=5000"`
	DiskPaths    string `json:"disk_paths" binding:"max=2000"` // host/nas+local：宿主 bind mount 目录（换行/逗号分隔）
	Enabled      bool   `json:"enabled"`
}

// Create 创建监控目标。
func (o *MonitorOper) Create(in TargetInput) (*models.MonitorTarget, error) {
	if err := validateTargetInput(in); err != nil {
		return nil, err
	}
	if in.RTSPURL != "" {
		if err := validateRTSPScheme(in.RTSPURL); err != nil {
			return nil, err
		}
	}
	t := &models.MonitorTarget{
		Name: in.Name, Type: in.Type, Host: in.Host, Port: in.Port,
		Vendor: in.Vendor, Protocol: in.Protocol, CredentialID: in.CredentialID, RTSPURL: in.RTSPURL,
		IntervalSec: normInterval(in.IntervalSec), TimeoutSec: normTimeout(in.TimeoutSec),
		Enabled: in.Enabled, Thresholds: in.Thresholds, DiskPaths: in.DiskPaths,
	}
	if err := o.ensureCredentialExists(in.CredentialID); err != nil {
		return nil, err
	}
	if err := o.setCreds(t, in.Username, in.Password, in.PrivateKey); err != nil {
		return nil, err
	}
	if err := o.db.Create(t).Error; err != nil {
		return nil, err
	}
	// GORM default:true 会把 Enabled=false 当零值吞为 true → 显式写回（同 SiteOper.Create）。
	if !in.Enabled {
		if err := o.db.Model(t).Update("enabled", false).Error; err != nil {
			return nil, err
		}
	}
	o.fillUsername(t)
	return t, nil
}

// Update 更新（密码/用户名为空则保留）。
func (o *MonitorOper) Update(id uint, in TargetInput) (*models.MonitorTarget, error) {
	t, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if err := validateTargetInput(in); err != nil {
		return nil, err
	}
	if in.RTSPURL != "" {
		if err := validateRTSPScheme(in.RTSPURL); err != nil {
			return nil, err
		}
	}
	t.Name, t.Type, t.Host, t.Port = in.Name, in.Type, in.Host, in.Port
	t.Vendor, t.Protocol, t.CredentialID, t.RTSPURL = in.Vendor, in.Protocol, in.CredentialID, in.RTSPURL
	t.IntervalSec = normInterval(in.IntervalSec)
	t.TimeoutSec = normTimeout(in.TimeoutSec)
	t.Enabled, t.Thresholds, t.DiskPaths = in.Enabled, in.Thresholds, in.DiskPaths
	if err := o.ensureCredentialExists(in.CredentialID); err != nil {
		return nil, err
	}
	// 凭据加密失败必须中止（对齐 Create 的 setCreds）：否则保留旧密文、API 误报成功，
	// 用户改密码后监控仍用旧凭证反复鉴权失败。
	if in.Username != "" {
		enc, err := o.c.Encrypt(in.Username)
		if err != nil {
			return nil, err
		}
		t.UsernameEnc = enc
	}
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		t.PasswordEnc = enc
	}
	if in.PrivateKey != "" {
		enc, err := o.c.Encrypt(in.PrivateKey)
		if err != nil {
			return nil, err
		}
		t.PrivateKeyEnc = enc
	}
	if err := o.db.Save(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (o *MonitorOper) setCreds(t *models.MonitorTarget, username, password, privateKey string) error {
	if username != "" {
		enc, err := o.c.Encrypt(username)
		if err != nil {
			return err
		}
		t.UsernameEnc = enc
	}
	if password != "" {
		enc, err := o.c.Encrypt(password)
		if err != nil {
			return err
		}
		t.PasswordEnc = enc
	}
	if privateKey != "" {
		enc, err := o.c.Encrypt(privateKey)
		if err != nil {
			return err
		}
		t.PrivateKeyEnc = enc
	}
	return nil
}

// List 列出全部。
func (o *MonitorOper) List() ([]models.MonitorTarget, error) {
	var ts []models.MonitorTarget
	err := o.db.Order("id asc").Find(&ts).Error
	if err == nil {
		for i := range ts {
			o.fillUsername(&ts[i])
		}
	}
	return ts, err
}

// ListEnabled 列出启用的。
func (o *MonitorOper) ListEnabled() ([]models.MonitorTarget, error) {
	var ts []models.MonitorTarget
	err := o.db.Where("enabled = ?", true).Order("id asc").Find(&ts).Error
	return ts, err
}

// Get 取单个。
func (o *MonitorOper) Get(id uint) (*models.MonitorTarget, error) {
	var t models.MonitorTarget
	err := o.db.First(&t, id).Error
	if err == nil {
		o.fillUsername(&t)
	}
	return &t, err
}

// Delete 删除（连带删快照/告警，单事务保证不残留孤儿行；对齐 RssOper/WorkflowOper 写法）。
func (o *MonitorOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.MonitorTarget{}, id).Error; err != nil {
			return err
		}
		tx.Where("target_id = ?", id).Delete(&models.MetricSnapshot{})
		tx.Where("target_id = ?", id).Delete(&models.MonitorAlert{})
		return nil
	})
}

// Credentials 解密返回凭据（无凭据时字段为空，不报错）。
func (o *MonitorOper) Credentials(t *models.MonitorTarget) (monitor.Credentials, error) {
	creds := monitor.Credentials{}
	if t.CredentialID > 0 {
		var c models.OpsCredential
		if err := o.db.First(&c, t.CredentialID).Error; err != nil {
			return creds, err
		}
		if c.UsernameEnc != "" {
			if u, err := o.c.Decrypt(c.UsernameEnc); err == nil {
				creds.Username = u
			} else {
				return creds, err
			}
		}
		if c.PasswordEnc != "" {
			if p, err := o.c.Decrypt(c.PasswordEnc); err == nil {
				creds.Password = p
			} else {
				return creds, err
			}
		}
		if c.PrivateKeyEnc != "" {
			if k, err := o.c.Decrypt(c.PrivateKeyEnc); err == nil {
				creds.PrivateKey = k
			} else {
				return creds, err
			}
		}
		return creds, nil
	}
	if t.UsernameEnc != "" {
		if u, err := o.c.Decrypt(t.UsernameEnc); err == nil {
			creds.Username = u
		} else {
			return creds, err
		}
	}
	if t.PasswordEnc != "" {
		if p, err := o.c.Decrypt(t.PasswordEnc); err == nil {
			creds.Password = p
		} else {
			return creds, err
		}
	}
	if t.PrivateKeyEnc != "" {
		if k, err := o.c.Decrypt(t.PrivateKeyEnc); err == nil {
			creds.PrivateKey = k
		} else {
			return creds, err
		}
	}
	return creds, nil
}

// fillUsername 解密回填 transient Username 与 HasPrivateKey（仅供 JSON 回显，编辑表单用）；
// 明文密码/私钥不回传，编辑时留空保留。
func (o *MonitorOper) fillUsername(t *models.MonitorTarget) {
	if t.CredentialID > 0 {
		var c models.OpsCredential
		if err := o.db.First(&c, t.CredentialID).Error; err == nil {
			t.CredentialName = c.Name
			if c.UsernameEnc != "" {
				if u, err := o.c.Decrypt(c.UsernameEnc); err == nil {
					t.Username = u
				}
			}
			t.HasPrivateKey = c.PrivateKeyEnc != ""
			return
		}
	}
	if t.UsernameEnc != "" {
		if u, err := o.c.Decrypt(t.UsernameEnc); err == nil {
			t.Username = u
		}
	}
	t.HasPrivateKey = t.PrivateKeyEnc != ""
}

func (o *MonitorOper) ensureCredentialExists(id uint) error {
	if id == 0 {
		return nil
	}
	var n int64
	if err := o.db.Model(&models.OpsCredential{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("凭据不存在")
	}
	return nil
}

// RecordSnapshot 写快照并更新 target 状态；对比上次快照算网络速率（service 委托）。
// 返回填充了 NetRxRate/NetTxRate 的 Metrics 副本 + 持久化错误。
// 注意：必须把算好的速率回传调用方——CollectNow 若只 return 原始 m，详情页网速会恒为 0。
// collectErr（采集错误）已写入 last_error 字段，不影响此处返回的持久化错误。
func (o *MonitorOper) RecordSnapshot(t *models.MonitorTarget, m monitor.Metrics, collectErr error) (monitor.Metrics, error) {
	now := time.Now()
	online := m.Online && collectErr == nil
	if online {
		o.computeNetRate(t.ID, now, &m)
	}
	snap := &models.MetricSnapshot{
		TargetID: t.ID, SnapAt: now, Online: online, Metrics: monitor.MetricsJSON(m),
	}
	var firstErr error
	if err := o.db.Create(snap).Error; err != nil {
		firstErr = fmt.Errorf("save snapshot: %w", err)
	}
	errMsg := ""
	if collectErr != nil {
		errMsg = truncateErr(collectErr.Error())
	}
	if err := o.db.Model(t).Updates(map[string]any{
		"last_checked_at": now,
		"last_error":      errMsg,
		"last_online":     online,
	}).Error; err != nil && firstErr == nil {
		firstErr = fmt.Errorf("update target status: %w", err)
	}
	return m, firstErr
}

// computeNetRate 对比上一条快照的累计字节算收发速率。
func (o *MonitorOper) computeNetRate(targetID uint, now time.Time, m *monitor.Metrics) {
	var prev models.MetricSnapshot
	if err := o.db.Where("target_id = ?", targetID).Order("snap_at desc").First(&prev).Error; err != nil {
		return
	}
	var pm monitor.Metrics
	if json.Unmarshal([]byte(prev.Metrics), &pm) != nil {
		return
	}
	dt := now.Sub(prev.SnapAt).Seconds()
	if dt <= 0 {
		return
	}
	if m.NetRxBytes >= pm.NetRxBytes {
		m.NetRxRate = uint64(float64(m.NetRxBytes-pm.NetRxBytes) / dt)
	}
	if m.NetTxBytes >= pm.NetTxBytes {
		m.NetTxRate = uint64(float64(m.NetTxBytes-pm.NetTxBytes) / dt)
	}
}

// UpdateRTSPURL 回填摄像头 RTSP 拉流地址（ONVIF GetStreamUri 获取后）。
func (o *MonitorOper) UpdateRTSPURL(id uint, url string) error {
	return o.db.Model(&models.MonitorTarget{}).Where("id = ?", id).Update("rtsp_url", url).Error
}

// ListSnapshots 取最近 limit 条快照，再按时间升序返回（便于前端绘图与取最新）。
// 绝不能 Order(asc)+Limit：那是「最旧 N 条」。卡片 limit=1 会永远显示第一笔旧数据
//（派糖磁盘修完后详情对、卡片仍 3.9TB 即此 bug）。正确：DESC 取最近 N，再反转为 ASC。
func (o *MonitorOper) ListSnapshots(targetID uint, since time.Time, limit int) ([]models.MetricSnapshot, error) {
	var snaps []models.MetricSnapshot
	q := o.db.Where("target_id = ?", targetID)
	if !since.IsZero() {
		q = q.Where("snap_at >= ?", since)
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	if err := q.Order("snap_at desc").Limit(limit).Find(&snaps).Error; err != nil {
		return nil, err
	}
	// 反转为升序：索引 0 最旧、末项最新——与前端 parseMetrics(snaps[last]) / 图表序列约定一致。
	for i, j := 0, len(snaps)-1; i < j; i, j = i+1, j-1 {
		snaps[i], snaps[j] = snaps[j], snaps[i]
	}
	return snaps, nil
}

// CleanupSnapshots 删除 before 之前的快照。
func (o *MonitorOper) CleanupSnapshots(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ?", before).Delete(&models.MetricSnapshot{})
	return res.RowsAffected, res.Error
}

// CleanupAlerts 删除 before 之前已解决的告警（resolved_at 非空）。未解决的活跃告警保留。
func (o *MonitorOper) CleanupAlerts(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ? AND resolved_at IS NOT NULL", before).Delete(&models.MonitorAlert{})
	return res.RowsAffected, res.Error
}

// ListAlerts 列告警。
func (o *MonitorOper) ListAlerts(targetID uint, unreadOnly bool) ([]models.MonitorAlert, error) {
	var alerts []models.MonitorAlert
	q := o.db.Order("created_at desc").Limit(200)
	if targetID > 0 {
		q = q.Where("target_id = ?", targetID)
	}
	if unreadOnly {
		q = q.Where("read = ?", false)
	}
	err := q.Find(&alerts).Error
	return alerts, err
}

// MarkAlertRead 标已读。
func (o *MonitorOper) MarkAlertRead(id uint) error {
	return o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Update("read", true).Error
}

// AckAlert marks an alert as acknowledged and read.
func (o *MonitorOper) AckAlert(id uint, by string) error {
	now := time.Now()
	return o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Updates(map[string]any{
		"read":            true,
		"acknowledged_at": now,
		"acknowledged_by": by,
	}).Error
}

// SilenceAlert suppresses repeated notifications for an open alert until the given time.
func (o *MonitorOper) SilenceAlert(id uint, until time.Time, by string) error {
	now := time.Now()
	return o.db.Model(&models.MonitorAlert{}).Where("id = ?", id).Updates(map[string]any{
		"read":            true,
		"acknowledged_at": now,
		"acknowledged_by": by,
		"silenced_until":  until,
	}).Error
}

// CreateAlert 新建告警记录。
func (o *MonitorOper) CreateAlert(a *models.MonitorAlert) error {
	return o.db.Create(a).Error
}

// ResolveOpenAlerts 把同 target+metric 未恢复的告警标记为已恢复。
func (o *MonitorOper) ResolveOpenAlerts(targetID uint, metric string) error {
	return o.db.Model(&models.MonitorAlert{}).
		Where("target_id = ? AND metric = ? AND resolved_at IS NULL", targetID, metric).
		Update("resolved_at", time.Now()).Error
}

// LastOpenAlert 取最近一条未恢复的告警（状态转换去重用）。无则返回 nil。
func (o *MonitorOper) LastOpenAlert(targetID uint, metric string) (*models.MonitorAlert, error) {
	var a models.MonitorAlert
	err := o.db.Where("target_id = ? AND metric = ? AND resolved_at IS NULL", targetID, metric).
		Order("id desc").First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func normInterval(v int) int {
	if v <= 0 {
		return 60
	}
	return v
}

func normTimeout(v int) int {
	if v <= 0 {
		return 15
	}
	return v
}

func truncateErr(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

// validateRTSPScheme 仅允许 rtsp/rtsps（拒绝其它 scheme，防协议滥用）。
func validateRTSPScheme(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid rtsp url: %w", err)
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return fmt.Errorf("rtsp url scheme must be rtsp/rtsps, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("rtsp url must have a host")
	}
	return nil
}

func validateTargetInput(in TargetInput) error {
	switch in.Type {
	case "http":
		u, err := url.Parse(strings.TrimSpace(in.Host))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("HTTP 监控地址必须是 http/https 完整 URL")
		}
	case "tcp":
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("TCP 监控地址不能为空")
		}
		if in.Port <= 0 || in.Port > 65535 {
			return fmt.Errorf("TCP 端口必须在 1-65535 之间")
		}
	case "process":
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("进程监控需要填写 SSH 主机地址")
		}
		if in.Protocol != "" && in.Protocol != "ssh" {
			return fmt.Errorf("进程监控仅支持 SSH 协议")
		}
		if strings.TrimSpace(monitor.ParseThresholds(in.Thresholds).ProcessName) == "" {
			return fmt.Errorf("进程监控需要填写进程名称")
		}
	case "script":
		if strings.TrimSpace(in.Host) == "" {
			return fmt.Errorf("脚本监控需要填写 SSH 主机地址")
		}
		if in.Protocol != "" && in.Protocol != "ssh" {
			return fmt.Errorf("脚本监控仅支持 SSH 协议")
		}
		if strings.TrimSpace(monitor.ParseThresholds(in.Thresholds).ScriptCommand) == "" {
			return fmt.Errorf("脚本监控需要填写命令")
		}
	}
	return nil
}
