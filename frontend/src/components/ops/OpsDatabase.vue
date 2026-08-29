<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { databaseApi, type DBTable, type DBColumn, type DBQueryResult } from '@/api/database'
import GlassIcon from '@/components/GlassIcon.vue'

const { t } = useI18n()
const tables = ref<DBTable[]>([])
const selectedTable = ref('')
const columns = ref<DBColumn[]>([])
const loading = ref(false)
const searchTable = ref('')

const sqlInput = ref('')
const queryResult = ref<DBQueryResult | null>(null)
const queryLimit = ref(200)
const querying = ref(false)

const filteredTables = computed(() =>
  tables.value.filter((t) => t.name.toLowerCase().includes(searchTable.value.toLowerCase()))
)

async function loadTables() {
  loading.value = true
  try {
    const res = await databaseApi.tables()
    tables.value = res.tables
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

async function selectTable(name: string) {
  selectedTable.value = name
  sqlInput.value = `SELECT * FROM ${name} LIMIT 50;`
  try {
    const res = await databaseApi.schema(name)
    columns.value = res.columns
  } catch (e: any) {
    columns.value = []
  }
  await runQuery()
}

async function runQuery() {
  if (!sqlInput.value.trim()) return
  querying.value = true
  try {
    queryResult.value = await databaseApi.query(sqlInput.value, queryLimit.value)
  } catch (e: any) {
    if (e?.response?.data?.code === 'risk_confirmation_required') {
      try {
        await ElMessageBox.confirm(t('opsdb.confirmWrite'), t('opsdb.confirmWriteTitle'), { type: 'warning' })
        queryResult.value = await databaseApi.query(sqlInput.value, queryLimit.value, true)
      } catch { /* cancelled */ }
    } else {
      ElMessage.error(e.message)
    }
  } finally {
    querying.value = false
  }
}

function fmtNum(n: number): string {
  if (n === undefined || n === null) return '-'
  return n.toLocaleString()
}

onMounted(loadTables)
</script>

<template>
  <div class="ops-database">
    <div class="db-layout">
      <!-- 左侧：表列表 -->
      <div class="db-sidebar glass">
        <div class="db-sidebar-header">
          <input v-model="searchTable" class="db-search" :placeholder="t('opsdb.searchTables')" />
        </div>
        <div class="db-table-list" v-loading="loading">
          <button
            v-for="t in filteredTables"
            :key="t.name"
            class="db-table-item"
            :class="{ active: selectedTable === t.name }"
            @click="selectTable(t.name)"
          >
            <span class="db-table-name">{{ t.name }}</span>
            <span class="db-table-count">{{ fmtNum(t.row_count) }}</span>
          </button>
          <div v-if="!filteredTables.length" class="db-empty">{{ t('opsdb.noMatch') }}</div>
        </div>
      </div>

      <!-- 右侧：查询区 -->
      <div class="db-main">
        <!-- SQL 编辑器 -->
        <div class="db-editor glass">
          <div class="db-editor-toolbar">
            <span class="db-editor-label">{{ t('opsdb.sqlQuery') }}</span>
            <select v-model="queryLimit" class="db-limit-select">
              <option :value="50">{{ t('opsdb.rowsN', { n: 50 }) }}</option>
              <option :value="100">{{ t('opsdb.rowsN', { n: 100 }) }}</option>
              <option :value="200">{{ t('opsdb.rowsN', { n: 200 }) }}</option>
              <option :value="500">{{ t('opsdb.rowsN', { n: 500 }) }}</option>
            </select>
            <button class="pt-btn pt-btn--primary db-run-btn" :disabled="querying" @click="runQuery">
              {{ querying ? t('common.loading') : '▶ ' + t('opsdb.execute') }}
            </button>
          </div>
          <textarea
            v-model="sqlInput"
            class="db-textarea"
            :placeholder="t('opsdb.sqlPlaceholder')"
            @keydown.ctrl.enter="runQuery"
            @keydown.meta.enter="runQuery"
          />
        </div>

        <!-- 列定义 -->
        <div v-if="columns.length" class="db-schema glass">
          <h4 class="db-section-title">{{ t('opsdb.schema') }}: {{ selectedTable }}</h4>
          <div class="db-col-list">
            <span v-for="col in columns" :key="col.name" class="db-col-chip" :title="col.type">
              <span class="db-col-name">{{ col.name }}</span>
              <span class="db-col-type">{{ col.type }}</span>
              <span v-if="col.primary_key" class="pt-badge pt-badge--primary" style="font-size: var(--fs-2xs);padding:1px 5px">PK</span>
              <span v-if="col.not_null" class="pt-badge pt-badge--warn" style="font-size: var(--fs-2xs);padding:1px 5px">NN</span>
            </span>
          </div>
        </div>

        <!-- 查询结果 -->
        <div v-if="queryResult" class="db-result glass">
          <div class="db-result-header">
            <span v-if="queryResult.is_write" class="pt-chip pt-chip--success">{{ t('opsdb.affectedRows', { n: queryResult.affected }) }}</span>
            <span v-else class="pt-chip pt-chip--info">{{ t('opsdb.rowsCount', { n: queryResult.rows.length }) }}</span>
          </div>
          <div v-if="!queryResult.is_write && queryResult.columns.length" class="db-result-table">
            <table>
              <thead>
                <tr><th v-for="col in queryResult.columns" :key="col">{{ col }}</th></tr>
              </thead>
              <tbody>
                <tr v-for="(row, i) in queryResult.rows" :key="i">
                  <td v-for="(cell, j) in row" :key="j">{{ cell === null ? 'NULL' : String(cell) }}</td>
                </tr>
                <tr v-if="!queryResult.rows.length"><td :colspan="queryResult.columns.length" class="td-empty">{{ t('mediaserver.noData') }}</td></tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ops-database { height: 100%; }
.db-layout { display: grid; grid-template-columns: 240px 1fr; gap: var(--sp-3); height: 100%; min-height: 500px; }
.db-sidebar { border-radius: var(--radius-md); display: flex; flex-direction: column; overflow: hidden; }
.db-sidebar-header { padding: var(--sp-2); border-bottom: 1px solid var(--divider); }
.db-search {
  width: 100%; padding: var(--sp-2); border: 1px solid var(--divider);
  border-radius: var(--radius-xs); font-size: var(--fs-sm); background: var(--surface-1); color: var(--text-strong);
}
.db-table-list { flex: 1; overflow-y: auto; }
.db-table-item {
  width: 100%; display: flex; justify-content: space-between; align-items: center;
  padding: var(--sp-2) var(--sp-3); border: 0; background: none; cursor: pointer; text-align: left;
  border-bottom: 1px solid var(--divider); transition: background var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth)
}
.db-table-item:hover { background: var(--surface-hover); }
.db-table-item.active { background: var(--surface-hover); border-left: 3px solid var(--pt-primary); }
.db-table-name { font-size: var(--fs-sm); font-family: monospace; color: var(--text-strong); }
.db-table-count { font-size: var(--fs-xs); color: var(--text-muted); }
.db-empty { padding: var(--sp-4); text-align: center; color: var(--text-muted); font-size: var(--fs-sm); }

.db-main { display: flex; flex-direction: column; gap: var(--sp-3); min-height: 0; }
.db-editor { border-radius: var(--radius-md); padding: var(--sp-3); }
.db-editor-toolbar { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: var(--sp-2); }
.db-editor-label { font-size: var(--fs-sm); color: var(--text-muted); }
.db-limit-select {
  margin-left: auto; padding: 4px 8px; border: 1px solid var(--divider); border-radius: var(--radius-xs);
  font-size: var(--fs-sm); background: var(--surface-1); color: var(--text-strong);
}
.db-run-btn { padding: 4px 12px; font-size: var(--fs-sm); }
.db-textarea {
  width: 100%; min-height: 80px; border: 1px solid var(--divider); border-radius: var(--radius-sm);
  padding: var(--sp-2); font-family: monospace; font-size: var(--fs-sm); resize: vertical;
  background: var(--surface-2); color: var(--text-strong);
}

.db-schema { border-radius: var(--radius-md); padding: var(--sp-3); }
.db-section-title { font-size: var(--fs-sm); margin: 0 0 var(--sp-2); color: var(--text-strong); }
.db-col-list { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.db-col-chip {
  display: inline-flex; align-items: center; gap: var(--sp-1); padding: 3px 8px;
  background: var(--surface-muted); border-radius: var(--radius-xs); font-size: var(--fs-xs);
}
.db-col-name { font-family: monospace; font-weight: 500; }
.db-col-type { color: var(--text-muted); }

.db-result { border-radius: var(--radius-md); padding: var(--sp-3); }
.db-result-header { margin-bottom: var(--sp-2); }
.db-result-table { overflow: auto; max-height: 400px; }
.db-result-table table { width: 100%; border-collapse: collapse; font-size: var(--fs-xs); }
.db-result-table th {
  text-align: left; padding: var(--sp-2); position: sticky; top: 0; z-index: 1;
  background: var(--surface-2); border-bottom: 2px solid var(--divider); white-space: nowrap;
}
.db-result-table td { padding: var(--sp-2); border-bottom: 1px solid var(--divider); max-width: 300px; overflow: hidden; text-overflow: ellipsis; }
.db-result-table tr:hover td { background: var(--surface-hover); }
.td-empty { text-align: center; color: var(--text-muted); padding: var(--sp-4); }

@media (max-width: 760px) {
  .db-layout { grid-template-columns: 1fr; }
  .db-sidebar { max-height: 200px; }
}
</style>
