import { http } from './http'

// === 数据库可视化管理（受限查询器）===

export interface DBTable {
  name: string
  row_count: number
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
  affected?: number
  limit?: number
  is_write: boolean
}

export const databaseApi = {
  tables: () =>
    http.get<{ tables: DBTable[] }>('/ops/database/tables').then((r) => r.data),
  schema: (table: string) =>
    http.get<{ columns: DBColumn[] }>(`/ops/database/tables/${table}/schema`).then((r) => r.data),
  query: (sql: string, limit = 200, confirmRisk = false) =>
    http.post<DBQueryResult>('/ops/database/query', { sql, limit, confirm_risk: confirmRisk }).then((r) => r.data),
}
