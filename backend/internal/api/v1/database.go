package v1

import (
	"database/sql"
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
}

func NewDatabaseHandler(db *gorm.DB) *DatabaseHandler {
	return &DatabaseHandler{db: db}
}

// dbTable 表元信息。
type dbTable struct {
	Name     string `json:"name"`
	RowCount int64  `json:"row_count"`
}

// dbColumn 列信息。
type dbColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	NotNull    bool   `json:"not_null"`
	PrimaryKey bool   `json:"primary_key"`
}

// Tables 列出所有用户表及其行数。
func (h *DatabaseHandler) Tables(c *gin.Context) {
	tables, err := h.listTables()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	for _, name := range raw {
		count, _ := h.countRows(name)
		out = append(out, dbTable{Name: name, RowCount: count})
	}
	return out, nil
}

func (h *DatabaseHandler) countRows(table string) (int64, error) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "table is required"})
		return
	}
	if !isValidIdent(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}
	cols, err := h.describeTable(table)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	SQL         string `json:"sql" binding:"required,max=5000"`
	Limit       int    `json:"limit"`
	ConfirmRisk bool   `json:"confirm_risk"`
}

// dbQueryResult 查询结果。
type dbQueryResult struct {
	Columns  []string `json:"columns"`
	Rows     [][]any  `json:"rows"`
	Affected int64    `json:"affected,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Took     string   `json:"took"`
	IsWrite  bool     `json:"is_write"`
}

var (
	// dbWritePattern 写操作检测。
	dbWritePattern = regexp.MustCompile(`(?i)^\s*(UPDATE|DELETE|INSERT|DROP|ALTER|CREATE|TRUNCATE|REPLACE)\b`)
	// dbForbiddenPattern 禁止操作（即使 confirm 也不允许）。
	dbForbiddenPattern = regexp.MustCompile(`(?i)\b(DROP\s+DATABASE|DROP\s+TABLE|TRUNCATE\s+TABLE|ALTER\s+TABLE.*DROP)\b`)
)

// Query 执行受限 SQL 查询。
func (h *DatabaseHandler) Query(c *gin.Context) {
	var in dbQueryRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sqlText := strings.TrimSpace(in.SQL)
	if sqlText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sql is required"})
		return
	}
	if dbForbiddenPattern.MatchString(sqlText) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "forbidden operation: this SQL pattern is blocked", "code": "forbidden_sql"})
		return
	}
	isWrite := dbWritePattern.MatchString(sqlText)

	// 写操作需要二次确认。
	if isWrite && !in.ConfirmRisk {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "write operation requires confirmation",
			"code":  "risk_confirmation_required",
		})
		return
	}
	// 阻止多语句注入：写操作只允许单条语句（不含分号结尾或多条用分号拼接）。
	trimmedSemi := strings.TrimRight(strings.TrimSpace(sqlText), ";")
	if strings.Contains(trimmedSemi, ";") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multi-statement SQL is not allowed", "code": "multi_statement_blocked"})
		return
	}

	if isWrite {
		res := h.db.Exec(trimmedSemi)
		if res.Error != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": res.Error.Error()})
			return
		}
		affected := res.RowsAffected
		c.JSON(http.StatusOK, dbQueryResult{Affected: affected, IsWrite: true})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	result, err := scanRows(rows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
