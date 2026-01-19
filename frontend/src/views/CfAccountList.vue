<template>
  <div class="cf-account-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>Cloudflare 账号管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建账号
          </el-button>
        </div>
      </template>

      <el-table :data="accountList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="email" label="账号邮箱" />
        <el-table-column prop="note" label="备注" show-overflow-tooltip />
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.updated_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="editAccount(row)">
              编辑
            </el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 创建账号对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建 Cloudflare 账号" width="600px" @close="resetCreateForm">
      <el-form :model="createForm" :rules="formRules" ref="createFormRef" label-width="120px">
        <el-form-item label="账号邮箱" prop="email">
          <el-input v-model="createForm.email" placeholder="请输入 Cloudflare 账号邮箱" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="createForm.password"
            type="password"
            placeholder="请输入 Cloudflare 账号密码"
            show-password
          />
        </el-form-item>
        <el-form-item label="API Token" prop="api_token">
          <el-input
            v-model="createForm.api_token"
            type="textarea"
            :rows="3"
            placeholder="请输入 Cloudflare API Token（可选）"
          />
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

    <!-- 编辑账号对话框 -->
    <el-dialog v-model="showEditDialog" title="编辑 Cloudflare 账号" width="600px" @close="resetEditForm">
      <el-form :model="editForm" :rules="editFormRules" ref="editFormRef" label-width="120px">
        <el-form-item label="账号邮箱" prop="email">
          <el-input v-model="editForm.email" placeholder="请输入 Cloudflare 账号邮箱" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="editForm.password"
            type="password"
            placeholder="留空则不修改密码"
            show-password
          />
        </el-form-item>
        <el-form-item label="API Token">
          <el-input
            v-model="editForm.api_token"
            type="textarea"
            :rows="3"
            placeholder="留空则不修改 API Token"
          />
        </el-form-item>
        <el-form-item label="备注">
          <el-input
            v-model="editForm.note"
            type="textarea"
            :rows="2"
            placeholder="请输入备注（可选）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="handleUpdate" :loading="updateLoading">
          更新
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { cfAccountApi } from '@/api/cf_account'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const loading = ref(false)
const accountList = ref([])

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createForm = ref({
  email: '',
  password: '',
  api_token: '',
  note: '',
})
const createFormRef = ref(null)

const showEditDialog = ref(false)
const updateLoading = ref(false)
const editForm = ref({
  id: null,
  email: '',
  password: '',
  api_token: '',
  note: '',
})
const editFormRef = ref(null)

const formRules = {
  email: [
    { required: true, message: '请输入账号邮箱', trigger: 'blur' },
    { type: 'email', message: '请输入有效的邮箱地址', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码长度至少 6 个字符', trigger: 'blur' },
  ],
}

const editFormRules = {
  email: [
    { required: true, message: '请输入账号邮箱', trigger: 'blur' },
    { type: 'email', message: '请输入有效的邮箱地址', trigger: 'blur' },
  ],
}

onMounted(() => {
  loadAccounts()
})

const loadAccounts = async () => {
  loading.value = true
  try {
    const res = await cfAccountApi.getCFAccountList()
    accountList.value = res
  } catch (error) {
    ElMessage.error('加载账号列表失败')
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.value = {
    email: '',
    password: '',
    api_token: '',
    note: '',
  }
  if (createFormRef.value) {
    createFormRef.value.clearValidate()
  }
}

const resetEditForm = () => {
  editForm.value = {
    id: null,
    email: '',
    password: '',
    api_token: '',
    note: '',
  }
  if (editFormRef.value) {
    editFormRef.value.clearValidate()
  }
}

const handleCreate = async () => {
  if (!createFormRef.value) return

  await createFormRef.value.validate(async (valid) => {
    if (!valid) return

    createLoading.value = true
    try {
      await cfAccountApi.createCFAccount(createForm.value)
      ElMessage.success('账号创建成功')
      showCreateDialog.value = false
      loadAccounts()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      createLoading.value = false
    }
  })
}

const editAccount = (row) => {
  editForm.value = {
    id: row.id,
    email: row.email,
    password: '',
    api_token: '',
    note: row.note || '',
  }
  showEditDialog.value = true
}

const handleUpdate = async () => {
  if (!editFormRef.value) return

  await editFormRef.value.validate(async (valid) => {
    if (!valid) return

    updateLoading.value = true
    try {
      const updateData = {
        email: editForm.value.email,
        note: editForm.value.note,
      }

      // 只有填写了密码才更新密码
      if (editForm.value.password) {
        updateData.password = editForm.value.password
      }

      // 只有填写了 API Token 才更新
      if (editForm.value.api_token) {
        updateData.api_token = editForm.value.api_token
      }

      await cfAccountApi.updateCFAccount(editForm.value.id, updateData)
      ElMessage.success('账号更新成功')
      showEditDialog.value = false
      loadAccounts()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      updateLoading.value = false
    }
  })
}

const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确定要删除账号 "${row.email}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await cfAccountApi.deleteCFAccount(row.id)
        ElMessage.success('账号删除成功')
        loadAccounts()
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
.cf-account-list {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
