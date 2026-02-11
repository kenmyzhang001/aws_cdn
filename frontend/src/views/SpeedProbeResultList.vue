<template>
  <div class="speed-probe-result-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>速度探测结果</span>
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
        <el-form-item label="客户端 IP">
          <el-input
            v-model="filters.client_ip"
            placeholder="IP 或部分 IP"
            clearable
            style="width: 140px"
          />
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="filters.status"
            placeholder="全部"
            clearable
            style="width: 110px"
          >
            <el-option label="成功" value="success" />
            <el-option label="失败" value="failed" />
            <el-option label="超时" value="timeout" />
          </el-select>
        </el-form-item>
        <el-form-item label="创建时间">
          <el-date-picker
            v-model="filters.start_time"
            type="datetime"
            placeholder="开始"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 170px"
          />
          <span class="time-sep">至</span>
          <el-date-picker
            v-model="filters.end_time"
            type="datetime"
            placeholder="结束"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 170px"
          />
        </el-form-item>
        <el-form-item label="速度(KB/s)">
          <el-input-number v-model="filters.speed_min" :min="0" :precision="2" placeholder="最小" style="width: 100px" controls-position="right" />
          <span class="time-sep">~</span>
          <el-input-number v-model="filters.speed_max" :min="0" :precision="2" placeholder="最大" style="width: 100px" controls-position="right" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleFilter">查询</el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="list" v-loading="loading" stripe max-height="560">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="url" label="URL" min-width="220" show-overflow-tooltip />
        <el-table-column prop="client_ip" label="客户端 IP" width="130" />
        <el-table-column prop="speed_kbps" label="速度(KB/s)" width="110" align="right">
          <template #default="{ row }">
            {{ row.speed_kbps != null ? row.speed_kbps.toFixed(2) : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="file_size" label="文件大小" width="100" align="right">
          <template #default="{ row }">
            {{ row.file_size != null ? formatSize(row.file_size) : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="download_time_ms" label="耗时(ms)" width="100" align="right">
          <template #default="{ row }">
            {{ row.download_time_ms != null ? row.download_time_ms : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">
              {{ statusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="error_message" label="错误信息" min-width="160" show-overflow-tooltip />
        <el-table-column prop="created_at" label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
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
  client_ip: '',
  status: '',
  start_time: '',
  end_time: '',
  speed_min: undefined,
  speed_max: undefined,
})

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
    if (filters.value.client_ip) params.client_ip = filters.value.client_ip
    if (filters.value.status) params.status = filters.value.status
    if (filters.value.start_time) params.start_time = filters.value.start_time
    if (filters.value.end_time) params.end_time = filters.value.end_time
    if (filters.value.speed_min != null && filters.value.speed_min !== '') params.speed_min = filters.value.speed_min
    if (filters.value.speed_max != null && filters.value.speed_max !== '') params.speed_max = filters.value.speed_max

    const res = await speedProbeApi.getProbeResultList(params)
    list.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    ElMessage.error('加载探测结果失败')
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
    client_ip: '',
    status: '',
    start_time: '',
    end_time: '',
    speed_min: undefined,
    speed_max: undefined,
  }
  currentPage.value = 1
  loadList()
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

const formatSize = (bytes) => {
  if (bytes == null) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB'
  return (bytes / 1024 / 1024).toFixed(2) + ' MB'
}

const statusText = (status) => {
  const m = { success: '成功', failed: '失败', timeout: '超时' }
  return m[status] || status
}

const statusType = (status) => {
  const m = { success: 'success', failed: 'danger', timeout: 'warning' }
  return m[status] || 'info'
}
</script>

<style scoped>
.speed-probe-result-list .card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.speed-probe-result-list .filter-form {
  margin-bottom: 16px;
}
.time-sep {
  margin: 0 6px;
  color: #909399;
}
</style>
