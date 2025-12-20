<template>
  <div class="download-package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>下载域名管理</span>
          <el-button type="primary" @click="openAddDomainDialog">
            <el-icon><Plus /></el-icon>
            添加下载域名
          </el-button>
        </div>
      </template>

      <!-- 域名列表 -->
      <div v-loading="loading">
        <div v-if="domainList.length > 0" class="domain-list">
          <el-card
            v-for="domain in domainList"
            :key="domain.id"
            class="domain-card"
            shadow="hover"
          >
            <template #header>
              <div class="domain-card-header">
                <div class="domain-info">
                  <span class="domain-name">{{ domain.domain_name }}</span>
                  <el-tag
                    :type="domain.certificate_status === 'issued' ? 'success' : 'warning'"
                    size="small"
                    style="margin-left: 8px"
                  >
                    证书: {{ getCertificateStatusText(domain.certificate_status) }}
                  </el-tag>
                  <el-tag size="small" style="margin-left: 8px">
                    {{ domain.file_count || 0 }} 个文件
                  </el-tag>
                  <el-tag
                    v-if="domain.cloudfront_id"
                    :type="domain.cloudfront_enabled ? 'success' : 'danger'"
                    size="small"
                    style="margin-left: 8px"
                  >
                    CloudFront: {{ domain.cloudfront_enabled ? '已启用' : '已禁用' }}
                  </el-tag>
                </div>
                <div class="domain-actions">
                  <el-button
                    size="small"
                    type="primary"
                    @click="openAddFileDialog(domain)"
                  >
                    <el-icon><Plus /></el-icon>
                    添加APK文件
                  </el-button>
                </div>
              </div>
            </template>

            <!-- 文件列表 -->
            <div v-if="domain.files && domain.files.length > 0">
              <el-table :data="domain.files" stripe size="small" border>
                <el-table-column prop="file_name" label="文件名" min-width="200">
                  <template #default="{ row }">
                    <el-icon style="margin-right: 4px"><Document /></el-icon>
                    {{ row.file_name }}
                  </template>
                </el-table-column>
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
                    <div v-if="row.download_url" style="display: flex; align-items: center; gap: 8px;">
                      <el-link
                        :href="row.download_url"
                        target="_blank"
                        type="primary"
                        style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
                      >
                        {{ row.download_url }}
                      </el-link>
                      <el-button
                        size="small"
                        :icon="CopyDocument"
                        circle
                        @click="copyDownloadUrl(row)"
                        title="复制链接"
                      />
                    </div>
                    <span v-else style="color: #909399">-</span>
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
            </div>
            <el-empty
              v-else
              description="该域名下暂无文件，点击上方按钮添加APK文件"
              :image-size="80"
            />
          </el-card>
        </div>

        <el-empty v-else description="暂无下载域名，请先添加下载域名" />
      </div>
    </el-card>

    <!-- 添加下载域名对话框 -->
    <el-dialog v-model="showAddDomainDialog" title="添加下载域名" width="600px">
      <el-form :model="addDomainForm" label-width="120px" :rules="addDomainRules" ref="addDomainFormRef">
        <el-form-item label="选择域名" prop="domain_id" required>
          <el-select
            v-model="addDomainForm.domain_id"
            placeholder="请选择已签发证书且未被使用的域名"
            style="width: 100%"
            filterable
          >
            <el-option
              v-for="domain in availableDomains"
              :key="domain.id"
              :label="domain.domain_name"
              :value="domain.id"
            >
              <div>
                <span>{{ domain.domain_name }}</span>
                <el-tag
                  :type="domain.certificate_status === 'issued' ? 'success' : 'warning'"
                  size="small"
                  style="margin-left: 8px"
                >
                  {{ getCertificateStatusText(domain.certificate_status) }}
                </el-tag>
              </div>
            </el-option>
          </el-select>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            只显示证书已签发且未被重定向使用的域名
          </div>
        </el-form-item>
        <el-form-item label="上传第一个APK文件" prop="file">
          <el-upload
            ref="addDomainUploadRef"
            :auto-upload="false"
            :on-change="handleAddDomainFileChange"
            :limit="1"
            :file-list="addDomainFileList"
          >
            <template #trigger>
              <el-button type="primary">选择APK文件</el-button>
            </template>
          </el-upload>
          <div v-if="addDomainSelectedFile" style="margin-top: 10px">
            <div>文件名: {{ addDomainSelectedFile.name }}</div>
            <div>文件大小: {{ formatFileSize(addDomainSelectedFile.size) }}</div>
          </div>
          <div v-if="addDomainLoading" style="margin-top: 15px">
            <el-progress
              v-if="addDomainUploadProgress > 0"
              :percentage="addDomainUploadProgress"
              :status="addDomainUploadProgress === 100 ? 'success' : null"
              :stroke-width="8"
            />
            <div v-else style="text-align: center; color: #909399; font-size: 12px">
              准备上传...
            </div>
            <div v-if="addDomainUploadProgress > 0" style="text-align: center; margin-top: 5px; color: #909399; font-size: 12px">
              上传中... {{ addDomainUploadProgress }}%
            </div>
          </div>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            可选：可以先添加域名，稍后再上传文件
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDomainDialog = false" :disabled="addDomainLoading">取消</el-button>
        <el-button type="primary" @click="handleAddDomain" :loading="addDomainLoading">
          {{ addDomainLoading ? '上传中...' : '确定' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 添加文件对话框（用于已有域名） -->
    <el-dialog v-model="showAddFileDialog" title="添加APK文件" width="600px">
      <el-form :model="addFileForm" label-width="120px" :rules="addFileRules" ref="addFileFormRef">
        <el-form-item label="域名">
          <el-input v-model="addFileForm.domain_name" disabled />
        </el-form-item>
        <el-form-item label="选择APK文件" prop="file" required>
          <el-upload
            ref="addFileUploadRef"
            :auto-upload="false"
            :on-change="handleAddFileChange"
            :limit="1"
            :file-list="addFileList"
            accept=".apk"
          >
            <template #trigger>
              <el-button type="primary">选择APK文件</el-button>
            </template>
          </el-upload>
          <div v-if="addFileSelectedFile" style="margin-top: 10px">
            <div>文件名: {{ addFileSelectedFile.name }}</div>
            <div>文件大小: {{ formatFileSize(addFileSelectedFile.size) }}</div>
          </div>
          <div v-if="addFileLoading" style="margin-top: 15px">
            <el-progress
              v-if="addFileUploadProgress > 0"
              :percentage="addFileUploadProgress"
              :status="addFileUploadProgress === 100 ? 'success' : null"
              :stroke-width="8"
            />
            <div v-else style="text-align: center; color: #909399; font-size: 12px">
              准备上传...
            </div>
            <div v-if="addFileUploadProgress > 0" style="text-align: center; margin-top: 5px; color: #909399; font-size: 12px">
              上传中... {{ addFileUploadProgress }}%
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddFileDialog = false" :disabled="addFileLoading">取消</el-button>
        <el-button type="primary" @click="handleAddFile" :loading="addFileLoading">
          {{ addFileLoading ? '上传中...' : '开始上传' }}
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
import { Plus, Document, CopyDocument } from '@element-plus/icons-vue'
import request from '@/api/request'
import { domainApi } from '@/api/domain'
import { downloadPackageApi } from '@/api/download_package'
import { uploadFile } from '@/utils/upload'

const loading = ref(false)
const domainList = ref([]) // 域名列表，每个域名包含其下的文件列表

// 添加下载域名
const showAddDomainDialog = ref(false)
const addDomainLoading = ref(false)
const addDomainUploadProgress = ref(0)
const addDomainForm = ref({
  domain_id: '',
  file_name: '',
  file: null,
})
const addDomainFormRef = ref(null)
const addDomainFileList = ref([])
const addDomainSelectedFile = ref(null)
const addDomainUploadRef = ref(null)
const availableDomains = ref([]) // 可用于添加的域名列表

// 添加文件到已有域名
const showAddFileDialog = ref(false)
const addFileLoading = ref(false)
const addFileUploadProgress = ref(0)
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

const addDomainRules = {
  domain_id: [{ required: true, message: '请选择下载域名', trigger: 'change' }],
}

const addFileRules = {
  file: [
    {
      required: true,
      message: '请选择APK文件',
      trigger: 'change',
      validator: (rule, value, callback) => {
        if (!value) {
          callback(new Error('请选择APK文件'))
        } else {
          callback()
        }
      },
    },
  ],
}

// 加载下载域名列表
const loadDomains = async () => {
  loading.value = true
  try {
    // 1. 获取所有可用于下载的域名（证书已签发且未被重定向使用）
    const domainResponse = await domainApi.getDomainList({ page: 1, page_size: 1000 })
    const allDomains = (domainResponse.data || []).filter(
      (d) => d.certificate_status === 'issued' && !d.used_by_redirect
    )

    // 2. 获取所有下载包
    const packageResponse = await request.get('/download-packages', {
      params: {
        page: 1,
        page_size: 1000,
      },
    })
    const allPackages = packageResponse.data || []

    // 3. 按域名组织数据（只处理有文件的域名）
    const domainMap = {}
    const domainInfoMap = {} // 用于存储域名信息
    
    // 创建域名信息映射
    allDomains.forEach((domain) => {
      domainInfoMap[domain.id] = domain
    })

    // 只处理有文件的域名
    allPackages.forEach((pkg) => {
      const domainInfo = domainInfoMap[pkg.domain_id]
      if (!domainInfo) return // 如果域名不存在，跳过
      
      if (!domainMap[pkg.domain_id]) {
        // 初始化域名信息
        domainMap[pkg.domain_id] = {
          id: domainInfo.id,
          domain_name: domainInfo.domain_name,
          certificate_status: domainInfo.certificate_status,
          file_count: 0,
          files: [],
          cloudfront_id: null,
          cloudfront_domain: null,
          cloudfront_status: null,
          cloudfront_enabled: false,
        }
      }
      
      // 添加文件
      domainMap[pkg.domain_id].files.push(pkg)
      domainMap[pkg.domain_id].file_count++
      
      // 更新CloudFront信息（使用第一个文件的CloudFront信息）
      if (pkg.cloudfront_id && !domainMap[pkg.domain_id].cloudfront_id) {
        domainMap[pkg.domain_id].cloudfront_id = pkg.cloudfront_id
        domainMap[pkg.domain_id].cloudfront_domain = pkg.cloudfront_domain
        domainMap[pkg.domain_id].cloudfront_status = pkg.cloudfront_status
        domainMap[pkg.domain_id].cloudfront_enabled = pkg.cloudfront_enabled
      }
    })

    // 只显示有文件的域名
    domainList.value = Object.values(domainMap)
  } catch (error) {
    ElMessage.error('加载下载域名列表失败: ' + (error.response?.data?.error || error.message))
  } finally {
    loading.value = false
  }
}

// 加载可用域名列表（用于添加新域名）
const loadAvailableDomains = async () => {
  try {
    const response = await domainApi.getDomainList({ page: 1, page_size: 1000 })
    // 过滤：只显示证书已签发且未被重定向使用的域名
    const allAvailable = (response.data || []).filter(
      (d) => d.certificate_status === 'issued' && !d.used_by_redirect
    )
    
    // 排除已经在下载域名列表中的域名
    const existingDomainIds = new Set(domainList.value.map((d) => d.id))
    availableDomains.value = allAvailable.filter((d) => !existingDomainIds.has(d.id))
  } catch (error) {
    console.error('加载域名列表失败:', error)
  }
}

// 打开添加下载域名对话框
const openAddDomainDialog = async () => {
  // 重置表单
  addDomainForm.value = {
    domain_id: '',
    file_name: '',
    file: null,
  }
  addDomainFileList.value = []
  addDomainSelectedFile.value = null
  
  // 加载可用域名列表
  await loadAvailableDomains()
  
  // 显示对话框
  showAddDomainDialog.value = true
}

// 处理添加域名时的文件选择
const handleAddDomainFileChange = (file) => {
  addDomainSelectedFile.value = file.raw
  addDomainForm.value.file_name = file.name
  addDomainForm.value.file = file.raw
}

// 添加下载域名
const handleAddDomain = async () => {
  if (!addDomainFormRef.value) return

  // 先进行表单验证
  try {
    await addDomainFormRef.value.validate()
  } catch (error) {
    return
  }

  // 如果没有选择文件，只添加域名（不创建下载包）
  if (!addDomainSelectedFile.value) {
    ElMessage.success('域名已添加，您可以稍后上传文件')
    showAddDomainDialog.value = false
    addDomainForm.value = {
      domain_id: '',
      file_name: '',
      file: null,
    }
    addDomainFileList.value = []
    addDomainSelectedFile.value = null
    loadDomains()
    loadAvailableDomains()
    return
  }

  // 如果有文件，上传文件（会自动创建域名关联）
  addDomainLoading.value = true
  addDomainUploadProgress.value = 0

  try {
    const formData = new FormData()
    formData.append('domain_id', addDomainForm.value.domain_id)
    formData.append('file_name', addDomainForm.value.file_name)
    formData.append('file', addDomainSelectedFile.value)

    await uploadFile(
      '/download-packages',
      formData,
      { timeout: 600000 },
      (progress) => {
        addDomainUploadProgress.value = progress
      }
    )

    ElMessage.success('上传成功，正在处理中...')
    showAddDomainDialog.value = false
    addDomainForm.value = {
      domain_id: '',
      file_name: '',
      file: null,
    }
    addDomainFileList.value = []
    addDomainSelectedFile.value = null
    addDomainUploadProgress.value = 0
    loadDomains()
    loadAvailableDomains()
  } catch (error) {
    ElMessage.error('上传失败: ' + (error.response?.data?.error || error.message))
  } finally {
    addDomainLoading.value = false
    addDomainUploadProgress.value = 0
  }
}

// 显示添加文件对话框
const openAddFileDialog = (domain) => {
  addFileForm.value = {
    domain_id: domain.id,
    domain_name: domain.domain_name,
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
  addFileUploadProgress.value = 0

  try {
    const formData = new FormData()
    formData.append('domain_id', addFileForm.value.domain_id)
    formData.append('file_name', addFileForm.value.file_name)
    formData.append('file', addFileSelectedFile.value)

    await uploadFile(
      '/download-packages',
      formData,
      { timeout: 600000 },
      (progress) => {
        addFileUploadProgress.value = progress
      }
    )

    ElMessage.success('上传成功，正在处理中...')
    showAddFileDialog.value = false
    addFileForm.value = {
      domain_id: '',
      domain_name: '',
      file: null,
    }
    addFileList.value = []
    addFileSelectedFile.value = null
    addFileUploadProgress.value = 0
    loadDomains()
  } catch (error) {
    ElMessage.error('上传失败: ' + (error.response?.data?.error || error.message))
  } finally {
    addFileLoading.value = false
    addFileUploadProgress.value = 0
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
    loadDomains()
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

    loadDomains()
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
  // 在所有域名的文件中查找
  let row = null
  for (const domain of domainList.value) {
    row = domain.files.find((p) => p.id === checkPackageId.value)
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

// 获取证书状态文本
const getCertificateStatusText = (status) => {
  const statusMap = {
    pending: '待签发',
    issued: '已签发',
    failed: '失败',
    pending_validation: '验证中',
  }
  return statusMap[status] || status || '未知'
}

onMounted(() => {
  loadDomains()
  loadAvailableDomains()
  // 每30秒刷新一次状态
  setInterval(() => {
    loadDomains()
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

.domain-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.domain-card {
  margin-bottom: 0;
}

.domain-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.domain-info {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.domain-name {
  font-weight: 600;
  font-size: 16px;
}

.domain-actions {
  display: flex;
  gap: 8px;
}
</style>

