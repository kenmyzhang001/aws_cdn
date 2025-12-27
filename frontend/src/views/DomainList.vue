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
        <el-table-column prop="domain_name" label="域名" />
        <el-table-column prop="registrar" label="原注册商" />
        <el-table-column prop="dns_provider" label="DNS提供商" width="120">
          <template #default="{ row }">
            <el-tag :type="row.dns_provider === 'cloudflare' ? 'success' : 'primary'">
              {{ row.dns_provider === 'cloudflare' ? 'Cloudflare' : 'AWS' }}
            </el-tag>
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
    <el-dialog v-model="showTransferDialog" title="新增域名" width="500px">
      <el-form :model="transferForm" label-width="100px">
        <el-form-item label="域名" required>
          <el-input v-model="transferForm.domain_name" placeholder="例如: example.com" />
        </el-form-item>
        <el-form-item label="原注册商" required>
          <el-input v-model="transferForm.registrar" placeholder="例如: GoDaddy aliyun" />
        </el-form-item>
        <el-form-item label="DNS提供商" required>
          <el-select v-model="transferForm.dns_provider" placeholder="请选择DNS提供商" style="width: 100%">
            <el-option label="Cloudflare" value="cloudflare" />
            <el-option label="AWS" value="aws" />
          </el-select>
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
        <el-button @click="showTransferDialog = false">取消</el-button>
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { domainApi } from '@/api/domain'
import { groupApi } from '@/api/group'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const loading = ref(false)
const domainList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)
const totalAll = ref(0)

const activeGroupId = ref(null)
const groups = ref([])

const showTransferDialog = ref(false)
const transferLoading = ref(false)
const transferForm = ref({
  domain_name: '',
  registrar: '',
  dns_provider: 'cloudflare', // 默认选择 Cloudflare
  group_id: null,
})

const showNSDialog = ref(false)
const nsServers = ref([])
const currentDomainId = ref(null)

const showCheckDialog = ref(false)
const checkResult = ref(null)
const currentCheckDomainId = ref(null)
const fixing = ref(false)

onMounted(() => {
  loadGroups()
  loadDomains()
})

const loadGroups = async () => {
  try {
    const res = await groupApi.getGroupList()
    groups.value = res
    // 加载每个分组的域名数量
    for (const group of groups.value) {
      const res = await domainApi.getDomainList({
        page: 1,
        page_size: 1,
        group_id: group.id,
      })
      group.count = res.total
    }
    // 加载全部数量
    const resAll = await domainApi.getDomainList({
      page: 1,
      page_size: 1,
    })
    totalAll.value = resAll.total
  } catch (error) {
    console.error('加载分组列表失败:', error)
  }
}

const handleGroupChange = () => {
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
    if (activeGroupId.value !== null) {
      params.group_id = activeGroupId.value
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

const handleTransfer = async () => {
  if (!transferForm.value.domain_name || !transferForm.value.registrar) {
    ElMessage.warning('请填写完整信息')
    return
  }

  transferLoading.value = true
  try {
    const data = { ...transferForm.value }
    if (!data.group_id) {
      delete data.group_id
    }
    await domainApi.transferDomain(data)
    ElMessage.success('域名转入请求已提交')
    showTransferDialog.value = false
    transferForm.value = { domain_name: '', registrar: '', dns_provider: 'cloudflare', group_id: null }
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


