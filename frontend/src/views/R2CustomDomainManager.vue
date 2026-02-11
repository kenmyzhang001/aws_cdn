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
        <el-table-column label="操作" width="450">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'failed' || row.status === 'pending'"
              size="small"
              type="warning"
              :loading="retryingDomainId === row.id"
              @click="handleRetry(row)"
            >
              重试
            </el-button>
            <el-button size="small" @click="viewCacheRules(row)">
              缓存规则
            </el-button>
            <el-button size="small" type="info" @click="viewConfigLogs(row)">
              查看日志
            </el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加域名对话框 -->
    <el-dialog v-model="showAddDialog" title="添加自定义域名" width="600px" @close="resetAddForm" @open="loadCFAccountDomains">
      <el-form :model="addForm" :rules="formRules" ref="addFormRef" label-width="140px">
        <el-form-item label="域名" prop="domain">
          <div style="display: flex; align-items: flex-start; gap: 10px;">
            <!-- 子域名前缀输入框 -->
            <div style="flex: 0 0 200px;">
              <el-input
                v-model="addForm.domain_prefix"
                placeholder="可选：如 www, api, cdn"
                clearable
                @input="updateDomain"
              >
                <template #append>.</template>
              </el-input>
              <div class="form-tip" style="margin-top: 4px; white-space: nowrap;">
                子域名前缀（可选）
              </div>
            </div>
            
            <!-- 基础域名选择框 -->
            <div style="flex: 1; min-width: 0;">
              <el-select
                v-model="addForm.base_domain"
                placeholder="选择或输入基础域名（必填）"
                style="width: 100%"
                filterable
                allow-create
                clearable
                default-first-option
                :loading="loadingCfDomains"
                :filter-method="filterCfDomains"
                @change="updateDomain"
              >
                <template #empty>
                  <div style="padding: 10px; text-align: center; color: #909399;">
                    <div v-if="cfDomainSearchQuery">
                      未找到匹配的域名
                      <div style="margin-top: 8px;">
                        <el-button size="small" type="primary" @click="useCustomDomain">
                          使用 "{{ cfDomainSearchQuery }}" 作为域名
                        </el-button>
                      </div>
                    </div>
                    <div v-else>
                      暂无可用域名，请输入完整域名
                    </div>
                  </div>
                </template>
                
                <el-option
                  v-for="domain in filteredCfDomains"
                  :key="domain"
                  :label="domain"
                  :value="domain"
                >
                  <div style="display: flex; justify-content: space-between; align-items: center;">
                    <span>{{ domain }}</span>
                    <el-tag size="small" type="success">已托管</el-tag>
                  </div>
                </el-option>
                
                <!-- 加载更多选项 -->
                <el-option
                  v-if="cfDomainsPagination.hasMore && !cfDomainSearchQuery"
                  :value="'__load_more__'"
                  disabled
                  style="background-color: #f5f7fa; cursor: pointer !important;"
                >
                  <div style="text-align: center; padding: 5px 0;">
                    <el-button 
                      type="primary" 
                      size="small"
                      @click.stop="loadMoreCfDomains"
                      :loading="loadingCfDomains"
                      style="width: 90%;"
                    >
                      <span v-if="!loadingCfDomains">
                        加载更多域名 ({{ cfDomains.length }}/{{ cfDomainsPagination.totalCount }})
                      </span>
                      <span v-else>加载中...</span>
                    </el-button>
                  </div>
                </el-option>
              </el-select>
              <div class="form-tip" style="margin-top: 4px;">
                基础域名（必填）
              </div>
            </div>
          </div>
          
          <!-- 完整域名预览 -->
          <div v-if="addForm.domain" style="margin-top: 10px; padding: 10px 14px; background: #f0f9ff; border: 1px solid #91caff; border-radius: 6px;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <el-icon color="#1890ff" :size="16"><Link /></el-icon>
              <span style="color: #1890ff; font-weight: 500;">完整域名:</span>
              <span style="color: #262626; font-family: 'Monaco', 'Menlo', monospace; font-size: 14px; font-weight: 500;">{{ addForm.domain }}</span>
            </div>
          </div>
          
          <div class="form-tip" v-if="loadingCfDomains" style="color: #409EFF;">
            <el-icon class="is-loading"><Loading /></el-icon>
            正在加载域名列表...
          </div>
          <div class="form-tip" v-else-if="cfDomains.length > 0" style="color: #67C23A;">
            已加载 {{ cfDomains.length }}/{{ cfDomainsPagination.totalCount }} 个托管域名
            <span v-if="filteredCfDomains.length < cfDomains.length">
              （搜索结果: {{ filteredCfDomains.length }} 个）
            </span>
            <el-button 
              v-if="cfDomainsPagination.hasMore" 
              type="primary" 
              link 
              size="small"
              @click="loadMoreCfDomains"
              :loading="loadingCfDomains"
              style="margin-left: 8px;"
            >
              加载更多 (第 {{ cfDomainsPagination.page + 1 }}/{{ cfDomainsPagination.totalPages }} 页)
            </el-button>
          </div>
          <div class="form-tip" v-else style="color: #E6A23C;">
            该账号暂无托管域名，请手动输入完整域名
          </div>
          <div class="form-tip">
            域名必须在 Cloudflare 上托管
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

    <!-- 配置日志查看对话框 -->
    <el-dialog v-model="showConfigLogsDialog" title="域名配置日志" width="900px">
      <div v-if="configLogs && configLogs.length > 0" style="max-height: 600px; overflow-y: auto;">
        <el-timeline>
          <el-timeline-item
            v-for="(log, index) in configLogs"
            :key="index"
            :timestamp="log.timestamp"
            :type="getLogType(log.level)"
            placement="top"
          >
            <el-card>
              <div style="display: flex; align-items: center; margin-bottom: 8px;">
                <el-tag :type="getLogType(log.level)" size="small" style="margin-right: 10px;">
                  {{ log.level.toUpperCase() }}
                </el-tag>
                <strong>{{ log.action }}</strong>
              </div>
              <div style="margin-bottom: 5px;">{{ log.message }}</div>
              <div v-if="log.details" style="color: #909399; font-size: 12px; white-space: pre-wrap; background: #f5f5f5; padding: 8px; border-radius: 4px; margin-top: 8px;">
                {{ log.details }}
              </div>
            </el-card>
          </el-timeline-item>
        </el-timeline>
      </div>
      <el-empty v-else description="暂无配置日志" />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { r2Api } from '@/api/r2'
import { cfAccountApi } from '@/api/cf_account'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Loading, Link } from '@element-plus/icons-vue'
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
  domain_prefix: '', // 子域名前缀
  base_domain: '', // 基础域名
  default_file_path: '',
  note: '',
})
const addFormRef = ref(null)

// 文件列表相关
const filesLoading = ref(false)
const fileList = ref([])

const showCacheRuleDialog = ref(false)
const selectedDomain = ref(null)

const showConfigLogsDialog = ref(false)
const configLogs = ref([])
const retryingDomainId = ref(null)

// CF 托管域名列表相关
const cfDomains = ref([])
const loadingCfDomains = ref(false)
const filteredCfDomains = ref([])
const cfDomainSearchQuery = ref('')

// CF 域名分页状态
const cfDomainsPagination = ref({
  page: 1,
  perPage: 50,
  totalPages: 0,
  totalCount: 0,
  hasMore: false
})

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

// 监听 CF 账号变化，重新加载 CF 域名列表
watch(() => props.bucket.cf_account_id, (newAccountId, oldAccountId) => {
  // 当 CF 账号变化时，如果对话框打开，则重新加载域名列表
  if (newAccountId !== oldAccountId && showAddDialog.value) {
    console.log('CF 账号已变化，重新加载域名列表')
    // 清空现有域名列表
    cfDomains.value = []
    filteredCfDomains.value = []
    cfDomainSearchQuery.value = ''
    cfDomainsPagination.value = {
      page: 1,
      perPage: 50,
      totalPages: 0,
      totalCount: 0,
      hasMore: false
    }
    // 重新加载
    loadCFAccountDomains()
  }
})

const loadDomains = async () => {
  loading.value = true
  try {
    const res = await r2Api.getR2CustomDomainList(props.bucket.id)
    domainList.value = res
    
    // 检查是否有 pending 或 processing 状态的域名，如果有则启动轮询
    res.forEach((domain) => {
      if ((domain.status === 'pending' || domain.status === 'processing') && !pollingTimers.value.has(domain.id)) {
        startPollingDomainStatus(domain.id)
      }
    })
  } catch (error) {
    ElMessage.error('加载域名列表失败')
  } finally {
    loading.value = false
  }
}

// 加载 CF 账号的托管域名列表
const loadCFAccountDomains = async (isLoadMore = false) => {
  if (loadingCfDomains.value) return
  
  // 从 bucket 中获取 cf_account_id
  const cfAccountId = props.bucket?.cf_account_id
  if (!cfAccountId) {
    console.warn('存储桶没有关联的 CF 账号 ID')
    return
  }
  
  try {
    loadingCfDomains.value = true
    
    const page = isLoadMore ? cfDomainsPagination.value.page + 1 : 1
    
    console.log('加载 CF 托管域名列表, cfAccountId:', cfAccountId, 'page:', page)
    
    const result = await cfAccountApi.getCFAccountZones(cfAccountId, {
      page: page,
      per_page: cfDomainsPagination.value.perPage
    })
    
    console.log('CF 托管域名列表响应:', result)
    
    // 兼容旧格式（数组）和新格式（带分页信息的对象）
    let zoneList = []
    if (Array.isArray(result)) {
      // 旧格式：直接返回数组
      zoneList = result
      cfDomainsPagination.value.page = 1
      cfDomainsPagination.value.totalPages = 1
      cfDomainsPagination.value.totalCount = result.length
      cfDomainsPagination.value.hasMore = false
    } else {
      // 新格式：带分页信息的对象
      zoneList = result.zones || []
      cfDomainsPagination.value.page = result.page || page
      cfDomainsPagination.value.totalPages = result.total_pages || 0
      cfDomainsPagination.value.totalCount = result.total_count || 0
      cfDomainsPagination.value.hasMore = cfDomainsPagination.value.page < cfDomainsPagination.value.totalPages
    }
    
    // 提取域名名称
    const newDomains = zoneList.map(zone => zone.name || zone)
    
    if (isLoadMore) {
      // 追加到现有列表
      cfDomains.value = [...cfDomains.value, ...newDomains]
    } else {
      // 替换列表
      cfDomains.value = newDomains
    }
    
    // 更新过滤列表
    if (!cfDomainSearchQuery.value) {
      filteredCfDomains.value = [...cfDomains.value]
    } else {
      // 重新应用搜索过滤
      filterCfDomains(cfDomainSearchQuery.value)
    }
    
    if (!isLoadMore && cfDomains.value.length > 0) {
      const moreMsg = cfDomainsPagination.value.hasMore ? `，还有更多域名可加载` : ''
      console.log(`已加载 ${cfDomains.value.length}/${cfDomainsPagination.value.totalCount} 个托管域名${moreMsg}`)
    }
  } catch (error) {
    console.error('加载 CF 托管域名失败:', error)
    if (!isLoadMore) {
      cfDomains.value = []
      filteredCfDomains.value = []
    }
  } finally {
    loadingCfDomains.value = false
  }
}

// 加载更多 CF 域名
const loadMoreCfDomains = async () => {
  if (!cfDomainsPagination.value.hasMore) {
    return
  }
  await loadCFAccountDomains(true)
}

// 过滤 CF 域名
const filterCfDomains = (query) => {
  cfDomainSearchQuery.value = query
  
  if (!query) {
    filteredCfDomains.value = [...cfDomains.value]
    return
  }
  
  const lowerQuery = query.toLowerCase()
  filteredCfDomains.value = cfDomains.value.filter(domain => {
    return domain.toLowerCase().includes(lowerQuery)
  })
  
  console.log('域名搜索:', query, '结果数:', filteredCfDomains.value.length)
}

// 使用自定义域名
const useCustomDomain = () => {
  if (cfDomainSearchQuery.value) {
    addForm.value.base_domain = cfDomainSearchQuery.value
    cfDomainSearchQuery.value = ''
    updateDomain()
  }
}

// 更新完整域名（组合前缀和基础域名）
const updateDomain = () => {
  const prefix = addForm.value.domain_prefix?.trim()
  const baseDomain = addForm.value.base_domain?.trim()
  
  if (!baseDomain) {
    addForm.value.domain = ''
    return
  }
  
  if (prefix) {
    // 有前缀：组合成 prefix.baseDomain
    addForm.value.domain = `${prefix}.${baseDomain}`
  } else {
    // 无前缀：直接使用基础域名
    addForm.value.domain = baseDomain
  }
  
  console.log('更新完整域名:', addForm.value.domain)
}

const resetAddForm = () => {
  addForm.value = {
    domain: '',
    domain_prefix: '',
    base_domain: '',
    default_file_path: '',
    note: '',
  }
  // 清空文件列表
  fileList.value = []
  // 清空 CF 域名列表
  cfDomains.value = []
  filteredCfDomains.value = []
  cfDomainSearchQuery.value = ''
  cfDomainsPagination.value = {
    page: 1,
    perPage: 50,
    totalPages: 0,
    totalCount: 0,
    hasMore: false
  }
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
      const newDomain = await r2Api.addR2CustomDomain(props.bucket.id, addForm.value)
      ElMessage.success('域名正在配置中，请稍候...')
      showAddDialog.value = false
      
      // 立即刷新列表，显示 pending 状态的域名
      loadDomains()
      
      // 开始轮询查询域名状态
      startPollingDomainStatus(newDomain.id)
    } catch (error) {
      // 错误已在拦截器中处理
    } finally {
      addLoading.value = false
    }
  })
}

// 轮询查询域名状态
const pollingTimers = ref(new Map()) // 存储每个域名的轮询定时器

const startPollingDomainStatus = (domainId) => {
  // 如果已经有该域名的轮询定时器，先清除
  if (pollingTimers.value.has(domainId)) {
    clearInterval(pollingTimers.value.get(domainId))
  }

  // 每 3 秒查询一次
  const timer = setInterval(async () => {
    try {
      const domain = await r2Api.getR2CustomDomain(domainId)
      
      // 如果状态变为 active 或 failed，停止轮询
      if (domain.status === 'active') {
        clearInterval(timer)
        pollingTimers.value.delete(domainId)
        ElMessage.success(`域名 ${domain.domain} 配置成功！`)
        loadDomains()
      } else if (domain.status === 'failed') {
        clearInterval(timer)
        pollingTimers.value.delete(domainId)
        ElMessage.error(`域名 ${domain.domain} 配置失败，请查看备注了解详情`)
        loadDomains()
      } else {
        // 状态仍为 pending 或 processing，更新列表
        loadDomains()
      }
    } catch (error) {
      // 如果查询失败，停止轮询
      clearInterval(timer)
      pollingTimers.value.delete(domainId)
      console.error('查询域名状态失败:', error)
    }
  }, 3000)

  pollingTimers.value.set(domainId, timer)
}

// 组件卸载时清除所有轮询定时器
onUnmounted(() => {
  pollingTimers.value.forEach((timer) => {
    clearInterval(timer)
  })
  pollingTimers.value.clear()
})

const handleRetry = async (row) => {
  retryingDomainId.value = row.id
  try {
    await r2Api.retryR2CustomDomain(row.id)
    ElMessage.success('已开始重试配置，请稍候...')
    loadDomains()
    startPollingDomainStatus(row.id)
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    retryingDomainId.value = null
  }
}

const viewCacheRules = (row) => {
  selectedDomain.value = row
  showCacheRuleDialog.value = true
}

const closeCacheRuleDialog = () => {
  selectedDomain.value = null
}

const viewConfigLogs = async (row) => {
  try {
    const domain = await r2Api.getR2CustomDomain(row.id)
    if (domain.config_logs) {
      try {
        configLogs.value = JSON.parse(domain.config_logs)
      } catch (e) {
        console.error('解析配置日志失败:', e)
        configLogs.value = []
        ElMessage.warning('配置日志格式错误')
      }
    } else {
      configLogs.value = []
    }
    showConfigLogsDialog.value = true
  } catch (error) {
    ElMessage.error('获取配置日志失败')
  }
}

const getLogType = (level) => {
  const typeMap = {
    info: 'success',
    warning: 'warning',
    error: 'danger',
  }
  return typeMap[level] || 'info'
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
    processing: 'info',
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

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.4;
}

.is-loading {
  animation: rotating 2s linear infinite;
}

@keyframes rotating {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}
</style>
