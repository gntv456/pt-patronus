import { http } from './http'

// === 数据库可视化管理（受限查询器）===

export interface DBTable {
  name: string
  // #397 后端在数不出行数时回 null 而不是 0 —— 0 在「数据库体检」这一页
  // 只有一个读法（数据没了），必须与「真的是空表」区分开。
  row_count: number | null
  count_error?: string
}

export interface DBColumn {
  name: string
  type: string
  not_null: boolean
  primary_key: boolean
}

export interface DBQueryResult {
  columns: string[]
  rows: any[][]
  limit?: number
  is_write: boolean
}

export const databaseApi = {
  // #397 行数读不出来时后端回 503 **并把表清单一起带在响应体里**（tables 字段）。
  // 这里把「错误 + 清单」收进正常返回值而不是抛：清单本身是好的、表名都有用，
  // 页面要靠它渲染每张表并把读不出行数的那几张标 ?。抛掉等于界面显示「一个表都没有」，
  // 那比显示 0 行更糟（用户会直接认定库被清空）。
  // 失败原因经 error 字段带回，由 loadTables 用它提示。
  tables: () =>
    http
      .get<{ tables?: DBTable[]; count_failed?: string[] }>('/ops/database/tables')
      .then((r) => ({ tables: r.data.tables || [], error: '' as string }))
      .catch((e: any) => ({
        tables: (e?.response?.data?.tables || []) as DBTable[],
        error: (e?.response?.data?.error || e?.message || '') as string,
      })),
  schema: (table: string) =>
    http.get<{ columns: DBColumn[] }>(`/ops/database/tables/${table}/schema`).then((r) => r.data),
  query: (sql: string, limit = 200) =>
    http.post<DBQueryResult>('/ops/database/query', { sql, limit }).then((r) => r.data),
}
