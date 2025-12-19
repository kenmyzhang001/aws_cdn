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

      <!-- 按域名分组显示 -->
      <div v-loading="loading">
        <el-collapse v-model="activeDomains" v-if="groupedPackages.length > 0">
          <el-collapse-item
            v-for="group in groupedPackages"
            :key="group.domain_id"
            :name="group.domain_id"
          >
            <template #title>
              <div class="domain-header">
                <div class="domain-info">
                  <span class="domain-name">{{ group.domain_name }}</span>
                  <el-tag size="small" style="margin-left: 8px">
                    {{ group.files.length }} 个文件
                  </el-tag>
                  <el-tag
                    v-if="group.cloudfront_id"
                    :type="group.cloudfront_enabled ? 'success' : 'danger'"
                    size="small"
                    style="margin-left: 8px"
                  >
                    CloudFront: {{ group.cloudfront_enabled ? '已启用' : '已禁用' }}
                  </el-tag>
                </div>
                <el-button
                  size="small"
                  type="primary"
                  @click.stop="openAddFileDialog(group)"
                >
                  <el-icon><Plus /></el-icon>
                  添加文件
                </el-button>
              </div>
            </template>

            <el-table :data="group.files" stripe size="small">
              <el-table-column prop="file_name" label="文件名" min-width="200" />
              <el-table-column prop="file_size" label="文件大小" width="120">
                <template #default="{ row }">
                  {{ formatFileSize(row.file_size) }}
                </template>
              </el-table-column>
              <el-table-column prop="status" label="状态" width="100">
                <template #default="{ row }">
                  <el-tag :type="getStatusType(row.status)" size="small">
                    {{ getStatusText(row.status) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="下载URL" min-width="300">
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
              <el-table-column prop="created_at" label="创建时间" width="180">
                <template #default="{ row }">
                  {{ formatDate(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="200" fixed="right">
                <template #default="{ row }">
                  <el-button
                    size="small"
                    type="primary"
                    :loading="row.checking"
                    @click="checkPackage(row)"
                  >
                    检查
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
          </el-collapse-item>
        </el-collapse>

        <el-empty v-else description="暂无下载包" />
      </div>
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

    <!-- 添加文件对话框（用于已有域名） -->
    <el-dialog v-model="showAddFileDialog" title="添加文件" width="600px">
      <el-form :model="addFileForm" label-width="120px" :rules="addFileRules" ref="addFileFormRef">
        <el-form-item label="域名">
          <el-input v-model="addFileForm.domain_name" disabled />
        </el-form-item>
        <el-form-item label="选择文件" prop="file" required>
          <el-upload
            ref="addFileUploadRef"
            :auto-upload="false"
            :on-change="handleAddFileChange"
            :limit="1"
            :file-list="addFileList"
          >
            <template #trigger>
              <el-button type="primary">选择文件</el-button>
            </template>
          </el-upload>
          <div v-if="addFileSelectedFile" style="margin-top: 10px">
            <div>文件名: {{ addFileSelectedFile.name }}</div>
            <div>文件大小: {{ formatFileSize(addFileSelectedFile.size) }}</div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddFileDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAddFile" :loading="addFileLoading">
          开始上传
        </el-button>
      </template>
    </el-dialog>

    <!-- 检查状态对话框 -->
    <el-dialog v-model="showCheckDialog" title="检查下载包状态" width="700px">
      <div v-if="checkStatus">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="下载包记录">
            <el-tag :type="checkStatus.package_exists ? 'success' : 'danger'">
              {{ checkStatus.package_exists ? '存在' : '不存在' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="S3文件">
            <el-tag :type="checkStatus.s3_file_exists ? 'success' : 'danger'">
              {{ checkStatus.s3_file_exists ? '存在' : '不存在' }}
            </el-tag>
            <span v-if="checkStatus.s3_file_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.s3_file_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront分发">
            <el-tag :type="checkStatus.cloudfront_exists ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_exists ? '存在' : '不存在' }}
            </el-tag>
            <span v-if="checkStatus.cloudfront_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.cloudfront_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront启用状态">
            <el-tag :type="checkStatus.cloudfront_enabled ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_enabled ? '已启用' : '已禁用' }}
            </el-tag>
            <span v-if="checkStatus.cloudfront_enabled_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.cloudfront_enabled_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront源路径">
            <el-tag :type="checkStatus.cloudfront_origin_path_match ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_origin_path_match ? '匹配' : '不匹配' }}
            </el-tag>
            <div v-if="checkStatus.cloudfront_origin_path_current || checkStatus.cloudfront_origin_path_expected" style="margin-top: 5px; font-size: 12px; color: #606266">
              <div v-if="checkStatus.cloudfront_origin_path_current">
                当前: <code style="background: #f5f7fa; padding: 2px 4px; border-radius: 2px">{{ checkStatus.cloudfront_origin_path_current || '(空)' }}</code>
              </div>
              <div v-if="checkStatus.cloudfront_origin_path_expected" style="margin-top: 3px">
                期望: <code style="background: #f5f7fa; padding: 2px 4px; border-radius: 2px">{{ checkStatus.cloudfront_origin_path_expected || '(空)' }}</code>
              </div>
            </div>
            <span v-if="checkStatus.cloudfront_origin_path_error" style="color: #f56c6c; margin-left: 10px; display: block; margin-top: 5px">
              {{ checkStatus.cloudfront_origin_path_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="Route53 DNS记录">
            <el-tag :type="checkStatus.route53_dns_configured ? 'success' : 'danger'">
              {{ checkStatus.route53_dns_configured ? '已配置' : '未配置' }}
            </el-tag>
            <span v-if="checkStatus.route53_dns_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.route53_dns_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="下载URL可访问">
            <el-tag :type="checkStatus.download_url_accessible ? 'success' : 'danger'">
              {{ checkStatus.download_url_accessible ? '可访问' : '不可访问' }}
            </el-tag>
            <span v-if="checkStatus.download_url_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.download_url_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="证书">
            <el-tag :type="checkStatus.certificate_found ? 'success' : 'danger'">
              {{ checkStatus.certificate_found ? '已找到' : '未找到' }}
            </el-tag>
            <span v-if="checkStatus.certificate_arn" style="margin-left: 10px; font-size: 12px; color: #909399">
              {{ checkStatus.certificate_arn }}
            </span>
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="checkStatus.issues && checkStatus.issues.length > 0" style="margin-top: 20px">
          <h4 style="color: #f56c6c; margin-bottom: 10px">发现的问题：</h4>
          <ul>
            <li v-for="(issue, index) in checkStatus.issues" :key="index" style="color: #f56c6c; margin-bottom: 5px">
              {{ issue }}
            </li>
          </ul>
        </div>

        <div v-else style="margin-top: 20px">
          <el-alert type="success" :closable="false">所有检查项均正常</el-alert>
        </div>
      </div>
      <template #footer>
        <el-button @click="showCheckDialog = false">关闭</el-button>
        <el-button
          v-if="checkStatus && checkStatus.can_fix"
          type="primary"
          @click="handleFix"
          :loading="fixLoading"
        >
          修复
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
import { downloadPackageApi } from '@/api/download_package'

const loading = ref(false)
const packageList = ref([])
const groupedPackages = ref([])
const activeDomains = ref([])

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

// 添加文件到已有域名
const showAddFileDialog = ref(false)
const addFileLoading = ref(false)
const addFileForm = ref({
  domain_id: '',
  domain_name: '',
  file: null,
})
const addFileFormRef = ref(null)
const addFileList = ref([])
const addFileSelectedFile = ref(null)
const addFileUploadRef = ref(null)

// 检查修复相关
const showCheckDialog = ref(false)
const checkStatus = ref(null)
const checkPackageId = ref(null)
const fixLoading = ref(false)

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

const addFileRules = {
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

// 加载下载包列表并按域名分组
const loadPackages = async () => {
  loading.value = true
  try {
    const response = await request.get('/download-packages', {
      params: {
        page: 1,
        page_size: 1000, // 获取所有数据用于分组
      },
    })
    packageList.value = response.data || []
    
    // 按域名分组
    const grouped = {}
    packageList.value.forEach((pkg) => {
      const key = pkg.domain_id
      if (!grouped[key]) {
        grouped[key] = {
          domain_id: pkg.domain_id,
          domain_name: pkg.domain_name,
          cloudfront_id: pkg.cloudfront_id,
          cloudfront_domain: pkg.cloudfront_domain,
          cloudfront_status: pkg.cloudfront_status,
          cloudfront_enabled: pkg.cloudfront_enabled,
          files: [],
        }
      }
      grouped[key].files.push(pkg)
    })
    
    groupedPackages.value = Object.values(grouped)
    
    // 默认展开所有域名
    activeDomains.value = groupedPackages.value.map((g) => g.domain_id)
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

// 显示添加文件对话框
const openAddFileDialog = (group) => {
  addFileForm.value = {
    domain_id: group.domain_id,
    domain_name: group.domain_name,
    file: null,
  }
  addFileList.value = []
  addFileSelectedFile.value = null
  showAddFileDialog.value = true
}

// 处理添加文件选择
const handleAddFileChange = (file) => {
  addFileSelectedFile.value = file.raw
  addFileForm.value.file_name = file.name
  addFileForm.value.file = file.raw
  // 手动触发表单验证
  if (addFileFormRef.value) {
    addFileFormRef.value.validateField('file')
  }
}

// 添加文件到已有域名
const handleAddFile = async () => {
  if (!addFileFormRef.value) return

  // 先进行表单验证
  try {
    await addFileFormRef.value.validate()
  } catch (error) {
    return
  }

  // 验证通过后，再次检查文件
  if (!addFileSelectedFile.value) {
    ElMessage.warning('请选择文件')
    return
  }

  addFileLoading.value = true

  try {
    const formData = new FormData()
    formData.append('domain_id', addFileForm.value.domain_id)
    formData.append('file_name', addFileForm.value.file_name)
    formData.append('file', addFileSelectedFile.value)

    await request.post('/download-packages', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 300000, // 5分钟超时
    })

    ElMessage.success('上传成功，正在处理中...')
    showAddFileDialog.value = false
    addFileForm.value = {
      domain_id: '',
      domain_name: '',
      file: null,
    }
    addFileList.value = []
    addFileSelectedFile.value = null
    loadPackages()
  } catch (error) {
    ElMessage.error('上传失败: ' + (error.response?.data?.error || error.message))
  } finally {
    addFileLoading.value = false
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
      timeout: 300000, // 5分钟超时，适合大文件上传
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

// 检查下载包
const checkPackage = async (row) => {
  row.checking = true
  checkPackageId.value = row.id
  try {
    const res = await downloadPackageApi.checkDownloadPackage(row.id)
    checkStatus.value = res
    showCheckDialog.value = true
  } catch (error) {
    ElMessage.error('检查失败: ' + (error.response?.data?.error || error.message))
  } finally {
    row.checking = false
  }
}

// 修复下载包
const fixPackage = async (row) => {
  try {
    await ElMessageBox.confirm(
      '确定要修复这个下载包吗？修复将重新创建CloudFront分发和DNS记录。',
      '提示',
      {
        type: 'warning',
      }
    )
    row.fixing = true

    // 如果还没有检查状态，先检查一下
    if (!checkStatus.value || checkPackageId.value !== row.id) {
      await checkPackage(row)
    }

    await downloadPackageApi.fixDownloadPackage(row.id)
    ElMessage.success('修复成功')

    // 重新检查状态
    if (checkPackageId.value === row.id) {
      const res = await downloadPackageApi.checkDownloadPackage(row.id)
      checkStatus.value = res
    }

    loadPackages()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('修复失败: ' + (error.response?.data?.error || error.message))
    }
  } finally {
    row.fixing = false
  }
}

// 在检查对话框中点击修复
const handleFix = async () => {
  if (!checkPackageId.value) return
  // 在所有分组中查找
  let row = null
  for (const group of groupedPackages.value) {
    row = group.files.find((p) => p.id === checkPackageId.value)
    if (row) break
  }
  if (row) {
    await fixPackage(row)
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

// 获取CloudFront状态类型
const getCloudFrontStatusType = (status) => {
  const statusMap = {
    InProgress: 'warning',
    Deployed: 'success',
    Disabled: 'info',
  }
  return statusMap[status] || 'info'
}

// 获取CloudFront状态文本
const getCloudFrontStatusText = (status) => {
  const statusTextMap = {
    InProgress: '部署中',
    Deployed: '已部署',
    Disabled: '已禁用',
  }
  return statusTextMap[status] || status || '未知'
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

.domain-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  padding-right: 20px;
}

.domain-info {
  display: flex;
  align-items: center;
}

.domain-name {
  font-weight: 600;
  font-size: 16px;
}
</style>

