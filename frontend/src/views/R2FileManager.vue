<template>
  <div class="r2-file-manager">
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
          <div style="display: flex; align-items: center; gap: 10px">
            <el-input
              v-model="currentPrefix"
              placeholder="当前路径"
              style="width: 300px"
              readonly
            >
              <template #prepend>路径</template>
            </el-input>
            <el-button @click="goUp" :disabled="currentPrefix === ''">
              <el-icon><ArrowUp /></el-icon>
              返回上级
            </el-button>
            <el-button @click="refresh">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
          </div>
          <div style="display: flex; gap: 10px">
            <el-button type="primary" @click="showUploadDialog = true">
              <el-icon><Upload /></el-icon>
              上传文件
            </el-button>
            <el-button @click="showCreateDirDialog = true">
              <el-icon><FolderAdd /></el-icon>
              创建目录
            </el-button>
          </div>
        </div>
      </template>

      <el-table :data="fileList" v-loading="loading" stripe>
        <el-table-column label="名称" min-width="300">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <el-icon v-if="isDirectory(row)">
                <Folder />
              </el-icon>
              <el-icon v-else>
                <Document />
              </el-icon>
              <span
                style="cursor: pointer; color: #409eff"
                @click="handleItemClick(row)"
              >
                {{ getFileName(row) }}
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="100">
          <template #default="{ row }">
            <el-tag v-if="isDirectory(row)" type="info">目录</el-tag>
            <el-tag v-else>文件</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button
              v-if="!isDirectory(row)"
              size="small"
              type="danger"
              @click="handleDelete(row)"
            >
              删除
            </el-button>
            <el-button
              v-if="isDirectory(row)"
              size="small"
              type="danger"
              @click="handleDeleteDir(row)"
            >
              删除目录
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 上传文件对话框 -->
    <el-dialog v-model="showUploadDialog" title="上传文件" width="600px" @close="resetUploadForm">
      <el-form :model="uploadForm" ref="uploadFormRef" label-width="100px">
        <el-form-item label="选择文件" prop="file">
          <el-upload
            ref="uploadRef"
            :auto-upload="false"
            :on-change="handleFileChange"
            :limit="1"
            drag
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">
              将文件拖到此处，或<em>点击上传</em>
            </div>
          </el-upload>
        </el-form-item>
        <el-form-item label="文件路径">
          <el-input
            v-model="uploadForm.key"
            :placeholder="`当前路径：${currentPrefix || '/'}`"
          >
            <template #prepend>{{ currentPrefix || '/' }}</template>
          </el-input>
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            留空则使用文件名，可指定完整路径
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showUploadDialog = false">取消</el-button>
        <el-button type="primary" @click="handleUpload" :loading="uploadLoading">
          上传
        </el-button>
      </template>
    </el-dialog>

    <!-- 创建目录对话框 -->
    <el-dialog v-model="showCreateDirDialog" title="创建目录" width="500px" @close="resetCreateDirForm">
      <el-form :model="createDirForm" :rules="createDirFormRules" ref="createDirFormRef" label-width="100px">
        <el-form-item label="目录名称" prop="prefix">
          <el-input
            v-model="createDirForm.prefix"
            :placeholder="`当前路径：${currentPrefix || '/'}`"
          />
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            将在当前路径下创建目录
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDirDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreateDir" :loading="createDirLoading">
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
import {
  Upload,
  FolderAdd,
  Refresh,
  ArrowUp,
  Folder,
  Document,
  UploadFilled,
} from '@element-plus/icons-vue'

const props = defineProps({
  bucket: {
    type: Object,
    required: true,
  },
})

const loading = ref(false)
const fileList = ref([])
const currentPrefix = ref('')

const showUploadDialog = ref(false)
const uploadLoading = ref(false)
const uploadForm = ref({
  file: null,
  key: '',
})
const uploadFormRef = ref(null)
const uploadRef = ref(null)

const showCreateDirDialog = ref(false)
const createDirLoading = ref(false)
const createDirForm = ref({
  prefix: '',
})
const createDirFormRef = ref(null)

const createDirFormRules = {
  prefix: [
    { required: true, message: '请输入目录名称', trigger: 'blur' },
  ],
}

onMounted(() => {
  loadFiles()
})

watch(() => props.bucket.id, () => {
  if (props.bucket.id) {
    currentPrefix.value = ''
    loadFiles()
  }
})

const loadFiles = async () => {
  loading.value = true
  try {
    const res = await r2Api.listFiles(props.bucket.id, currentPrefix.value)
    // 处理文件列表，去重并排序
    const files = [...new Set(res.files || [])]
    files.sort((a, b) => {
      const aIsDir = a.endsWith('/')
      const bIsDir = b.endsWith('/')
      if (aIsDir && !bIsDir) return -1
      if (!aIsDir && bIsDir) return 1
      return a.localeCompare(b)
    })
    fileList.value = files
  } catch (error) {
    ElMessage.error('加载文件列表失败')
  } finally {
    loading.value = false
  }
}

const refresh = () => {
  loadFiles()
}

const goUp = () => {
  if (currentPrefix.value === '') return
  const parts = currentPrefix.value.split('/').filter(p => p)
  parts.pop()
  currentPrefix.value = parts.length > 0 ? parts.join('/') + '/' : ''
  loadFiles()
}

const handleItemClick = (row) => {
  if (isDirectory(row)) {
    // 进入目录
    currentPrefix.value = row
    loadFiles()
  }
}

const isDirectory = (key) => {
  return key.endsWith('/')
}

const getFileName = (key) => {
  if (currentPrefix.value) {
    // 移除当前前缀
    if (key.startsWith(currentPrefix.value)) {
      return key.substring(currentPrefix.value.length)
    }
  }
  return key
}

const handleFileChange = (file) => {
  uploadForm.value.file = file.raw
  if (!uploadForm.value.key) {
    uploadForm.value.key = file.name
  }
}

const resetUploadForm = () => {
  uploadForm.value = {
    file: null,
    key: '',
  }
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
}

const handleUpload = async () => {
  if (!uploadForm.value.file) {
    ElMessage.warning('请选择要上传的文件')
    return
  }

  uploadLoading.value = true
  try {
    const formData = new FormData()
    formData.append('file', uploadForm.value.file)
    const key = currentPrefix.value
      ? `${currentPrefix.value}${uploadForm.value.key}`
      : uploadForm.value.key
    formData.append('key', key)

    await r2Api.uploadFile(props.bucket.id, formData)
    ElMessage.success('文件上传成功')
    showUploadDialog.value = false
    loadFiles()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    uploadLoading.value = false
  }
}

const resetCreateDirForm = () => {
  createDirForm.value = {
    prefix: '',
  }
  if (createDirFormRef.value) {
    createDirFormRef.value.clearValidate()
  }
}

const handleCreateDir = async () => {
  if (!createDirFormRef.value) return

  await createDirFormRef.value.validate(async (valid) => {
    if (!valid) return

    createDirLoading.value = true
    try {
      const prefix = currentPrefix.value
        ? `${currentPrefix.value}${createDirForm.value.prefix}`
        : createDirForm.value.prefix
      await r2Api.createDirectory(props.bucket.id, prefix)
      ElMessage.success('目录创建成功')
      showCreateDirDialog.value = false
      loadFiles()
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      createDirLoading.value = false
    }
  })
}

const handleDelete = (row) => {
  ElMessageBox.confirm(
    `确定要删除文件 "${getFileName(row)}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await r2Api.deleteFile(props.bucket.id, row)
        ElMessage.success('文件删除成功')
        loadFiles()
      } catch (error) {
        // 错误已在拦截器中处理
      }
    })
    .catch(() => {
      // 用户取消删除
    })
}

const handleDeleteDir = (row) => {
  ElMessageBox.confirm(
    `确定要删除目录 "${getFileName(row)}" 吗？此操作不可恢复。`,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        // 删除目录实际上也是删除文件（目录在 R2 中是一个以 / 结尾的对象）
        await r2Api.deleteFile(props.bucket.id, row)
        ElMessage.success('目录删除成功')
        loadFiles()
      } catch (error) {
        // 错误已在拦截器中处理
      }
    })
    .catch(() => {
      // 用户取消删除
    })
}
</script>

<style scoped>
.r2-file-manager {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}
</style>
