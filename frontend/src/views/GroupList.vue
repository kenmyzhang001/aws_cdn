<template>
  <div class="group-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>分组管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建分组
          </el-button>
        </div>
      </template>

      <el-table :data="groupList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="name" label="分组名称" />
        <el-table-column label="是否默认" width="120">
          <template #default="{ row }">
            <el-tag :type="row.is_default ? 'success' : 'info'" size="small">
              {{ row.is_default ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
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
            <el-button
              v-if="!row.is_default"
              size="small"
              @click="editGroup(row)"
            >
              编辑
            </el-button>
            <el-button
              v-if="!row.is_default"
              size="small"
              type="danger"
              @click="handleDelete(row)"
            >
              删除
            </el-button>
            <span v-else style="color: #c0c4cc; font-size: 12px;">默认分组不可操作</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 创建分组对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建分组" width="500px" @close="resetCreateForm">
      <el-form :model="createForm" :rules="formRules" ref="createFormRef" label-width="100px">
        <el-form-item label="分组名称" prop="name">
          <el-input v-model="createForm.name" placeholder="请输入分组名称" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="createLoading">
          创建
        </el-button>
      </template>
    </el-dialog>

    <!-- 编辑分组对话框 -->
    <el-dialog v-model="showEditDialog" title="编辑分组" width="500px" @close="resetEditForm">
      <el-form :model="editForm" :rules="formRules" ref="editFormRef" label-width="100px">
        <el-form-item label="分组名称" prop="name">
          <el-input v-model="editForm.name" placeholder="请输入分组名称" />
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
import { groupApi } from '@/api/group'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const loading = ref(false)
const groupList = ref([])

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createForm = ref({
  name: '',
})
const createFormRef = ref(null)

const showEditDialog = ref(false)
const updateLoading = ref(false)
const editForm = ref({
  id: null,
  name: '',
})
const editFormRef = ref(null)

const formRules = {
  name: [
    { required: true, message: '请输入分组名称', trigger: 'blur' },
    { min: 1, max: 255, message: '分组名称长度在 1 到 255 个字符', trigger: 'blur' },
  ],
}

onMounted(() => {
  loadGroups()
})

const loadGroups = async () => {
  loading.value = true
  try {
    const res = await groupApi.getGroupList()
    groupList.value = res
  } catch (error) {
    ElMessage.error('加载分组列表失败')
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.value = { name: '' }
  if (createFormRef.value) {
    createFormRef.value.clearValidate()
  }
}

const resetEditForm = () => {
  editForm.value = { id: null, name: '' }
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
      await groupApi.createGroup(createForm.value)
      ElMessage.success('分组创建成功')
      showCreateDialog.value = false
      loadGroups()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      createLoading.value = false
    }
  })
}

const editGroup = (row) => {
  editForm.value = {
    id: row.id,
    name: row.name,
  }
  showEditDialog.value = true
}

const handleUpdate = async () => {
  if (!editFormRef.value) return
  
  await editFormRef.value.validate(async (valid) => {
    if (!valid) return

    updateLoading.value = true
    try {
      await groupApi.updateGroup(editForm.value.id, { name: editForm.value.name })
      ElMessage.success('分组更新成功')
      showEditDialog.value = false
      loadGroups()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      updateLoading.value = false
    }
  })
}

const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确定要删除分组 "${row.name}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await groupApi.deleteGroup(row.id)
        ElMessage.success('分组删除成功')
        loadGroups()
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
.group-list {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

