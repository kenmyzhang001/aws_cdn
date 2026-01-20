<template>
  <div class="r2-cache-rule-manager">
    <div style="margin-bottom: 20px">
      <el-alert
        :title="`域名：${domain.domain}`"
        type="info"
        :closable="false"
      />
    </div>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>缓存规则管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建规则
          </el-button>
        </div>
      </template>

      <el-table :data="ruleList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="rule_name" label="规则名称" />
        <el-table-column prop="expression" label="匹配表达式" show-overflow-tooltip>
          <template #default="{ row }">
            <code style="font-size: 12px">{{ row.expression }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="cache_status" label="缓存状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.cache_status === 'Eligible' ? 'success' : 'info'">
              {{ row.cache_status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="edge_ttl" label="Edge TTL" width="120" />
        <el-table-column prop="browser_ttl" label="Browser TTL" width="120" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">{{ row.status || 'unknown' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" show-overflow-tooltip />
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 创建规则对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建缓存规则" width="800px" @close="resetCreateForm">
      <el-form :model="createForm" :rules="formRules" ref="createFormRef" label-width="140px">
        <el-form-item label="规则名称" prop="rule_name">
          <el-input v-model="createForm.rule_name" placeholder="例如：Cache APK Files Only" />
        </el-form-item>
        <el-form-item label="匹配表达式" prop="expression">
          <el-input
            v-model="createForm.expression"
            type="textarea"
            :rows="3"
            placeholder='例如：(http.host eq "assets.jjj0108.com" and http.request.uri.path.extension eq "apk")'
          />
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            使用 Cloudflare 规则表达式语法，支持 http.host、http.request.uri.path 等字段
          </div>
        </el-form-item>
        <el-form-item label="缓存状态" prop="cache_status">
          <el-select v-model="createForm.cache_status" placeholder="请选择缓存状态" style="width: 100%">
            <el-option label="Eligible（可缓存）" value="Eligible" />
            <el-option label="Bypass（绕过缓存）" value="Bypass" />
          </el-select>
        </el-form-item>
        <el-form-item label="Edge TTL" prop="edge_ttl">
          <el-input v-model="createForm.edge_ttl" placeholder="例如：1 month, 7 days, 3600 seconds" />
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            边缘节点缓存时间，支持：数字 + 单位（month/day/hour/minute/second）
          </div>
        </el-form-item>
        <el-form-item label="Browser TTL" prop="browser_ttl">
          <el-input v-model="createForm.browser_ttl" placeholder="例如：1 month, 7 days, 3600 seconds" />
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            浏览器缓存时间，支持：数字 + 单位（month/day/hour/minute/second）
          </div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input
            v-model="createForm.note"
            type="textarea"
            :rows="2"
            placeholder="请输入备注（可选）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="createLoading">
          创建
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { r2Api } from '@/api/r2'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const props = defineProps({
  domain: {
    type: Object,
    required: true,
  },
})

const loading = ref(false)
const ruleList = ref([])

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createForm = ref({
  rule_name: '',
  expression: '',
  cache_status: 'Eligible',
  edge_ttl: '1 month',
  browser_ttl: '1 month',
  note: '',
})
const createFormRef = ref(null)

const formRules = {
  rule_name: [
    { required: true, message: '请输入规则名称', trigger: 'blur' },
  ],
  expression: [
    { required: true, message: '请输入匹配表达式', trigger: 'blur' },
  ],
  cache_status: [
    { required: true, message: '请选择缓存状态', trigger: 'change' },
  ],
  edge_ttl: [
    { required: true, message: '请输入 Edge TTL', trigger: 'blur' },
  ],
  browser_ttl: [
    { required: true, message: '请输入 Browser TTL', trigger: 'blur' },
  ],
}

onMounted(() => {
  loadRules()
  // 设置默认表达式
  createForm.value.expression = `(http.host eq "${props.domain.domain}" and http.request.uri.path.extension eq "apk")`
})

watch(() => props.domain.id, () => {
  if (props.domain.id) {
    loadRules()
  }
})

const loadRules = async () => {
  loading.value = true
  try {
    const res = await r2Api.getR2CacheRuleList(props.domain.id)
    ruleList.value = res
  } catch (error) {
    ElMessage.error('加载缓存规则列表失败')
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.value = {
    rule_name: '',
    expression: `(http.host eq "${props.domain.domain}" and http.request.uri.path.extension eq "apk")`,
    cache_status: 'Eligible',
    edge_ttl: '1 month',
    browser_ttl: '1 month',
    note: '',
  }
  if (createFormRef.value) {
    createFormRef.value.clearValidate()
  }
}

const handleCreate = async () => {
  if (!createFormRef.value) return

  await createFormRef.value.validate(async (valid) => {
    if (!valid) return

    createLoading.value = true
    try {
      await r2Api.createR2CacheRule(props.domain.id, createForm.value)
      ElMessage.success('缓存规则创建成功')
      showCreateDialog.value = false
      loadRules()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      createLoading.value = false
    }
  })
}

const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确定要删除缓存规则 "${row.rule_name}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await r2Api.deleteR2CacheRule(row.id)
        ElMessage.success('缓存规则删除成功')
        loadRules()
      } catch (error) {
        // 错误已在拦截器中处理
      }
    })
    .catch(() => {
      // 用户取消删除
    })
}

const getStatusType = (status) => {
  const statusMap = {
    active: 'success',
    pending: 'warning',
    failed: 'danger',
  }
  return statusMap[status] || 'info'
}

const formatDate = (dateString) => {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}
</script>

<style scoped>
.r2-cache-rule-manager {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
