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
    <el-dialog v-model="showUploadDialog" title="上传文件" width="800px" @close="resetUploadForm">
      <el-form :model="uploadForm" ref="uploadFormRef" label-width="100px">
        <el-form-item label="选择文件">
          <el-upload
            ref="uploadRef"
            :auto-upload="false"
            :on-change="handleFileChange"
            :on-remove="handleFileRemove"
            :multiple="true"
            drag
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">
              将文件拖到此处，或<em>点击上传</em>
            </div>
            <template #tip>
              <div class="el-upload__tip">
                支持多文件上传，可同时选择多个文件
              </div>
            </template>
          </el-upload>
        </el-form-item>
        <el-form-item label="上传路径">
          <el-input
            v-model="uploadForm.path"
            :placeholder="`当前路径：${currentPrefix || '/'}`"
          >
            <template #prepend>{{ currentPrefix || '/' }}</template>
          </el-input>
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            留空则使用文件名，可指定子目录路径（如：images/）
          </div>
        </el-form-item>
      </el-form>

      <!-- 上传进度列表 -->
      <div v-if="uploadFiles.length > 0" style="margin-top: 20px">
        <el-divider>上传进度</el-divider>
        <div v-for="(file, index) in uploadFiles" :key="index" style="margin-bottom: 15px">
          <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 5px">
            <div style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
              <el-icon style="margin-right: 5px"><Document /></el-icon>
              <span>{{ file.name }}</span>
            </div>
            <div style="display: flex; align-items: center; gap: 10px; min-width: 150px">
              <el-tag v-if="file.status === 'uploading'" type="info">上传中</el-tag>
              <el-tag v-else-if="file.status === 'success'" type="success">成功</el-tag>
              <el-tag v-else-if="file.status === 'error'" type="danger">失败</el-tag>
              <el-tag v-else type="warning">等待</el-tag>
              <el-button
                v-if="file.status === 'uploading'"
                size="small"
                type="danger"
                text
                @click="cancelUpload(file)"
              >
                取消
              </el-button>
            </div>
          </div>
          <el-progress
            :percentage="file.progress"
            :status="file.status === 'error' ? 'exception' : file.status === 'success' ? 'success' : null"
            :stroke-width="8"
          />
          <div v-if="file.error" style="font-size: 12px; color: #f56c6c; margin-top: 5px">
            {{ file.error }}
          </div>
        </div>
      </div>

      <template #footer>
        <el-button @click="showUploadDialog = false" :disabled="isUploading">取消</el-button>
        <el-button
          type="primary"
          @click="handleUpload"
          :loading="isUploading"
          :disabled="uploadFiles.length === 0"
        >
          {{ isUploading ? '上传中...' : '开始上传' }}
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
import { ref, onMounted, watch, computed } from 'vue'
import { r2Api } from '@/api/r2'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'
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
const uploadForm = ref({
  path: '',
})
const uploadFormRef = ref(null)
const uploadRef = ref(null)
const uploadFiles = ref([]) // 上传文件列表，包含进度信息

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

const handleFileChange = (file, fileList) => {
  // 更新上传文件列表
  uploadFiles.value = fileList.map((f) => ({
    name: f.name,
    raw: f.raw,
    status: 'waiting', // waiting, uploading, success, error
    progress: 0,
    error: null,
    cancelToken: null,
  }))
}

const handleFileRemove = (file, fileList) => {
  // 如果文件正在上传，取消上传
  const uploadFile = uploadFiles.value.find((f) => f.name === file.name)
  if (uploadFile && uploadFile.status === 'uploading' && uploadFile.cancelToken) {
    uploadFile.cancelToken.cancel('用户取消上传')
  }
  
  // 更新文件列表
  uploadFiles.value = fileList.map((f) => ({
    name: f.name,
    raw: f.raw,
    status: 'waiting',
    progress: 0,
    error: null,
    cancelToken: null,
  }))
}

const resetUploadForm = () => {
  // 取消所有正在上传的文件
  uploadFiles.value.forEach((file) => {
    if (file.status === 'uploading' && file.cancelToken) {
      file.cancelToken.cancel('对话框关闭')
    }
  })
  
  uploadForm.value = {
    path: '',
  }
  uploadFiles.value = []
  if (uploadRef.value) {
    uploadRef.value.clearFiles()
  }
}

const cancelUpload = (file) => {
  if (file.cancelToken) {
    file.cancelToken.cancel('用户取消上传')
    file.status = 'error'
    file.error = '已取消'
  }
}

const isUploading = computed(() => {
  return uploadFiles.value.some((f) => f.status === 'uploading')
})

const handleUpload = async () => {
  if (uploadFiles.value.length === 0) {
    ElMessage.warning('请选择要上传的文件')
    return
  }

  // 重置所有文件状态
  uploadFiles.value.forEach((file) => {
    file.status = 'waiting'
    file.progress = 0
    file.error = null
  })

  // 并发上传所有文件
  const uploadPromises = uploadFiles.value.map(async (file) => {
    try {
      file.status = 'uploading'
      file.progress = 0

      const formData = new FormData()
      formData.append('file', file.raw)
      
      // 构建文件路径
      let key = file.name
      if (uploadForm.value.path) {
        // 如果指定了路径，使用路径 + 文件名
        const path = uploadForm.value.path.endsWith('/')
          ? uploadForm.value.path
          : uploadForm.value.path + '/'
        key = path + file.name
      }
      
      // 添加当前前缀
      if (currentPrefix.value) {
        key = currentPrefix.value + key
      }
      
      formData.append('key', key)

      // 创建 CancelToken
      const CancelToken = axios.CancelToken
      const source = CancelToken.source()
      file.cancelToken = source

      // 上传文件，带进度回调
      await r2Api.uploadFileWithProgress(
        props.bucket.id,
        formData,
        (progressEvent) => {
          if (progressEvent.total) {
            file.progress = Math.round((progressEvent.loaded * 100) / progressEvent.total)
          }
        },
        source.token
      )

      file.status = 'success'
      file.progress = 100
    } catch (error) {
      if (axios.isCancel(error)) {
        file.status = 'error'
        file.error = '已取消'
      } else {
        file.status = 'error'
        file.error = error.response?.data?.error || error.message || '上传失败'
      }
    } finally {
      file.cancelToken = null
    }
  })

  // 等待所有文件上传完成
  await Promise.allSettled(uploadPromises)

  // 检查是否有成功上传的文件
  const successCount = uploadFiles.value.filter((f) => f.status === 'success').length
  const errorCount = uploadFiles.value.filter((f) => f.status === 'error').length

  if (successCount > 0) {
    ElMessage.success(`成功上传 ${successCount} 个文件${errorCount > 0 ? `，${errorCount} 个失败` : ''}`)
    // 如果所有文件都上传完成（成功或失败），关闭对话框并刷新列表
    if (uploadFiles.value.every((f) => f.status === 'success' || f.status === 'error')) {
      setTimeout(() => {
        showUploadDialog.value = false
        loadFiles()
      }, 1000)
    }
  } else if (errorCount > 0) {
    ElMessage.error(`上传失败：${errorCount} 个文件`)
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
