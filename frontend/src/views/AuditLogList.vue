<template>
  <div class="audit-log-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>审计日志</span>
          <el-button @click="loadAuditLogs" :loading="loading">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
      </template>

      <!-- 筛选条件 -->
      <el-form :inline="true" :model="filters" class="filter-form">
        <el-form-item label="用户名">
          <el-input
            v-model="filters.username"
            placeholder="请输入用户名"
            clearable
            style="width: 150px"
            @clear="handleFilter"
          />
        </el-form-item>
        <el-form-item label="操作类型">
          <el-select
            v-model="filters.action"
            placeholder="请选择操作类型"
            clearable
            style="width: 150px"
            @change="handleFilter"
          >
            <el-option label="创建" value="create" />
            <el-option label="更新" value="update" />
            <el-option label="删除" value="delete" />
            <el-option label="生成证书" value="generate_certificate" />
            <el-option label="修复证书" value="fix_certificate" />
            <el-option label="检查证书" value="check_certificate" />
            <el-option label="绑定域名" value="bind_domain" />
            <el-option label="添加目标" value="add_target" />
            <el-option label="删除目标" value="remove_target" />
            <el-option label="创建分发" value="create_distribution" />
            <el-option label="更新分发" value="update_distribution" />
            <el-option label="删除分发" value="delete_distribution" />
            <el-option label="修复资源" value="fix_resource" />
            <el-option label="检查资源" value="check_resource" />
          </el-select>
        </el-form-item>
        <el-form-item label="资源类型">
          <el-select
            v-model="filters.resource"
            placeholder="请选择资源类型"
            clearable
            style="width: 150px"
            @change="handleFilter"
          >
            <el-option label="域名" value="domains" />
            <el-option label="重定向规则" value="redirects" />
            <el-option label="CloudFront分发" value="cloudfront" />
            <el-option label="下载包" value="download-packages" />
          </el-select>
        </el-form-item>
        <el-form-item label="IP地址">
          <el-input
            v-model="filters.ip"
            placeholder="请输入IP地址"
            clearable
            style="width: 150px"
            @clear="handleFilter"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleFilter">查询</el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="auditLogList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" width="120" />
        <el-table-column prop="action" label="操作类型" width="120">
          <template #default="{ row }">
            <el-tag :type="getActionType(row.action)" size="small">
              {{ getActionText(row.action) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="resource" label="资源类型" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.resource" size="small">
              {{ getResourceText(row.resource) }}
            </el-tag>
            <span v-else style="color: #c0c4cc;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="resource_id" label="资源ID" width="100" />
        <el-table-column prop="method" label="HTTP方法" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.method" :type="getMethodType(row.method)" size="small">
              {{ row.method }}
            </el-tag>
            <span v-else style="color: #c0c4cc;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="请求路径" min-width="200" show-overflow-tooltip />
        <el-table-column prop="ip" label="IP地址" width="130" />
        <el-table-column prop="status" label="状态码" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="duration" label="耗时(ms)" width="100" />
        <el-table-column prop="created_at" label="操作时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="viewDetail(row)">
              查看详情
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadAuditLogs"
        @current-change="loadAuditLogs"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 详情对话框 -->
    <el-dialog v-model="showDetailDialog" title="审计日志详情" width="800px">
      <el-descriptions :column="2" border v-if="currentLog">
        <el-descriptions-item label="ID">{{ currentLog.id }}</el-descriptions-item>
        <el-descriptions-item label="用户名">{{ currentLog.username }}</el-descriptions-item>
        <el-descriptions-item label="操作类型">
          <el-tag :type="getActionType(currentLog.action)" size="small">
            {{ getActionText(currentLog.action) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="资源类型">
          <el-tag v-if="currentLog.resource" size="small">
            {{ getResourceText(currentLog.resource) }}
          </el-tag>
          <span v-else>-</span>
        </el-descriptions-item>
        <el-descriptions-item label="资源ID">{{ currentLog.resource_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="HTTP方法">
          <el-tag v-if="currentLog.method" :type="getMethodType(currentLog.method)" size="small">
            {{ currentLog.method }}
          </el-tag>
          <span v-else>-</span>
        </el-descriptions-item>
        <el-descriptions-item label="请求路径" :span="2">
          {{ currentLog.path || '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="IP地址">{{ currentLog.ip || '-' }}</el-descriptions-item>
        <el-descriptions-item label="User-Agent" :span="2">
          <div style="word-break: break-all;">{{ currentLog.user_agent || '-' }}</div>
        </el-descriptions-item>
        <el-descriptions-item label="状态码">
          <el-tag :type="getStatusType(currentLog.status)" size="small">
            {{ currentLog.status }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="耗时">{{ currentLog.duration }} ms</el-descriptions-item>
        <el-descriptions-item label="操作时间" :span="2">
          {{ formatTime(currentLog.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="操作描述" :span="2">
          {{ currentLog.message || '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="错误信息" :span="2" v-if="currentLog.error">
          <el-alert type="error" :closable="false">
            {{ currentLog.error }}
          </el-alert>
        </el-descriptions-item>
        <el-descriptions-item label="请求数据" :span="2" v-if="currentLog.request">
          <el-input
            type="textarea"
            :rows="5"
            :value="formatJSON(currentLog.request)"
            readonly
          />
        </el-descriptions-item>
        <el-descriptions-item label="响应数据" :span="2" v-if="currentLog.response">
          <el-input
            type="textarea"
            :rows="5"
            :value="formatJSON(currentLog.response)"
            readonly
          />
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { auditApi } from '@/api/audit'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'

const loading = ref(false)
const auditLogList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)

const filters = ref({
  username: '',
  action: '',
  resource: '',
  ip: '',
})

const showDetailDialog = ref(false)
const currentLog = ref(null)

onMounted(() => {
  loadAuditLogs()
})

const loadAuditLogs = async () => {
  loading.value = true
  try {
    const params = {
      page: currentPage.value,
      page_size: pageSize.value,
    }
    
    // 添加过滤条件
    if (filters.value.username) {
      params.username = filters.value.username
    }
    if (filters.value.action) {
      params.action = filters.value.action
    }
    if (filters.value.resource) {
      params.resource = filters.value.resource
    }
    if (filters.value.ip) {
      params.ip = filters.value.ip
    }

    const res = await auditApi.getAuditLogList(params)
    auditLogList.value = res.data
    total.value = res.total
  } catch (error) {
    ElMessage.error('加载审计日志失败')
  } finally {
    loading.value = false
  }
}

const handleFilter = () => {
  currentPage.value = 1
  loadAuditLogs()
}

const resetFilter = () => {
  filters.value = {
    username: '',
    action: '',
    resource: '',
    ip: '',
  }
  currentPage.value = 1
  loadAuditLogs()
}

const viewDetail = (row) => {
  currentLog.value = row
  showDetailDialog.value = true
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

const getActionText = (action) => {
  const actionMap = {
    create: '创建',
    update: '更新',
    delete: '删除',
    generate_certificate: '生成证书',
    fix_certificate: '修复证书',
    check_certificate: '检查证书',
    bind_domain: '绑定域名',
    add_target: '添加目标',
    remove_target: '删除目标',
    create_distribution: '创建分发',
    update_distribution: '更新分发',
    delete_distribution: '删除分发',
    fix_resource: '修复资源',
    check_resource: '检查资源',
  }
  return actionMap[action] || action
}

const getActionType = (action) => {
  if (action === 'delete' || action === 'remove_target' || action === 'delete_distribution') {
    return 'danger'
  }
  if (action === 'create' || action === 'add_target' || action === 'create_distribution') {
    return 'success'
  }
  if (action === 'update' || action === 'update_distribution') {
    return 'warning'
  }
  return 'info'
}

const getResourceText = (resource) => {
  const resourceMap = {
    domains: '域名',
    redirects: '重定向规则',
    cloudfront: 'CloudFront分发',
    'download-packages': '下载包',
  }
  return resourceMap[resource] || resource
}

const getMethodType = (method) => {
  if (method === 'DELETE') return 'danger'
  if (method === 'POST') return 'success'
  if (method === 'PUT') return 'warning'
  return 'info'
}

const getStatusType = (status) => {
  if (!status) return 'info'
  if (status >= 200 && status < 300) return 'success'
  if (status >= 300 && status < 400) return 'warning'
  if (status >= 400) return 'danger'
  return 'info'
}
</script>

<style scoped>
.audit-log-list {
  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .filter-form {
    margin-bottom: 20px;
  }
}
</style>




