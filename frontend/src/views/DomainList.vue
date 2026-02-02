<template>
  <div class="domain-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>域名管理</span>
          <el-button type="primary" @click="showTransferDialog = true">
            <el-icon><Plus /></el-icon>
            新增域名
          </el-button>
        </div>
      </template>

      <!-- 搜索框和筛选条件 -->
      <div style="margin-bottom: 20px; display: flex; gap: 15px; align-items: center;">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索域名..."
          clearable
          style="width: 300px"
          @input="handleSearch"
          @clear="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        <el-select
          v-model="usageFilter"
          placeholder="使用状态"
          clearable
          style="width: 150px"
          @change="handleUsageFilterChange"
        >
          <el-option label="全部" :value="null" />
          <el-option label="已使用" value="used" />
          <el-option label="未使用" value="unused" />
        </el-select>
      </div>

      <!-- 分组Tab -->
      <el-tabs v-model="activeGroupId" @tab-change="handleGroupChange" style="margin-bottom: 20px">
        <el-tab-pane :label="`全部 (${totalAll})`" :name="null"></el-tab-pane>
        <el-tab-pane
          v-for="group in groups"
          :key="group.id"
          :label="`${group.name} (${group.count || 0})`"
          :name="group.id"
        ></el-tab-pane>
      </el-tabs>

      <el-table :data="domainList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="domain_name" label="域名" width="200" />
        <el-table-column prop="dns_provider" label="DNS提供商" width="120">
          <template #default="{ row }">
            <el-tag :type="row.dns_provider === 'cloudflare' ? 'success' : 'primary'">
              {{ row.dns_provider === 'cloudflare' ? 'Cloudflare' : 'AWS' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="CF 账号" width="180">
          <template #default="{ row }">
            <span v-if="row.dns_provider === 'cloudflare' && row.cf_account">
              {{ row.cf_account.email }}
            </span>
            <span v-else-if="row.dns_provider === 'cloudflare' && !row.cf_account" style="color: #f56c6c;">
              默认
            </span>
            <span v-else style="color: #c0c4cc; font-size: 12px;">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="certificate_status" label="证书状态">
          <template #default="{ row }">
            <el-tag :type="getCertificateStatusType(row.certificate_status)">
              {{ getCertificateStatusText(row.certificate_status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="备注" width="200">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px;">
              <span v-if="row.note" style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" :title="row.note">
                {{ row.note }}
              </span>
              <span v-else style="color: #c0c4cc; font-size: 12px;">-</span>
              <el-button
                size="small"
                type="text"
                @click="editNote(row)"
                style="padding: 0; min-height: auto;"
              >
                <el-icon><Edit /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="使用状态" width="200">
          <template #default="{ row }">
            <div style="display: flex; flex-direction: column; gap: 5px;">
              <el-tag
                v-if="row.used_by_redirect"
                size="small"
                type="warning"
              >
                重定向使用中
              </el-tag>
              <el-tag
                v-if="row.used_by_download_package"
                size="small"
                type="success"
              >
                下载包使用中
              </el-tag>
              <span v-if="!row.used_by_redirect && !row.used_by_download_package" style="color: #c0c4cc; font-size: 12px;">
                未使用
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="450">
          <template #default="{ row }">
            <el-button 
              v-if="row.dns_provider !== 'cloudflare'"
              size="small" 
              @click="viewNServers(row)"
            >
              查看 NS
            </el-button>
            <el-button
              size="small"
              type="success"
              @click="generateCert(row)"
              :disabled="row.certificate_status === 'issued'"
            >
              生成证书
            </el-button>
            <el-button
              v-if="row.dns_provider !== 'cloudflare'"
              size="small"
              type="warning"
              @click="checkCertificate(row)"
              :loading="row.checking"
            >
              检查
            </el-button>
            <el-button size="small" @click="refreshStatus(row)">刷新状态</el-button>
            <el-button
              size="small"
              type="info"
              @click="openMoveGroupDialog(row)"
            >
              移动分组
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
        @size-change="loadDomains"
        @current-change="loadDomains"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 新增域名对话框 -->
    <el-dialog v-model="showTransferDialog" title="新增域名" width="600px">
      <el-form :model="transferForm" label-width="120px">
        <el-form-item label="DNS提供商" required>
          <el-select v-model="transferForm.dns_provider" disabled style="width: 100%">
            <el-option label="Cloudflare" value="cloudflare" />
          </el-select>
        </el-form-item>
        <el-form-item label="CF 账号" required>
          <el-select 
            v-model="transferForm.cf_account_id" 
            placeholder="请选择 CF 账号" 
            style="width: 100%"
            @change="handleCFAccountChange"
          >
            <el-option
              v-for="account in cfAccountList"
              :key="account.id"
              :label="account.email"
              :value="account.id"
            />
          </el-select>
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            选择 CF 账号用于域名管理操作
          </div>
        </el-form-item>
        <el-form-item label="根域名" required>
          <el-select 
            v-model="transferForm.zone_id" 
            placeholder="请先选择 CF 账号，然后输入域名搜索"
            :disabled="!transferForm.cf_account_id"
            :loading="loadingZones"
            filterable
            remote
            :remote-method="searchZones"
            style="width: 100%"
            @change="handleZoneChange"
          >
            <el-option
              v-for="zone in zoneList"
              :key="zone.id"
              :label="zone.name"
              :value="zone.id"
            >
              <span>{{ zone.name }}</span>
              <span style="float: right; color: #8492a6; font-size: 12px">{{ zone.status }}</span>
            </el-option>
          </el-select>
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            从 CF 账号下选择一个根域名（支持输入域名进行搜索）
          </div>
        </el-form-item>
        <el-form-item label="子域名前缀">
          <el-input 
            v-model="transferForm.subdomain_prefix" 
            placeholder="留空则使用根域名，例如：dl、cdn" 
            :disabled="!transferForm.zone_id"
          >
            <template #append v-if="selectedZoneName">
              .{{ selectedZoneName }}
            </template>
          </el-input>
          <div style="font-size: 12px; color: #909399; margin-top: 5px">
            留空则使用根域名，填写则创建子域名（如 dl.example.com）
          </div>
        </el-form-item>
        <el-form-item label="最终域名">
          <el-input :value="finalDomainName" disabled />
        </el-form-item>
        <el-form-item label="分组">
          <el-select v-model="transferForm.group_id" placeholder="请选择分组（不选则使用默认分组）" clearable style="width: 100%">
            <el-option
              v-for="group in groups"
              :key="group.id"
              :label="group.name"
              :value="group.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="closeTransferDialog">取消</el-button>
        <el-button type="primary" @click="handleTransfer" :loading="transferLoading">
          确认新增
        </el-button>
      </template>
    </el-dialog>

    <!-- NS 服务器对话框 -->
    <el-dialog v-model="showNSDialog" title="NS 服务器配置" width="600px">
      <el-alert
        type="info"
        :closable="false"
        style="margin-bottom: 20px"
      >
        请将以下 NS 服务器配置到您的域名注册商处
      </el-alert>
      <el-table :data="nsServers" border>
        <el-table-column label="NS 服务器" prop="server" />
      </el-table>
      <template #footer>
        <el-button type="primary" @click="copyNServers">复制</el-button>
        <el-button @click="showNSDialog = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 证书检查结果对话框 -->
    <el-dialog v-model="showCheckDialog" title="证书检查结果" width="700px">
      <div v-if="checkResult">
        <el-alert
          :type="checkResult.has_issues ? 'warning' : 'success'"
          :closable="false"
          style="margin-bottom: 20px"
        >
          <template #title>
            <span>{{ checkResult.has_issues ? '发现问题' : '检查通过' }}</span>
          </template>
        </el-alert>

        <el-descriptions :column="1" border>
          <el-descriptions-item label="证书状态">
            <el-tag :type="getCertificateStatusType(checkResult.certificate_status)">
              {{ getCertificateStatusText(checkResult.certificate_status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="证书是否存在">
            {{ checkResult.certificate_exists ? '是' : '否' }}
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="checkResult.validation_records && checkResult.validation_records.length > 0" style="margin-top: 20px">
          <h4>验证记录：</h4>
          <el-table :data="checkResult.validation_records.map(r => ({ record: r }))" border style="margin-top: 10px">
            <el-table-column label="CNAME 记录" prop="record" />
          </el-table>
        </div>

        <div v-if="checkResult.missing_cname_records && checkResult.missing_cname_records.length > 0" style="margin-top: 20px">
          <h4 style="color: #e6a23c">缺失的 CNAME 记录：</h4>
          <el-table :data="checkResult.missing_cname_records.map(r => ({ record: r }))" border style="margin-top: 10px">
            <el-table-column label="CNAME 记录" prop="record" />
          </el-table>
        </div>

        <div v-if="checkResult.incorrect_cname_records && checkResult.incorrect_cname_records.length > 0" style="margin-top: 20px">
          <h4 style="color: #f56c6c">值不正确的 CNAME 记录：</h4>
          <el-table :data="checkResult.incorrect_cname_records.map(r => ({ record: r }))" border style="margin-top: 10px">
            <el-table-column label="CNAME 记录" prop="record" />
          </el-table>
        </div>

        <div v-if="checkResult.issues && checkResult.issues.length > 0" style="margin-top: 20px">
          <h4 style="color: #f56c6c">问题列表：</h4>
          <ul style="margin-top: 10px; padding-left: 20px">
            <li v-for="(issue, index) in checkResult.issues" :key="index" style="margin-bottom: 5px">
              {{ issue }}
            </li>
          </ul>
        </div>
      </div>
      <template #footer>
        <el-button
          v-if="checkResult && checkResult.has_issues"
          type="primary"
          @click="fixCertificate"
          :loading="fixing"
        >
          修复
        </el-button>
        <el-button @click="showCheckDialog = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 编辑备注对话框 -->
    <el-dialog v-model="showNoteDialog" title="编辑备注" width="500px">
      <el-form :model="noteForm" label-width="80px">
        <el-form-item label="备注">
          <el-input
            v-model="noteForm.note"
            type="textarea"
            :rows="4"
            placeholder="请输入备注信息"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showNoteDialog = false">取消</el-button>
        <el-button type="primary" @click="saveNote" :loading="noteLoading">
          保存
        </el-button>
      </template>
    </el-dialog>

    <!-- 移动分组对话框 -->
    <el-dialog v-model="showMoveGroupDialog" title="移动分组" width="500px">
      <el-form :model="moveGroupForm" label-width="100px">
        <el-form-item label="域名">
          <el-input v-model="moveGroupForm.domain_name" disabled />
        </el-form-item>
        <el-form-item label="目标分组">
          <el-select v-model="moveGroupForm.group_id" placeholder="请选择分组（不选则使用默认分组）" clearable style="width: 100%">
            <el-option
              v-for="group in groups"
              :key="group.id"
              :label="group.name"
              :value="group.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showMoveGroupDialog = false">取消</el-button>
        <el-button type="primary" @click="handleMoveGroup" :loading="moveGroupLoading">
          确认移动
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { domainApi } from '@/api/domain'
import { groupApi } from '@/api/group'
import { cfAccountApi } from '@/api/cf_account'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, Edit } from '@element-plus/icons-vue'

const loading = ref(false)
const domainList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)
const totalAll = ref(0)

const activeGroupId = ref(null)
const groups = ref([])
const searchKeyword = ref('')
const usageFilter = ref(null) // null: 全部, 'used': 已使用, 'unused': 未使用
let searchTimer = null

const showTransferDialog = ref(false)
const transferLoading = ref(false)
const cfAccountList = ref([])
const zoneList = ref([])
const loadingZones = ref(false)
const transferForm = ref({
  dns_provider: 'cloudflare', // 默认选择 Cloudflare
  cf_account_id: null,
  zone_id: null,
  subdomain_prefix: '',
  group_id: null,
})

// 选中的 Zone 名称
const selectedZoneName = computed(() => {
  if (!transferForm.value.zone_id) return ''
  const zone = zoneList.value.find(z => z.id === transferForm.value.zone_id)
  return zone ? zone.name : ''
})

// 最终域名
const finalDomainName = computed(() => {
  if (!selectedZoneName.value) return ''
  if (!transferForm.value.subdomain_prefix || !transferForm.value.subdomain_prefix.trim()) {
    return selectedZoneName.value
  }
  return `${transferForm.value.subdomain_prefix.trim()}.${selectedZoneName.value}`
})

const showNSDialog = ref(false)
const nsServers = ref([])
const currentDomainId = ref(null)

const showCheckDialog = ref(false)
const checkResult = ref(null)
const currentCheckDomainId = ref(null)
const fixing = ref(false)

const showNoteDialog = ref(false)
const noteForm = ref({
  id: null,
  note: '',
})
const noteLoading = ref(false)

const showMoveGroupDialog = ref(false)
const moveGroupForm = ref({
  id: null,
  domain_name: '',
  group_id: null,
})
const moveGroupLoading = ref(false)

onMounted(() => {
  loadGroups()
  loadDomains()
  loadCFAccounts()
})

// 加载 CF 账号列表
const loadCFAccounts = async () => {
  try {
    const res = await cfAccountApi.getCFAccountList()
    cfAccountList.value = res || []
  } catch (error) {
    console.error('加载 CF 账号列表失败', error)
  }
}

// 当 CF 账号改变时，加载该账号下的 Zones
const handleCFAccountChange = async (accountId) => {
  if (!accountId) {
    zoneList.value = []
    transferForm.value.zone_id = null
    transferForm.value.subdomain_prefix = ''
    return
  }
  
  loadingZones.value = true
  try {
    const res = await cfAccountApi.getCFAccountZones(accountId, { per_page: 50 })
    console.log('API 返回数据:', res)
    console.log('zones 数据:', res.zones)
    zoneList.value = res.zones || []
    console.log('zoneList.value:', zoneList.value)
    // 清空之前选择的 Zone
    transferForm.value.zone_id = null
    transferForm.value.subdomain_prefix = ''
    
    if (zoneList.value.length === 0) {
      ElMessage.warning('该 CF 账号下没有域名，请先在 Cloudflare 中添加域名')
    } else {
      ElMessage.success(`成功加载 ${zoneList.value.length} 个域名，支持搜索筛选`)
    }
  } catch (error) {
    console.error('加载域名列表失败:', error)
    ElMessage.error('加载域名列表失败: ' + (error.message || '未知错误'))
    zoneList.value = []
  } finally {
    loadingZones.value = false
  }
}

// 搜索域名（远程搜索）
const searchZones = async (query) => {
  if (!transferForm.value.cf_account_id) {
    return
  }
  
  loadingZones.value = true
  try {
    const params = { per_page: 50 }
    // 如果有搜索关键字，添加 name 参数
    if (query && query.trim()) {
      params.name = query.trim()
    }
    
    const res = await cfAccountApi.getCFAccountZones(transferForm.value.cf_account_id, params)
    zoneList.value = res.zones || []
  } catch (error) {
    console.error('搜索域名失败:', error)
    // 搜索失败时不显示错误提示，保持用户体验
    zoneList.value = []
  } finally {
    loadingZones.value = false
  }
}

// 当 Zone 改变时，清空子域名前缀
const handleZoneChange = () => {
  transferForm.value.subdomain_prefix = ''
}

// 关闭新增域名对话框
const closeTransferDialog = () => {
  showTransferDialog.value = false
  transferForm.value = {
    dns_provider: 'cloudflare',
    cf_account_id: null,
    zone_id: null,
    subdomain_prefix: '',
    group_id: null,
  }
  zoneList.value = []
}

const loadGroups = async () => {
  try {
    // 使用优化接口，一次性获取分组列表和统计信息
    const res = await groupApi.getGroupListWithStats()
    groups.value = res
    // 设置每个分组的域名数量
    for (const group of groups.value) {
      group.count = group.domain_count || 0
    }
    // 计算全部数量
    totalAll.value = groups.value.reduce((sum, group) => sum + (group.domain_count || 0), 0)
  } catch (error) {
    console.error('加载分组列表失败:', error)
    // 降级到普通接口
    try {
      const res = await groupApi.getGroupList()
      groups.value = res
      for (const group of groups.value) {
        group.count = 0
      }
      totalAll.value = 0
    } catch (fallbackError) {
      console.error('加载分组列表失败（降级方案）:', fallbackError)
    }
  }
}

const handleGroupChange = (value) => {
  // 确保当切换到"全部"时，activeGroupId 为 null
  // el-tabs 可能会将 null 转换为字符串 "null"，需要处理这种情况
  if (value === null || value === 'null' || value === undefined || value === '') {
    activeGroupId.value = null
  } else {
    activeGroupId.value = value
  }
  currentPage.value = 1
  loadDomains()
}

const loadDomains = async () => {
  loading.value = true
  try {
    const params = {
      page: currentPage.value,
      page_size: pageSize.value,
    }
    // 确保只有当 activeGroupId 是有效的数字时才添加 group_id 参数
    if (activeGroupId.value !== null && activeGroupId.value !== undefined && activeGroupId.value !== 'null' && activeGroupId.value !== '') {
      params.group_id = activeGroupId.value
    }
    if (searchKeyword.value && searchKeyword.value.trim()) {
      params.search = searchKeyword.value.trim()
    }
    if (usageFilter.value) {
      params.used_status = usageFilter.value
    }
    const res = await domainApi.getDomainList(params)
    domainList.value = res.data
    total.value = res.total
  } catch (error) {
    ElMessage.error('加载域名列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  // 清除之前的定时器
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  // 设置新的定时器，300ms后执行搜索
  searchTimer = setTimeout(() => {
    currentPage.value = 1 // 搜索时重置到第一页
    loadDomains()
  }, 300)
}

const handleUsageFilterChange = () => {
  currentPage.value = 1 // 筛选时重置到第一页
  loadDomains()
}

const handleTransfer = async () => {
  // 验证必填字段
  if (!transferForm.value.cf_account_id) {
    ElMessage.warning('请选择 CF 账号')
    return
  }
  if (!transferForm.value.zone_id) {
    ElMessage.warning('请选择根域名')
    return
  }

  transferLoading.value = true
  try {
    const data = {
      domain_name: finalDomainName.value,
      dns_provider: transferForm.value.dns_provider,
      cf_account_id: transferForm.value.cf_account_id,
    }
    
    // 添加可选字段
    if (transferForm.value.group_id) {
      data.group_id = transferForm.value.group_id
    }
    
    await domainApi.transferDomain(data)
    ElMessage.success('域名转入请求已提交')
    closeTransferDialog()
    loadGroups()
    loadDomains()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    transferLoading.value = false
  }
}

const viewNServers = async (row) => {
  currentDomainId.value = row.id
  try {
    const res = await domainApi.getNServers(row.id)
    nsServers.value = res.n_servers.map(server => ({ server }))
    showNSDialog.value = true
  } catch (error) {
    ElMessage.error('获取 NS 服务器失败')
  }
}

const copyNServers = () => {
  const text = nsServers.value.map(item => item.server).join('\n')
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success('已复制到剪贴板')
  })
}

const generateCert = async (row) => {
  try {
    await domainApi.generateCertificate(row.id)
    ElMessage.success('证书生成请求已提交，请稍后查看状态')
    
    // 自动调用修复接口
    try {
      await domainApi.fixCertificate(row.id)
      ElMessage.success('证书修复请求已提交')
    } catch (fixError) {
      // 修复接口的错误已在拦截器中处理
      console.error('修复证书失败:', fixError)
    }
    
    setTimeout(() => {
      loadDomains()
    }, 2000)
  } catch (error) {
    // 错误已在拦截器中处理
  }
}

const refreshStatus = async (row) => {
  try {
    const [statusRes, certRes] = await Promise.all([
      domainApi.getDomainStatus(row.id),
      domainApi.getCertificateStatus(row.id),
    ])
    row.status = statusRes.status
    row.certificate_status = certRes.certificate_status
    ElMessage.success('状态已刷新')
  } catch (error) {
    ElMessage.error('刷新状态失败')
  }
}

const handleDelete = (row) => {
  const isCloudflare = row.dns_provider === 'cloudflare'
  const message = isCloudflare
    ? `确定要删除域名 "${row.domain_name}" 吗？此操作将删除数据库中的域名记录，且无法恢复。`
    : `确定要删除域名 "${row.domain_name}" 吗？此操作将同时删除相关的 AWS 资源（Route53 Hosted Zone 和 ACM 证书），且无法恢复。`
  
  ElMessageBox.confirm(
    message,
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }
  )
    .then(async () => {
      try {
        await domainApi.deleteDomain(row.id)
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
  const map = {
    pending: 'info',
    in_progress: 'warning',
    completed: 'success',
    failed: 'danger',
  }
  return map[status] || 'info'
}

const getStatusText = (status) => {
  const map = {
    pending: '待转入',
    in_progress: '转入中',
    completed: '已完成',
    failed: '失败',
  }
  return map[status] || status
}

const getCertificateStatusType = (status) => {
  const map = {
    pending: 'warning',
    'pending_validation': 'warning',
    issued: 'success',
    failed: 'danger',
    'validation_timed_out': 'danger',
    revoked: 'danger',
    expired: 'warning',
    not_requested: 'info',
  }
  return map[status] || 'info'
}

const getCertificateStatusText = (status) => {
  const map = {
    pending: '验证中',
    'pending_validation': '待验证',
    issued: '已签发',
    failed: '失败',
    'validation_timed_out': '验证超时',
    revoked: '已撤销',
    expired: '已过期',
    not_requested: '未申请',
  }
  return map[status] || status || '未知'
}

const checkCertificate = async (row) => {
  // 设置检查状态
  row.checking = true
  currentCheckDomainId.value = row.id

  try {
    const res = await domainApi.checkCertificate(row.id)
    checkResult.value = res
    showCheckDialog.value = true
  } catch (error) {
    ElMessage.error('检查证书配置失败')
  } finally {
    row.checking = false
  }
}

const fixCertificate = async () => {
  if (!currentCheckDomainId.value) {
    return
  }

  fixing.value = true
  try {
    await domainApi.fixCertificate(currentCheckDomainId.value)
    ElMessage.success('证书修复请求已提交，请稍后查看状态')
    showCheckDialog.value = false
    // 刷新列表
    setTimeout(() => {
      loadDomains()
    }, 2000)
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    fixing.value = false
  }
}

const editNote = (row) => {
  noteForm.value = {
    id: row.id,
    note: row.note || '',
  }
  showNoteDialog.value = true
}

const saveNote = async () => {
  noteLoading.value = true
  try {
    await domainApi.updateDomainNote(noteForm.value.id, noteForm.value.note)
    ElMessage.success('备注更新成功')
    showNoteDialog.value = false
    loadDomains()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    noteLoading.value = false
  }
}

const openMoveGroupDialog = (row) => {
  moveGroupForm.value = {
    id: row.id,
    domain_name: row.domain_name,
    group_id: row.group_id,
  }
  showMoveGroupDialog.value = true
}

const handleMoveGroup = async () => {
  moveGroupLoading.value = true
  try {
    await domainApi.moveDomainToGroup(moveGroupForm.value.id, moveGroupForm.value.group_id)
    ElMessage.success('域名移动分组成功')
    showMoveGroupDialog.value = false
    loadGroups()
    loadDomains()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    moveGroupLoading.value = false
  }
}
</script>

<style scoped>
.domain-list {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>


