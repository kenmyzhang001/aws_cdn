<template>
  <div class="download-package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>下载包管理</span>
          <el-button type="primary" @click="showUploadDialog = true">
            <el-icon><Plus /></el-icon>
            上传下载包
          </el-button>
        </div>
      </template>

      <el-table :data="packageList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="domain_name" label="域名" />
        <el-table-column prop="file_name" label="文件名" />
        <el-table-column prop="file_size" label="文件大小">
          <template #default="{ row }">
            {{ formatFileSize(row.file_size) }}
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="download_url" label="下载URL">
          <template #default="{ row }">
            <el-link
              v-if="row.download_url"
              :href="row.download_url"
              target="_blank"
              type="primary"
            >
              {{ row.download_url }}
            </el-link>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button
              v-if="row.download_url"
              size="small"
              type="success"
              @click="copyDownloadUrl(row)"
            >
              复制链接
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="handleDelete(row)"
            >
              删除
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
        @size-change="loadPackages"
        @current-change="loadPackages"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 上传下载包对话框 -->
    <el-dialog v-model="showUploadDialog" title="上传下载包" width="600px">
      <el-form :model="uploadForm" label-width="120px" :rules="uploadRules" ref="uploadFormRef">
        <el-form-item label="选择下载域名" prop="domain_id" required>
          <el-select
            v-model="uploadForm.domain_id"
            placeholder="请选择已签发证书的域名"
            style="width: 100%"
            filterable
          >
            <el-option
              v-for="domain in availableDomains"
              :key="domain.id"
              :label="domain.domain_name"
              :value="domain.id"
            >
              <span>{{ domain.domain_name }}</span>
            </el-option>
          </el-select>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            只显示证书已签发且未被重定向使用的域名
          </div>
        </el-form-item>
        <el-form-item label="选择文件" prop="file" required>
          <el-upload
            ref="uploadRef"
            :auto-upload="false"
            :on-change="handleFileChange"
            :limit="1"
            :file-list="fileList"
          >
            <template #trigger>
              <el-button type="primary">选择文件</el-button>
            </template>
          </el-upload>
          <div v-if="selectedFile" style="margin-top: 10px">
            <div>文件名: {{ selectedFile.name }}</div>
            <div>文件大小: {{ formatFileSize(selectedFile.size) }}</div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showUploadDialog = false">取消</el-button>
        <el-button type="primary" @click="handleUpload" :loading="uploadLoading">
          开始上传
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import request from '@/api/request'
import { domainApi } from '@/api/domain'

const loading = ref(false)
const packageList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)

const showUploadDialog = ref(false)
const uploadLoading = ref(false)
const uploadForm = ref({
  domain_id: '',
  file: null,
})
const uploadFormRef = ref(null)
const fileList = ref([])
const selectedFile = ref(null)
const availableDomains = ref([])

const uploadRules = {
  domain_id: [{ required: true, message: '请选择下载域名', trigger: 'change' }],
  file: [
    {
      required: true,
      message: '请选择文件',
      trigger: 'change',
      validator: (rule, value, callback) => {
        if (!value) {
          callback(new Error('请选择文件'))
        } else {
          callback()
        }
      },
    },
  ],
}

// 加载下载包列表
const loadPackages = async () => {
  loading.value = true
  try {
    const response = await request.get('/download-packages', {
      params: {
        page: currentPage.value,
        page_size: pageSize.value,
      },
    })
    packageList.value = response.data
    total.value = response.total
  } catch (error) {
    ElMessage.error('加载下载包列表失败: ' + (error.response?.data?.error || error.message))
  } finally {
    loading.value = false
  }
}

// 加载可用域名列表（只显示未被重定向使用的域名）
const loadAvailableDomains = async () => {
  try {
    const response = await domainApi.getDomainList({ page: 1, page_size: 1000 })
    // 过滤：只显示证书已签发且未被重定向使用的域名
    availableDomains.value = (response.data || []).filter(
      (d) => d.certificate_status === 'issued' && !d.used_by_redirect
    )
  } catch (error) {
    console.error('加载域名列表失败:', error)
  }
}

// 处理文件选择
const handleFileChange = (file) => {
  selectedFile.value = file.raw
  uploadForm.value.file_name = file.name
  uploadForm.value.file = file.raw // 设置文件到表单数据，用于验证
  // 手动触发表单验证
  if (uploadFormRef.value) {
    uploadFormRef.value.validateField('file')
  }
}

// 上传文件
const handleUpload = async () => {
  if (!uploadFormRef.value) return

  // 先进行表单验证
  try {
    await uploadFormRef.value.validate()
  } catch (error) {
    // 验证失败，直接返回
    return
  }

  // 验证通过后，再次检查文件
  if (!selectedFile.value) {
    ElMessage.warning('请选择文件')
    return
  }

  uploadLoading.value = true

  try {
    const formData = new FormData()
    formData.append('domain_id', uploadForm.value.domain_id)
    formData.append('file_name', uploadForm.value.file_name)
    formData.append('file', selectedFile.value)

    const response = await request.post('/download-packages', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    })

    ElMessage.success('上传成功，正在处理中...')
    showUploadDialog.value = false
    uploadForm.value = {
      domain_id: '',
      file: null,
    }
    fileList.value = []
    selectedFile.value = null
    loadPackages()
  } catch (error) {
    ElMessage.error('上传失败: ' + (error.response?.data?.error || error.message))
  } finally {
    uploadLoading.value = false
  }
}

// 删除下载包
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个下载包吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })

    await request.delete(`/download-packages/${row.id}`)
    ElMessage.success('删除成功')
    loadPackages()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.response?.data?.error || error.message))
    }
  }
}

// 复制下载链接
const copyDownloadUrl = (row) => {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(row.download_url).then(() => {
      ElMessage.success('链接已复制到剪贴板')
    })
  } else {
    // 降级方案
    const textarea = document.createElement('textarea')
    textarea.value = row.download_url
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
    ElMessage.success('链接已复制到剪贴板')
  }
}

// 格式化文件大小
const formatFileSize = (bytes) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i]
}

// 格式化日期
const formatDate = (dateString) => {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString('zh-CN')
}

// 获取状态类型
const getStatusType = (status) => {
  const statusMap = {
    pending: 'info',
    uploading: 'warning',
    processing: 'warning',
    completed: 'success',
    failed: 'danger',
  }
  return statusMap[status] || 'info'
}

// 获取状态文本
const getStatusText = (status) => {
  const statusMap = {
    pending: '待处理',
    uploading: '上传中',
    processing: '处理中',
    completed: '已完成',
    failed: '失败',
  }
  return statusMap[status] || status
}

onMounted(() => {
  loadPackages()
  loadAvailableDomains()
  // 每30秒刷新一次状态
  setInterval(() => {
    loadPackages()
  }, 30000)
})
</script>

<style scoped>
.download-package-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

