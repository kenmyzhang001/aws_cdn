<template>
  <div class="r2-bucket-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>R2 存储桶管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建存储桶
          </el-button>
        </div>
      </template>

      <el-table :data="bucketList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="bucket_name" label="存储桶名称" width="200" />
        <el-table-column prop="cf_account.email" label="CF 账号" width="200"  />
        <el-table-column prop="location" label="存储位置" width="120" />
        <el-table-column prop="note" label="备注" show-overflow-tooltip />
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="500">
          <template #default="{ row }">
            <el-button size="small" @click="viewFiles(row)">
              文件管理
            </el-button>
            <el-button size="small" @click="viewDomains(row)">
              域名管理
            </el-button>
            <el-button size="small" @click="configureCORS(row)">
              配置 CORS
            </el-button>
            <el-button size="small" @click="editBucket(row)">
              编辑备注
            </el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 创建存储桶对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建 R2 存储桶" width="600px" @close="resetCreateForm">
      <el-form :model="createForm" :rules="formRules" ref="createFormRef" label-width="120px">
        <el-form-item label="CF 账号" prop="cf_account_id">
          <el-select v-model="createForm.cf_account_id" placeholder="请选择 CF 账号" style="width: 100%">
            <el-option
              v-for="account in cfAccountList"
              :key="account.id"
              :label="account.email"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="存储桶名称" prop="bucket_name">
          <el-input v-model="createForm.bucket_name" placeholder="请输入存储桶名称（小写字母、数字、连字符）" />
        </el-form-item>
        <el-form-item label="存储位置">
          <el-select v-model="createForm.location" placeholder="请选择存储位置" style="width: 100%">
            <el-option label="🌏 亚太地区 (推荐)" value="apac">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span>🌏 亚太地区 (APAC)</span>
                <el-tag size="small" type="success">推荐</el-tag>
              </div>
            </el-option>
            <el-option label="🌎 北美西部 (WNAM)" value="wnam" />
            <el-option label="🌎 北美东部 (ENAM)" value="enam" />
            <el-option label="🌍 欧洲西部 (WEUR)" value="weur" />
            <el-option label="🌍 欧洲东部 (EEUR)" value="eeur" />
            <el-option label="🤖 自动选择" value="auto" />
          </el-select>
          <div style="margin-top: 5px; font-size: 12px; color: #909399;">
            💡 提示：如果您的用户主要在亚洲，选择"亚太地区"可获得 5-10 倍速度提升
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

    <!-- 编辑备注对话框 -->
    <el-dialog v-model="showEditDialog" title="编辑备注" width="500px">
      <el-form :model="editForm" ref="editFormRef" label-width="100px">
        <el-form-item label="备注">
          <el-input
            v-model="editForm.note"
            type="textarea"
            :rows="3"
            placeholder="请输入备注"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="handleUpdateNote" :loading="updateLoading">
          更新
        </el-button>
      </template>
    </el-dialog>

    <!-- 配置 CORS 对话框 -->
    <el-dialog v-model="showCorsDialog" title="配置 CORS" width="700px">
      <el-alert
        title="CORS 配置说明"
        type="info"
        :closable="false"
        style="margin-bottom: 20px"
      >
        <template #default>
          <div style="font-size: 12px; line-height: 1.6">
            <p>配置跨域资源共享规则，允许指定域名访问存储桶中的文件。</p>
            <p>示例配置：</p>
            <pre style="background: #f5f5f5; padding: 10px; border-radius: 4px; font-size: 11px; margin-top: 10px;">[
{
    "allowed": {
        "origins": [
            "*"
        ],
        "methods": [
            "GET",
            "HEAD",
            "PUT",
            "POST",
            "DELETE"
        ],
        "headers": [
            "*"
        ]
    },
    "exposeHeaders": [
        "ETag",
        "Content-Length",
        "Content-Type"
    ],
    "maxAgeSeconds": 3600,
    "id": "rule-0"
}
]</pre>
          </div>
        </template>
      </el-alert>
      <el-form :model="corsForm" ref="corsFormRef" label-width="100px">
        <el-form-item label="CORS 配置">
          <el-input
            v-model="corsForm.corsConfig"
            type="textarea"
            :rows="12"
            placeholder='请输入 JSON 格式的 CORS 配置，例如：[{"allowed":{"origins":["*"],"methods":["GET","HEAD","PUT","POST","DELETE"],"headers":["*"]},"exposeHeaders":["ETag","Content-Length","Content-Type"],"maxAgeSeconds":3600,"id":"rule-0"}]'
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCorsDialog = false">取消</el-button>
        <el-button type="primary" @click="handleConfigureCORS" :loading="corsLoading">
          配置
        </el-button>
      </template>
    </el-dialog>

    <!-- 域名管理对话框 -->
    <el-dialog v-model="showDomainDialog" title="自定义域名管理" width="900px" @close="closeDomainDialog">
      <R2CustomDomainManager v-if="selectedBucket" :bucket="selectedBucket" />
    </el-dialog>

    <!-- 文件管理对话框 -->
    <el-dialog v-model="showFileDialog" title="文件管理" width="1000px" @close="closeFileDialog">
      <R2FileManager v-if="selectedBucket" :bucket="selectedBucket" />
    </el-dialog>

  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { r2Api } from '@/api/r2'
import { cfAccountApi } from '@/api/cf_account'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import R2CustomDomainManager from './R2CustomDomainManager.vue'
import R2FileManager from './R2FileManager.vue'

const loading = ref(false)
const bucketList = ref([])
const cfAccountList = ref([])

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createForm = ref({
  cf_account_id: null,
  account_id: '',
  bucket_name: '',
  location: 'apac', // 默认选择亚太地区
  note: '',
})
const createFormRef = ref(null)

const showEditDialog = ref(false)
const updateLoading = ref(false)
const editForm = ref({
  id: null,
  note: '',
})
const editFormRef = ref(null)

const showCorsDialog = ref(false)
const corsLoading = ref(false)
const corsForm = ref({
  bucketId: null,
  corsConfig: JSON.stringify([
    {
      allowed: {
        origins: ['*'],
        methods: ['GET', 'HEAD', 'PUT', 'POST', 'DELETE'],
        headers: ['*'],
      },
      exposeHeaders: ['ETag', 'Content-Length', 'Content-Type'],
      maxAgeSeconds: 3600,
      id: 'rule-0',
    },
  ], null, 2),
})
const corsFormRef = ref(null)

const showDomainDialog = ref(false)
const selectedBucket = ref(null)

const showFileDialog = ref(false)

const formRules = {
  cf_account_id: [
    { required: true, message: '请选择 CF 账号', trigger: 'change' },
  ],
  bucket_name: [
    { required: true, message: '请输入存储桶名称', trigger: 'blur' },
    { pattern: /^[a-z0-9-]+$/, message: '存储桶名称只能包含小写字母、数字和连字符', trigger: 'blur' },
    { min: 3, max: 63, message: '存储桶名称长度应在 3-63 个字符之间', trigger: 'blur' },
  ],
}

onMounted(() => {
  loadBuckets()
  loadCFAccounts()
})

const loadBuckets = async () => {
  loading.value = true
  try {
    const res = await r2Api.getR2BucketList()
    bucketList.value = res
  } catch (error) {
    ElMessage.error('加载存储桶列表失败')
  } finally {
    loading.value = false
  }
}

const loadCFAccounts = async () => {
  try {
    const res = await cfAccountApi.getCFAccountList()
    cfAccountList.value = res
  } catch (error) {
    // 静默失败
  }
}

const resetCreateForm = () => {
  createForm.value = {
    cf_account_id: null,
    account_id: '',
    bucket_name: '',
    location: 'apac', // 默认选择亚太地区
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
      await r2Api.createR2Bucket(createForm.value)
      ElMessage.success('存储桶创建成功')
      showCreateDialog.value = false
      loadBuckets()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      createLoading.value = false
    }
  })
}

const editBucket = (row) => {
  editForm.value = {
    id: row.id,
    note: row.note || '',
  }
  showEditDialog.value = true
}

const handleUpdateNote = async () => {
  updateLoading.value = true
  try {
    await r2Api.updateR2BucketNote(editForm.value.id, editForm.value.note)
    ElMessage.success('备注更新成功')
    showEditDialog.value = false
    loadBuckets()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    updateLoading.value = false
  }
}

const configureCORS = (row) => {
  corsForm.value.bucketId = row.id
  corsForm.value.corsConfig = JSON.stringify([
    {
      allowed: {
        origins: ['*'],
        methods: ['GET', 'HEAD', 'PUT', 'POST', 'DELETE'],
        headers: ['*'],
      },
      exposeHeaders: ['ETag', 'Content-Length', 'Content-Type'],
      maxAgeSeconds: 3600,
      id: 'rule-0',
    },
  ], null, 2)
  showCorsDialog.value = true
}

const handleConfigureCORS = async () => {
  try {
    const corsConfig = JSON.parse(corsForm.value.corsConfig)
    corsLoading.value = true
    await r2Api.configureCORS(corsForm.value.bucketId, corsConfig)
    ElMessage.success('CORS 配置成功')
    showCorsDialog.value = false
  } catch (error) {
    if (error.message && error.message.includes('JSON')) {
      ElMessage.error('CORS 配置格式错误，请输入有效的 JSON')
    }
    // 其他错误已在拦截器中处理
  } finally {
    corsLoading.value = false
  }
}

const viewDomains = (row) => {
  selectedBucket.value = row
  showDomainDialog.value = true
}

const closeDomainDialog = () => {
  selectedBucket.value = null
}

const viewFiles = (row) => {
  selectedBucket.value = row
  showFileDialog.value = true
}

const closeFileDialog = () => {
  selectedBucket.value = null
}


const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确定要删除存储桶 "${row.bucket_name}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await r2Api.deleteR2Bucket(row.id)
        ElMessage.success('存储桶删除成功')
        loadBuckets()
      } catch (error) {
        // 错误已在拦截器中处理
      }
    })
    .catch(() => {
      // 用户取消删除
    })
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
.r2-bucket-list {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
