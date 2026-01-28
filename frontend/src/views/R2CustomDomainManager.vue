<template>
  <div class="r2-custom-domain-manager">
    <div style="margin-bottom: 20px">
      <el-alert
        :title="`存储桶：${bucket.bucket_name}`"
        type="info"
        :closable="false"
      />
    </div>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>自定义域名管理</span>
          <el-button type="primary" @click="showAddDialog = true">
            <el-icon><Plus /></el-icon>
            添加域名
          </el-button>
        </div>
      </template>

      <el-table :data="domainList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="domain" label="域名"width="200" />
        <el-table-column prop="zone_id" label="Zone ID" width="200" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">{{ row.status || 'unknown' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="default_file_path" label="默认文件" width="180">
          <template #default="{ row }">
            <span v-if="row.default_file_path" style="color: #67C23A;">
              {{ row.default_file_path }}
            </span>
            <span v-else style="color: #909399;">未设置</span>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" show-overflow-tooltip />
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button size="small" @click="viewCacheRules(row)">
              缓存规则
            </el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加域名对话框 -->
    <el-dialog v-model="showAddDialog" title="添加自定义域名" width="600px" @close="resetAddForm">
      <el-form :model="addForm" :rules="formRules" ref="addFormRef" label-width="140px">
        <el-form-item label="域名" prop="domain">
          <el-input v-model="addForm.domain" placeholder="例如：assets.jjj0108.com" />
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            请输入要绑定的子域名，域名必须在 Cloudflare 上托管
          </div>
        </el-form-item>
        <el-form-item label="默认文件路径">
          <el-select
            v-model="addForm.default_file_path"
            placeholder="请选择文件或手动输入路径"
            filterable
            allow-create
            clearable
            style="width: 100%"
            :loading="filesLoading"
            @visible-change="handleSelectVisibleChange"
          >
            <el-option
              v-for="file in fileList"
              :key="file"
              :label="file"
              :value="file"
            >
              <span style="float: left">{{ getFileName(file) }}</span>
              <!--span style="float: right; color: #909399; font-size: 12px">{{ file }}</span-->
            </el-option>
          </el-select>
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            💡 设置后，访问域名根路径（如 https://assets.example.com/）时将自动下载该文件
          </div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input
            v-model="addForm.note"
            type="textarea"
            :rows="2"
            placeholder="请输入备注（可选）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAdd" :loading="addLoading">
          添加
        </el-button>
      </template>
    </el-dialog>

    <!-- 缓存规则管理对话框 -->
    <el-dialog v-model="showCacheRuleDialog" title="缓存规则管理" width="1000px" @close="closeCacheRuleDialog">
      <R2CacheRuleManager v-if="selectedDomain" :domain="selectedDomain" />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { r2Api } from '@/api/r2'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import R2CacheRuleManager from './R2CacheRuleManager.vue'

const props = defineProps({
  bucket: {
    type: Object,
    required: true,
  },
})

const loading = ref(false)
const domainList = ref([])

const showAddDialog = ref(false)
const addLoading = ref(false)
const addForm = ref({
  domain: '',
  default_file_path: '',
  note: '',
})
const addFormRef = ref(null)

// 文件列表相关
const filesLoading = ref(false)
const fileList = ref([])

const showCacheRuleDialog = ref(false)
const selectedDomain = ref(null)

const formRules = {
  domain: [
    { required: true, message: '请输入域名', trigger: 'blur' },
    { pattern: /^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?(\.[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?)*$/, message: '请输入有效的域名格式', trigger: 'blur' },
  ],
}

onMounted(() => {
  loadDomains()
})

watch(() => props.bucket.id, () => {
  if (props.bucket.id) {
    loadDomains()
  }
})

const loadDomains = async () => {
  loading.value = true
  try {
    const res = await r2Api.getR2CustomDomainList(props.bucket.id)
    domainList.value = res
  } catch (error) {
    ElMessage.error('加载域名列表失败')
  } finally {
    loading.value = false
  }
}

const resetAddForm = () => {
  addForm.value = {
    domain: '',
    default_file_path: '',
    note: '',
  }
  // 清空文件列表
  fileList.value = []
  if (addFormRef.value) {
    addFormRef.value.clearValidate()
  }
}

const handleAdd = async () => {
  if (!addFormRef.value) return

  await addFormRef.value.validate(async (valid) => {
    if (!valid) return

    addLoading.value = true
    try {
      await r2Api.addR2CustomDomain(props.bucket.id, addForm.value)
      ElMessage.success('域名添加成功')
      showAddDialog.value = false
      loadDomains()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      addLoading.value = false
    }
  })
}

const viewCacheRules = (row) => {
  selectedDomain.value = row
  showCacheRuleDialog.value = true
}

const closeCacheRuleDialog = () => {
  selectedDomain.value = null
}

const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确定要删除域名 "${row.domain}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await r2Api.deleteR2CustomDomain(row.id)
        ElMessage.success('域名删除成功')
        loadDomains()
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

// 加载文件列表
const loadFileList = async () => {
  if (!props.bucket || !props.bucket.id) return
  
  filesLoading.value = true
  try {
    const res = await r2Api.listFiles(props.bucket.id)
    // 过滤掉目录（以 / 结尾的）
    fileList.value = (res.files || []).filter(file => !file.endsWith('/'))
  } catch (error) {
    // 静默失败，用户仍可手动输入
    console.error('加载文件列表失败:', error)
  } finally {
    filesLoading.value = false
  }
}

// 下拉框显示/隐藏时触发
const handleSelectVisibleChange = (visible) => {
  // 当下拉框打开且文件列表为空时，加载文件列表
  if (visible && fileList.value.length === 0) {
    loadFileList()
  }
}

// 从完整路径中提取文件名
const getFileName = (filePath) => {
  if (!filePath) return ''
  const parts = filePath.split('/')
  return parts[parts.length - 1]
}
</script>

<style scoped>
.r2-custom-domain-manager {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
