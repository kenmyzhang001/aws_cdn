<template>
  <div class="redirect-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>重定向管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建重定向规则
          </el-button>
        </div>
      </template>

      <el-table :data="redirectList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column label="域名信息" width="280">
          <template #default="{ row }">
            <div style="display: flex; flex-direction: column; gap: 6px;">
              <div style="font-weight: 500;">{{ row.source_domain }}</div>
              <div style="display: flex; gap: 8px; align-items: center;">
                <span style="font-size: 12px; color: #909399;">域名:</span>
                <el-tag v-if="row.domain_status" :type="getDomainStatusType(row.domain_status)" size="small">
                  {{ getDomainStatusText(row.domain_status) }}
                </el-tag>
                <span v-else style="color: #c0c4cc; font-size: 12px;">未找到</span>
                <span style="font-size: 12px; color: #909399; margin-left: 8px;">证书:</span>
                <el-tag v-if="row.certificate_status" :type="getCertificateStatusType(row.certificate_status)" size="small">
                  {{ getCertificateStatusText(row.certificate_status) }}
                </el-tag>
                <span v-else style="color: #c0c4cc; font-size: 12px;">未申请</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="目标 URL" min-width="200">
          <template #default="{ row }">
            <div v-for="target in row.targets" :key="target.id" style="margin-bottom: 5px">
              <el-tag
                :type="getURLStatusType(target.url_status)"
                style="margin-right: 5px"
              >
                {{ getURLStatusText(target.url_status) }}
              </el-tag>
              <span>{{ target.target_url }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="CloudFront" width="240">
          <template #default="{ row }">
            <div style="display: flex; flex-direction: column; gap: 6px;">
              <div v-if="row.cloudfront_id" style="font-size: 12px; color: #606266; font-family: monospace;">
                {{ row.cloudfront_id }}
              </div>
              <div v-else style="color: #c0c4cc; font-size: 12px;">未绑定</div>
              <div style="display: flex; align-items: center;">
                <el-tag v-if="row.cloudfront_status" :type="getCloudFrontStatusType(row.cloudfront_status)" size="small">
                  {{ getCloudFrontStatusText(row.cloudfront_status) }}
                </el-tag>
                <span v-else style="color: #c0c4cc; font-size: 12px;">未部署</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="DNS 记录" width="200">
          <template #default="{ row }">
            <div style="display: flex; flex-direction: column; gap: 6px;">
              <div style="display: flex; align-items: center; gap: 6px;">
                <span style="font-size: 12px; color: #909399;">Route 53:</span>
                <el-tag v-if="row.route53_dns_status" :type="getRoute53DNSStatusType(row.route53_dns_status)" size="small">
                  {{ getRoute53DNSStatusText(row.route53_dns_status) }}
                </el-tag>
                <span v-else style="color: #c0c4cc; font-size: 12px;">未检查</span>
              </div>
              <div style="display: flex; align-items: center; gap: 6px;">
                <span style="font-size: 12px; color: #909399;">www CNAME:</span>
                <el-tag v-if="row.www_cname_status" :type="getRoute53DNSStatusType(row.www_cname_status)" size="small">
                  {{ getRoute53DNSStatusText(row.www_cname_status) }}
                </el-tag>
                <span v-else-if="row.source_domain && row.source_domain.startsWith('www.')" style="color: #c0c4cc; font-size: 12px;">不适用</span>
                <span v-else style="color: #c0c4cc; font-size: 12px;">未检查</span>
              </div>
              <div style="display: flex; align-items: center; gap: 6px;">
                <span style="font-size: 12px; color: #909399;">S3 Policy:</span>
                <el-tag v-if="row.s3_bucket_policy_status" :type="getRoute53DNSStatusType(row.s3_bucket_policy_status)" size="small">
                  {{ getRoute53DNSStatusText(row.s3_bucket_policy_status) }}
                </el-tag>
                <span v-else style="color: #c0c4cc; font-size: 12px;">未检查</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="350">
          <template #default="{ row }">
            <el-button size="small" @click="viewDetails(row)">详情</el-button>
            <el-button size="small" type="success" @click="addTarget(row)">添加目标</el-button>
            <el-button size="small" type="info" @click="checkRule(row)">检查</el-button>
            <el-button size="small" type="warning" @click="fixRule(row)">修复</el-button>
            <el-button size="small" type="danger" @click="deleteRule(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadRedirects"
        @current-change="loadRedirects"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 创建重定向规则对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建重定向规则" width="700px" @close="resetCreateForm" @open="loadAvailableDomains">
      <el-form :model="createForm" label-width="120px">
        <el-form-item label="源域名" required>
          <el-select
            v-model="createForm.source_domain"
            placeholder="选择或输入域名"
            filterable
            allow-create
            default-first-option
            style="width: 100%"
          >
            <el-option
              v-for="domain in availableDomains"
              :key="domain.id"
              :label="domain.domain_name"
              :value="domain.domain_name"
            >
              <span>{{ domain.domain_name }}</span>
              <el-tag
                v-if="domain.certificate_status === 'issued'"
                size="small"
                type="success"
                style="margin-left: 10px"
              >
                证书已签发
              </el-tag>
            </el-option>
          </el-select>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            下拉列表只显示未被下载包使用的域名，也可以手动输入新域名
          </div>
        </el-form-item>
        <el-form-item label="目标 URL" required>
          <el-input
            type="textarea"
            v-model="targetUrlInput"
            :rows="6"
            placeholder="请输入目标 URL，每行一个，或使用逗号分隔&#10;例如：&#10;https://target1.com&#10;https://target2.com&#10;或者：&#10;https://target1.com, https://target2.com"
            @blur="parseTargetUrls"
            @paste="handlePaste"
          />
          <div style="margin-top: 10px">
            <div style="margin-bottom: 5px; color: #909399; font-size: 12px">
              已添加 {{ createForm.target_urls.length }} 个目标 URL
            </div>
            <el-tag
              v-for="(url, index) in createForm.target_urls"
              :key="index"
              closable
              @close="removeTargetUrl(index)"
              style="margin-right: 5px; margin-bottom: 5px"
            >
              {{ url }}
            </el-tag>
            <div v-if="createForm.target_urls.length === 0" style="color: #c0c4cc; font-size: 12px; margin-top: 5px">
              提示：输入URL后，点击输入框外部或粘贴内容会自动解析添加
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="createLoading">
          创建
        </el-button>
      </template>
    </el-dialog>

    <!-- 详情对话框 -->
    <el-dialog v-model="showDetailDialog" title="重定向规则详情" width="700px">
      <el-descriptions :column="1" border v-if="currentRule">
        <el-descriptions-item label="源域名">
          {{ currentRule.source_domain }}
        </el-descriptions-item>
        <el-descriptions-item label="CloudFront ID">
          {{ currentRule.cloudfront_id || '未绑定' }}
        </el-descriptions-item>
        <el-descriptions-item label="目标 URL">
          <el-table :data="currentRule.targets" size="small" border>
            <el-table-column prop="target_url" label="URL" />
            <el-table-column prop="weight" label="权重" width="80" />
            <el-table-column label="状态" width="80">
              <template #default="{ row }">
                <el-tag :type="row.is_active ? 'success' : 'info'">
                  {{ row.is_active ? '活跃' : '禁用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button
                  size="small"
                  type="danger"
                  @click="removeTarget(row.id)"
                >
                  删除
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <!-- 添加目标对话框 -->
    <el-dialog v-model="showAddTargetDialog" title="添加目标 URL" width="500px">
      <el-form :model="targetForm" label-width="100px">
        <el-form-item label="目标 URL" required>
          <el-input v-model="targetForm.target_url" placeholder="例如: https://target.com" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddTargetDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAddTarget" :loading="addTargetLoading">
          添加
        </el-button>
      </template>
    </el-dialog>

    <!-- 检查状态对话框 -->
    <el-dialog v-model="showCheckDialog" title="检查重定向规则状态" width="600px">
      <div v-if="checkStatus">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="规则存在">
            <el-tag :type="checkStatus.rule_exists ? 'success' : 'danger'">
              {{ checkStatus.rule_exists ? '是' : '否' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="HTML文件已上传">
            <el-tag :type="checkStatus.html_uploaded ? 'success' : 'danger'">
              {{ checkStatus.html_uploaded ? '是' : '否' }}
            </el-tag>
            <span v-if="checkStatus.html_upload_error" style="color: #f56c6c; margin-left: 10px">
              ({{ checkStatus.html_upload_error }})
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="S3 Bucket Policy">
            <el-tag :type="checkStatus.s3_bucket_policy_configured ? 'success' : 'danger'">
              {{ checkStatus.s3_bucket_policy_configured ? '已配置' : '未配置' }}
            </el-tag>
            <span v-if="checkStatus.s3_bucket_policy_error" style="color: #f56c6c; margin-left: 10px">
              ({{ checkStatus.s3_bucket_policy_error }})
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront分发存在">
            <el-tag :type="checkStatus.cloudfront_exists ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_exists ? '是' : '否' }}
            </el-tag>
            <span v-if="checkStatus.cloudfront_error" style="color: #f56c6c; margin-left: 10px">
              ({{ checkStatus.cloudfront_error }})
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="Route 53 DNS记录">
            <el-tag :type="checkStatus.route53_dns_configured ? 'success' : 'danger'">
              {{ checkStatus.route53_dns_configured ? '已配置' : '未配置' }}
            </el-tag>
            <span v-if="checkStatus.route53_dns_error" style="color: #f56c6c; margin-left: 10px">
              ({{ checkStatus.route53_dns_error }})
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="www CNAME记录">
            <el-tag :type="checkStatus.www_cname_configured ? 'success' : 'danger'">
              {{ checkStatus.www_cname_configured ? '已配置' : '未配置' }}
            </el-tag>
            <span v-if="checkStatus.www_cname_error" style="color: #f56c6c; margin-left: 10px">
              ({{ checkStatus.www_cname_error }})
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="证书已找到">
            <el-tag :type="checkStatus.certificate_found ? 'success' : 'warning'">
              {{ checkStatus.certificate_found ? '是' : '否' }}
            </el-tag>
            <span v-if="checkStatus.certificate_arn" style="margin-left: 10px; font-size: 12px; color: #909399">
              ({{ checkStatus.certificate_arn }})
            </span>
          </el-descriptions-item>
        </el-descriptions>
        
        <div v-if="checkStatus.issues && checkStatus.issues.length > 0" style="margin-top: 20px">
          <el-alert
            title="发现的问题"
            type="warning"
            :closable="false"
            show-icon
          >
            <ul style="margin: 10px 0; padding-left: 20px">
              <li v-for="(issue, index) in checkStatus.issues" :key="index">{{ issue }}</li>
            </ul>
          </el-alert>
        </div>
        
        <div v-else style="margin-top: 20px">
          <el-alert
            title="检查通过"
            type="success"
            :closable="false"
            show-icon
          >
            所有检查项均正常
          </el-alert>
        </div>
      </div>
      <template #footer>
        <el-button @click="showCheckDialog = false">关闭</el-button>
        <el-button type="primary" @click="handleFix" :loading="fixLoading">
          修复
        </el-button>
      </template>
    </el-dialog>

  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { redirectApi } from '@/api/redirect'
import { domainApi } from '@/api/domain'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const loading = ref(false)
const redirectList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createForm = ref({
  source_domain: '',
  target_urls: [],
})
const targetUrlInput = ref('')

const showDetailDialog = ref(false)
const currentRule = ref(null)

const showAddTargetDialog = ref(false)
const addTargetLoading = ref(false)
const targetForm = ref({
  target_url: '',
})
const currentRuleId = ref(null)

const showCheckDialog = ref(false)
const checkStatus = ref(null)
const checkLoading = ref(false)
const fixLoading = ref(false)
const checkRuleId = ref(null)

const availableDomains = ref([])

onMounted(() => {
  loadRedirects()
})

// 加载可用域名列表（只显示未被下载包使用的域名）
const loadAvailableDomains = async () => {
  try {
    const response = await domainApi.getDomainList({ page: 1, page_size: 1000 })
    // 过滤：只显示未被下载包使用的域名（允许手动输入，所以不过滤证书状态）
    // 但下拉列表中只显示未被下载包使用的域名
    availableDomains.value = (response.data || []).filter(
      (d) => !d.used_by_download_package
    )
  } catch (error) {
    console.error('加载域名列表失败:', error)
  }
}

const loadRedirects = async () => {
  loading.value = true
  try {
    const res = await redirectApi.getRedirectList({
      page: currentPage.value,
      page_size: pageSize.value,
    })
    redirectList.value = res.data
    total.value = res.total
  } catch (error) {
    ElMessage.error('加载重定向列表失败')
  } finally {
    loading.value = false
  }
}

// 解析目标URL（支持换行和逗号分隔）
const parseTargetUrls = () => {
  if (!targetUrlInput.value.trim()) {
    return
  }

  // 支持换行分隔
  let urls = targetUrlInput.value
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)

  // 如果只有一行，尝试用逗号分隔
  if (urls.length === 1 && urls[0].includes(',')) {
    urls = urls[0]
      .split(',')
      .map((url) => url.trim())
      .filter((url) => url.length > 0)
  }

  // 验证URL格式并添加到列表（去重）
  urls.forEach((url) => {
    // 简单的URL验证
    if (url && (url.startsWith('http://') || url.startsWith('https://'))) {
      // 检查是否已存在
      if (!createForm.value.target_urls.includes(url)) {
        createForm.value.target_urls.push(url)
      }
    }
  })

  // 清空输入框
  targetUrlInput.value = ''
}

// 处理粘贴事件
const handlePaste = (event) => {
  // 延迟解析，等待粘贴内容写入
  setTimeout(() => {
    parseTargetUrls()
  }, 10)
}

// 添加单个URL（保留原有功能，用于兼容）
const addTargetUrl = () => {
  parseTargetUrls()
}

const removeTargetUrl = (index) => {
  createForm.value.target_urls.splice(index, 1)
}

// 重置创建表单
const resetCreateForm = () => {
  createForm.value = {
    source_domain: '',
    target_urls: [],
  }
  targetUrlInput.value = ''
}

const handleCreate = async () => {
  if (!createForm.value.source_domain || createForm.value.target_urls.length === 0) {
    ElMessage.warning('请填写源域名和至少一个目标 URL')
    return
  }

  // 如果输入框还有内容，先解析
  if (targetUrlInput.value.trim()) {
    parseTargetUrls()
  }

  // 再次检查
  if (createForm.value.target_urls.length === 0) {
    ElMessage.warning('请至少添加一个有效的目标 URL')
    return
  }

  createLoading.value = true
  try {
    // 构建请求数据（系统会自动查找证书并创建CloudFront）
    const requestData = {
      source_domain: createForm.value.source_domain,
      target_urls: createForm.value.target_urls,
    }

    const res = await redirectApi.createRedirectRule(requestData)
    
    // 检查是否有警告信息
    if (res.warnings && res.warnings.length > 0) {
      // 检查是否有权限错误
      const hasPermissionError = res.warnings.some(w => 
        w.includes('访问被拒绝') || 
        w.includes('AccessDenied') || 
        w.includes('Access Denied') ||
        w.includes('403')
      )
      
      // 显示所有警告信息（延迟显示，避免消息重叠）
      res.warnings.forEach((warning, index) => {
        setTimeout(() => {
          // 如果是权限错误，使用error类型显示
          const messageType = (warning.includes('访问被拒绝') || 
                              warning.includes('AccessDenied') || 
                              warning.includes('Access Denied') ||
                              warning.includes('403')) ? 'error' : 'warning'
          ElMessage[messageType]({
            message: warning,
            duration: hasPermissionError ? 10000 : 5000, // 权限错误显示10秒
            showClose: true,
          })
        }, index * 300) // 每个警告消息间隔300ms显示
      })
      // 仍然显示成功消息，因为规则已创建
      setTimeout(() => {
        if (hasPermissionError) {
          ElMessage.warning('重定向规则已创建，但部署失败。请检查AWS权限配置后使用"修复"功能重新部署')
        } else {
          ElMessage.success('重定向规则创建成功，但存在警告，请查看上方提示')
        }
      }, res.warnings.length * 300)
    } else {
    ElMessage.success('重定向规则创建成功')
    }
    
    showCreateDialog.value = false
    resetCreateForm()
    loadRedirects()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    createLoading.value = false
  }
}

const viewDetails = async (row) => {
  try {
    const res = await redirectApi.getRedirectRule(row.id)
    currentRule.value = res
    showDetailDialog.value = true
  } catch (error) {
    ElMessage.error('获取详情失败')
  }
}

const addTarget = (row) => {
  currentRuleId.value = row.id
  targetForm.value.target_url = ''
  showAddTargetDialog.value = true
}

const handleAddTarget = async () => {
  if (!targetForm.value.target_url) {
    ElMessage.warning('请输入目标 URL')
    return
  }

  addTargetLoading.value = true
  try {
    await redirectApi.addTarget(currentRuleId.value, targetForm.value)
    ElMessage.success('目标添加成功')
    showAddTargetDialog.value = false
    loadRedirects()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    addTargetLoading.value = false
  }
}

const removeTarget = async (targetId) => {
  try {
    await ElMessageBox.confirm('确定要删除这个目标吗？', '提示', {
      type: 'warning',
    })
    await redirectApi.removeTarget(targetId)
    ElMessage.success('目标删除成功')
    loadRedirects()
    if (showDetailDialog.value) {
      viewDetails({ id: currentRule.value.id })
    }
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}


const deleteRule = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个重定向规则吗？', '提示', {
      type: 'warning',
    })
    await redirectApi.deleteRule(row.id)
    ElMessage.success('删除成功')
    loadRedirects()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

const checkRule = async (row) => {
  checkRuleId.value = row.id
  checkLoading.value = true
  checkStatus.value = null
  try {
    const res = await redirectApi.checkRedirectRule(row.id)
    checkStatus.value = res
    showCheckDialog.value = true
  } catch (error) {
    ElMessage.error('检查失败')
  } finally {
    checkLoading.value = false
  }
}

const fixRule = async (row) => {
  try {
    await ElMessageBox.confirm('确定要修复这个重定向规则吗？修复将重新部署到 S3 和 CloudFront，并创建/更新 DNS 记录。', '提示', {
      type: 'warning',
    })
    fixLoading.value = true
    
    // 如果还没有检查状态，先检查一下
    if (!checkStatus.value || checkRuleId.value !== row.id) {
      await checkRule(row)
    }
    const res = await redirectApi.fixRedirectRule(row.id)
    
    // 检查是否有警告信息
    if (res.warnings && res.warnings.length > 0) {
      // 检查是否有权限错误
      const hasPermissionError = res.warnings.some(w => 
        w.includes('访问被拒绝') || 
        w.includes('AccessDenied') || 
        w.includes('Access Denied') ||
        w.includes('403')
      )
      
      res.warnings.forEach((warning, index) => {
        setTimeout(() => {
          // 如果是权限错误，使用error类型显示
          const messageType = (warning.includes('访问被拒绝') || 
                              warning.includes('AccessDenied') || 
                              warning.includes('Access Denied') ||
                              warning.includes('403')) ? 'error' : 'warning'
          ElMessage[messageType]({
            message: warning,
            duration: hasPermissionError ? 10000 : 5000, // 权限错误显示10秒
            showClose: true,
          })
        }, index * 300)
      })
      setTimeout(() => {
        if (hasPermissionError) {
          ElMessage.warning('修复失败，请检查AWS权限配置。需要s3:PutObject和s3:GetObject权限')
        } else {
          ElMessage.success('修复完成，但存在警告，请查看上方提示')
        }
      }, res.warnings.length * 300)
    } else {
      ElMessage.success('修复成功')
    }
    
    // 重新检查状态
    if (checkRuleId.value === row.id) {
      const checkRes = await redirectApi.checkRedirectRule(row.id)
      checkStatus.value = checkRes
    }
    
    loadRedirects()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('修复失败')
    }
  } finally {
    fixLoading.value = false
  }
}

const handleFix = async () => {
  if (!checkRuleId.value) return
  await fixRule({ id: checkRuleId.value })
}

const getCloudFrontStatusType = (status) => {
  const statusMap = {
    InProgress: 'warning',
    Deployed: 'success',
    Disabled: 'info',
  }
  return statusMap[status] || 'info'
}

const getCloudFrontStatusText = (status) => {
  const statusTextMap = {
    InProgress: '部署中',
    Deployed: '已部署',
    Disabled: '已禁用',
  }
  return statusTextMap[status] || status || '未知'
}

const getDomainStatusType = (status) => {
  const statusMap = {
    pending: 'info',
    in_progress: 'warning',
    completed: 'success',
    failed: 'danger',
  }
  return statusMap[status] || 'info'
}

const getDomainStatusText = (status) => {
  const statusTextMap = {
    pending: '待转入',
    in_progress: '转入中',
    completed: '已完成',
    failed: '失败',
  }
  return statusTextMap[status] || status || '未知'
}

const getCertificateStatusType = (status) => {
  const statusMap = {
    pending: 'warning',
    'pending_validation': 'warning',
    issued: 'success',
    failed: 'danger',
    'validation_timed_out': 'danger',
    revoked: 'danger',
    expired: 'warning',
    not_requested: 'info',
  }
  return statusMap[status] || 'info'
}

const getCertificateStatusText = (status) => {
  const statusTextMap = {
    pending: '验证中',
    'pending_validation': '待验证',
    issued: '已签发',
    failed: '失败',
    'validation_timed_out': '验证超时',
    revoked: '已撤销',
    expired: '已过期',
    not_requested: '未申请',
  }
  return statusTextMap[status] || status || '未知'
}

const getURLStatusType = (status) => {
  const statusMap = {
    active: 'success',
    unreachable: 'danger',
    error: 'warning',
  }
  return statusMap[status] || 'info'
}

const getURLStatusText = (status) => {
  const statusTextMap = {
    active: '正常',
    unreachable: '不可访问',
    error: '错误',
  }
  return statusTextMap[status] || status || '未知'
}

const getRoute53DNSStatusType = (status) => {
  const statusMap = {
    configured: 'success',
    not_configured: 'warning',
    mismatched: 'danger',
    error: 'danger',
  }
  return statusMap[status] || 'info'
}

const getRoute53DNSStatusText = (status) => {
  const statusTextMap = {
    configured: '已配置',
    not_configured: '未配置',
    mismatched: '指向错误',
    error: '检查失败',
  }
  return statusTextMap[status] || status || '未知'
}
</script>

<style scoped>
.redirect-list {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>


