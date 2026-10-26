package v1

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DatabaseHandler 提供对主库的受限查询能力（只读为主 + 白名单写）。
// 安全模型：绝不让任意 SQL 直达数据库——查询受白名单约束，
// 写操作（UPDATE/DELETE/INSERT）额外要求 confirm_risk 并受表名黑名单保护。
type DatabaseHandler struct {
	db *gorm.DB
	// #397 countRowsFn 只给测试注入用（钉住「计数失败」这一档，不必与具体 SQL 方言
	// 的故障方式较劲）；nil 时走真实 countRows。生产路径不设它。
	countRowsFn func(table string) (int64, error)
}

func NewDatabaseHandler(db *gorm.DB) *DatabaseHandler {
	return &DatabaseHandler{db: db}
}

// dbTable 表元信息。
//
// #397 RowCount 改为 *int64（可空）：数不出来时必须回 null 而不是 0。
// 这一页是管理员的「库还在不在」体检页，0 在这里只有一个读法——数据没了；
// 而 COUNT(*) 失败的原因常常只是连接池打满 / 库被锁 / 表刚被并发删掉，数据好好在那儿。
// 回 0 等于把「没数出来」说成「数出来是空的」，用户会去查备份、想去重建。
type dbTable struct {
	Name string `json:"name"`
	// nil = 行数未知（读失败，见 TableCountErrors）；非 nil 才是真实行数，含真实的 0。
	RowCount *int64 `json:"row_count"`
	// CountErr 非空时附在响应里的原因，供界面显示到该表上（不进日志以外的地方）。
	CountErr string `json:"count_error,omitempty"`
}

// dbColumn 列信息。
type dbColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	NotNull    bool   `json:"not_null"`
	PrimaryKey bool   `json:"primary_key"`
}

// Tables 列出所有用户表及其行数。
//
// #397 计数失败这一档单独成码：表清单本身仍然发出去（表名有用、页面还能用），
// 但必须把「哪几张表没数出来 + 为什么」一并带出去，否则界面只能显示 ?，
// 而「?」不解释的话，用户会当成 0 行。
func (h *DatabaseHandler) Tables(c *gin.Context) {
	tables, err := h.listTables()
	if err != nil {
		var ce *tableCountError
		if errors.As(err, &ce) {
			// 503 而不是 500：这是「暂时数不出来」（库被锁 / 连接池打满），不是服务端崩了。
			//
			// 表清单本身是好的（表名都列出来了），必须跟着错误一起发出去：
			// 前端要靠它把每张表画出来，并在读不出行数的那几张上显示 ? 与原因。
			// 只发错误不发 tables，界面就变成「一个表都没有」——那比 0 行更能误导人
			// （用户会以为库被清空了）。这正是本仓反复出现的那一族：失败出口把
			// 本来可用的数据一起连坐掉了。
			errcode.RespondWithFields(c,
				errcode.DatabaseTableCountFailed.WithDetailKeepMessage(
					errors.New("没数出来的表："+strings.Join(ce.FailedTables(), "、"))),
				gin.H{"tables": tables, "count_failed": ce.FailedTables()})
			return
		}
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

func (h *DatabaseHandler) listTables() ([]dbTable, error) {
	var raw []string
	switch h.db.Dialector.Name() {
	case "postgres":
		if err := h.db.Raw(`SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`).Scan(&raw).Error; err != nil {
			return nil, err
		}
	default: // sqlite
		if err := h.db.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`).Scan(&raw).Error; err != nil {
			return nil, err
		}
	}
	out := make([]dbTable, 0, len(raw))
	var failed []string
	for _, name := range raw {
		// #397 从前是 `count, _ := h.countRows(name)`：计数失败被整个吞掉、照样 200、
		// 那一张显示成 0。这里保留 nil 并把表名与原因带出去——由 Tables 决定怎么报。
		count, err := h.countRows(name)
		if err != nil {
			failed = append(failed, name)
			out = append(out, dbTable{Name: name, RowCount: nil, CountErr: err.Error()})
			continue
		}
		n := count
		out = append(out, dbTable{Name: name, RowCount: &n})
	}
	if len(failed) > 0 {
		return out, &tableCountError{tables: failed}
	}
	return out, nil
}

// tableCountError：部分表的行数读不出来。表名与原因都在这里，
// 交给出口决定怎么说；表本身已经带着 RowCount=nil 随 200 一起出去了（页面还能用）。
type tableCountError struct {
	tables []string
}

func (e *tableCountError) Error() string {
	return "以下表的行数读不出来：" + strings.Join(e.tables, "、")
}

// FailedTables 返回读不出行数的表名。
func (e *tableCountError) FailedTables() []string { return e.tables }

func (h *DatabaseHandler) countRows(table string) (int64, error) {
	if h.countRowsFn != nil {
		return h.countRowsFn(table)
	}
	var count int64
	if err := h.db.Table(table).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Schema 返回单表的列定义。
func (h *DatabaseHandler) Schema(c *gin.Context) {
	table := c.Param("table")
	if table == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if !isValidIdent(table) {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	cols, err := h.describeTable(table)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// 任何真实表/视图都至少有一列：零列只可能是「表不存在」（PRAGMA 对缺失表
	// 返回空结果、PG 的 information_schema 同理）。回 200 空数组会让界面画一张
	// 没有列的表，和真正的结构读取失败同形。
	if len(cols) == 0 {
		errcode.Respond(c, errcode.DatabaseTableNotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{"columns": cols})
}

func (h *DatabaseHandler) describeTable(table string) ([]dbColumn, error) {
	qt := quoteIdent(table)
	switch h.db.Dialector.Name() {
	case "postgres":
		type pgCol struct {
			Name    string `gorm:"column:column_name"`
			Type    string `gorm:"column:data_type"`
			NotNull bool   `gorm:"column:is_nullable"`
			IsPK    string `gorm:"column:column_default"`
		}
		var rows []pgCol
		err := h.db.Raw(`SELECT column_name, data_type,
			(is_nullable='NO') as is_nullable, column_default
			FROM information_schema.columns WHERE table_name=? ORDER BY ordinal_position`, table).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		out := make([]dbColumn, 0, len(rows))
		for _, r := range rows {
			out = append(out, dbColumn{Name: r.Name, Type: r.Type, NotNull: r.NotNull})
		}
		return out, nil
	default:
		type sqliteCol struct {
			Cid  int    `gorm:"column:cid"`
			Name string `gorm:"column:name"`
			Type string `gorm:"column:type"`
			NotN int    `gorm:"column:notnull"`
			PK   int    `gorm:"column:pk"`
		}
		var rows []sqliteCol
		if err := h.db.Raw("PRAGMA table_info(" + qt + ")").Scan(&rows).Error; err != nil {
			return nil, err
		}
		out := make([]dbColumn, 0, len(rows))
		for _, r := range rows {
			out = append(out, dbColumn{Name: r.Name, Type: r.Type, NotNull: r.NotN == 1, PrimaryKey: r.PK > 0})
		}
		return out, nil
	}
}

// dbQueryRequest 查询请求。
type dbQueryRequest struct {
	SQL   string `json:"sql" binding:"required,max=5000"`
	Limit int    `json:"limit"`
}

// dbQueryResult 查询结果。
type dbQueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Limit   int      `json:"limit,omitempty"`
	Took    string   `json:"took"`
	IsWrite bool     `json:"is_write"`
}

// dbReadPattern 只读语句白名单：仅允许 SELECT / WITH...SELECT 开头的单条查询。
// 曾允许 confirm_risk 后执行 UPDATE/DELETE 等任意写 SQL（黑名单可绕过，
// 属于 admin 凭据失陷后的提权后门），已收敛为只读（安全审计 P0-2）。
var dbReadPattern = regexp.MustCompile(`(?is)^\s*(SELECT|WITH)\b`)

// dbForbiddenPattern 禁止出现的语句形态（即使嵌在子查询里也拒绝）。
var dbForbiddenPattern = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|REPLACE|DROP|ALTER|CREATE|TRUNCATE|ATTACH|DETACH|PRAGMA|GRANT|REVOKE|VACUUM|REINDEX)\b`)

// Query 执行只读 SQL 查询（写操作一律拒绝，运维需求请走对应结构化 API 或 SQL 客户端直连）。
func (h *DatabaseHandler) Query(c *gin.Context) {
	var in dbQueryRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	sqlText := strings.TrimSpace(in.SQL)
	if sqlText == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if !dbReadPattern.MatchString(sqlText) {
		errcode.Respond(c, errcode.WriteSQLForbidden)
		return
	}
	if dbForbiddenPattern.MatchString(sqlText) {
		errcode.Respond(c, errcode.ForbiddenSQL)
		return
	}
	// 阻止多语句注入：只允许单条语句（不含分号结尾或多条用分号拼接）。
	trimmedSemi := strings.TrimRight(strings.TrimSpace(sqlText), ";")
	if strings.Contains(trimmedSemi, ";") {
		errcode.Respond(c, errcode.MultiStatement)
		return
	}

	// 只读查询，强制 LIMIT。
	limit := in.Limit
	if limit <= 0 {
		limit = 200 // 默认值
	} else if limit > 500 {
		limit = 500 // 钳到上限
	}
	wrapped := "SELECT * FROM (" + trimmedSemi + ") AS _q LIMIT " + itoa(limit)
	rows, err := h.db.Raw(wrapped).Rows()
	if err != nil {
		// 执行期报错（表名/列名不存在、关键字拼错）与前面四类「输入不合规」不是一回事：
		// 用户需要的是数据库那句原文，通用「请求参数有误」等于什么都没指。
		// 本端点仅管理员可达（routes_ops.go:164 /ops/database 组），同一页已能列出全部表名，
		// 因此回显驱动报错不新增信息暴露面。
		errcode.Respond(c, errcode.DatabaseQueryFailed.WithDetail(fmt.Errorf("%s：%v", errcode.DatabaseQueryFailed.Message, err)))
		return
	}
	defer rows.Close()
	result, err := scanRows(rows)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	result.IsWrite = false
	result.Limit = limit
	c.JSON(http.StatusOK, result)
}

// scanRows 把 sql.Rows 扫描成 columns + rows。
func scanRows(rows *sql.Rows) (*dbQueryResult, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := &dbQueryResult{Columns: cols, Rows: [][]any{}}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]any, len(cols))
		for i, v := range values {
			row[i] = normalizeSQLValue(v)
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

// normalizeSQLValue 把 []byte 等类型转成 JSON 友好的值。
func normalizeSQLValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	default:
		return t
	}
}

// isValidIdent 校验标识符只含字母数字下划线。
func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// quoteIdent 用双引号包裹标识符（SQL 标准）。
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// itoa 轻量整数转字符串。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
