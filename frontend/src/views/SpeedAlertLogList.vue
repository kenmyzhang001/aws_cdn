<template>
  <div class="speed-alert-log-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>速度告警记录</span>
          <el-button @click="loadList" :loading="loading">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
      </template>

      <el-form :inline="true" :model="filters" class="filter-form">
        <el-form-item label="URL">
          <el-input
            v-model="filters.url"
            placeholder="URL 关键词"
            clearable
            style="width: 200px"
          />
        </el-form-item>
        <el-form-item label="时间窗口">
          <el-date-picker
            v-model="filters.time_window_from"
            type="datetime"
            placeholder="窗口开始 ≥"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 170px"
          />
          <span class="time-sep">至</span>
          <el-date-picker
            v-model="filters.time_window_to"
            type="datetime"
            placeholder="窗口结束 ≤"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 170px"
          />
        </el-form-item>
        <el-form-item label="记录创建时间">
          <el-date-picker
            v-model="filters.created_start"
            type="datetime"
            placeholder="开始"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 170px"
          />
          <span class="time-sep">至</span>
          <el-date-picker
            v-model="filters.created_end"
            type="datetime"
            placeholder="结束"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 170px"
          />
        </el-form-item>
        <el-form-item label="已发告警">
          <el-select
            v-model="filters.alert_sent"
            placeholder="全部"
            clearable
            style="width: 100px"
          >
            <el-option label="是" value="true" />
            <el-option label="否" value="false" />
          </el-select>
        </el-form-item>
        <el-form-item label="未达标率(%)">
          <el-input-number v-model="filters.failed_rate_min" :min="0" :max="100" :precision="2" placeholder="最小" style="width: 90px" controls-position="right" />
          <span class="time-sep">~</span>
          <el-input-number v-model="filters.failed_rate_max" :min="0" :max="100" :precision="2" placeholder="最大" style="width: 90px" controls-position="right" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleFilter">查询</el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="list" v-loading="loading" stripe max-height="520">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="url" label="URL" min-width="220" show-overflow-tooltip />
        <el-table-column prop="time_window_start" label="时间窗口开始" width="170">
          <template #default="{ row }">{{ formatTime(row.time_window_start) }}</template>
        </el-table-column>
        <el-table-column prop="time_window_end" label="时间窗口结束" width="170">
          <template #default="{ row }">{{ formatTime(row.time_window_end) }}</template>
        </el-table-column>
        <el-table-column prop="total_ips" label="IP 总数" width="90" align="right" />
        <el-table-column prop="failed_ips" label="未达标数" width="95" align="right" />
        <el-table-column prop="success_ips" label="达标数" width="90" align="right" />
        <el-table-column prop="failed_rate" label="未达标率(%)" width="110" align="right">
          <template #default="{ row }">
            {{ row.failed_rate != null ? row.failed_rate.toFixed(2) : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="avg_speed_kbps" label="平均速度(KB/s)" width="120" align="right">
          <template #default="{ row }">
            {{ row.avg_speed_kbps != null ? row.avg_speed_kbps.toFixed(2) : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="alert_sent" label="已发告警" width="95">
          <template #default="{ row }">
            <el-tag :type="row.alert_sent ? 'success' : 'info'" size="small">
              {{ row.alert_sent ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="viewDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadList"
        @current-change="loadList"
        style="margin-top: 16px"
      />
    </el-card>

    <el-dialog v-model="showDetail" title="告警详情" width="700px">
      <el-descriptions :column="1" border v-if="currentRow">
        <el-descriptions-item label="URL">{{ currentRow.url }}</el-descriptions-item>
        <el-descriptions-item label="时间窗口">{{ formatTime(currentRow.time_window_start) }} ~ {{ formatTime(currentRow.time_window_end) }}</el-descriptions-item>
        <el-descriptions-item label="IP 总数 / 未达标 / 达标">{{ currentRow.total_ips }} / {{ currentRow.failed_ips }} / {{ currentRow.success_ips }}</el-descriptions-item>
        <el-descriptions-item label="未达标率">{{ currentRow.failed_rate != null ? currentRow.failed_rate.toFixed(2) + '%' : '-' }}</el-descriptions-item>
        <el-descriptions-item label="平均速度(KB/s)">{{ currentRow.avg_speed_kbps != null ? currentRow.avg_speed_kbps.toFixed(2) : '-' }}</el-descriptions-item>
        <el-descriptions-item label="已发告警">{{ currentRow.alert_sent ? '是' : '否' }}</el-descriptions-item>
        <el-descriptions-item label="告警消息" v-if="currentRow.alert_message">
          <pre class="alert-msg">{{ currentRow.alert_message }}</pre>
        </el-descriptions-item>
        <el-descriptions-item label="IP 详情(JSON)" v-if="currentRow.ip_details">
          <pre class="ip-details">{{ formatJSON(currentRow.ip_details) }}</pre>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { speedProbeApi } from '@/api/speedProbe'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'

const loading = ref(false)
const list = ref([])
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)

const filters = ref({
  url: '',
  time_window_from: '',
  time_window_to: '',
  created_start: '',
  created_end: '',
  alert_sent: '',
  failed_rate_min: undefined,
  failed_rate_max: undefined,
})

const showDetail = ref(false)
const currentRow = ref(null)

onMounted(() => {
  loadList()
})

const loadList = async () => {
  loading.value = true
  try {
    const params = {
      page: currentPage.value,
      page_size: pageSize.value,
    }
    if (filters.value.url) params.url = filters.value.url
    if (filters.value.time_window_from) params.time_window_from = filters.value.time_window_from
    if (filters.value.time_window_to) params.time_window_to = filters.value.time_window_to
    if (filters.value.created_start) params.created_start = filters.value.created_start
    if (filters.value.created_end) params.created_end = filters.value.created_end
    if (filters.value.alert_sent !== '' && filters.value.alert_sent != null) params.alert_sent = filters.value.alert_sent
    if (filters.value.failed_rate_min != null && filters.value.failed_rate_min !== '') params.failed_rate_min = filters.value.failed_rate_min
    if (filters.value.failed_rate_max != null && filters.value.failed_rate_max !== '') params.failed_rate_max = filters.value.failed_rate_max

    const res = await speedProbeApi.getAlertLogList(params)
    list.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    ElMessage.error('加载告警记录失败')
  } finally {
    loading.value = false
  }
}

const handleFilter = () => {
  currentPage.value = 1
  loadList()
}

const resetFilter = () => {
  filters.value = {
    url: '',
    time_window_from: '',
    time_window_to: '',
    created_start: '',
    created_end: '',
    alert_sent: '',
    failed_rate_min: undefined,
    failed_rate_max: undefined,
  }
  currentPage.value = 1
  loadList()
}

const viewDetail = (row) => {
  currentRow.value = row
  showDetail.value = true
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

const formatJSON = (str) => {
  if (!str) return ''
  try {
    const obj = typeof str === 'string' ? JSON.parse(str) : str
    return JSON.stringify(obj, null, 2)
  } catch {
    return str
  }
}
</script>

<style scoped>
.speed-alert-log-list .card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.speed-alert-log-list .filter-form {
  margin-bottom: 16px;
}
.time-sep {
  margin: 0 6px;
  color: #909399;
}
.alert-msg,
.ip-details {
  margin: 0;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 4px;
  max-height: 200px;
  overflow: auto;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
