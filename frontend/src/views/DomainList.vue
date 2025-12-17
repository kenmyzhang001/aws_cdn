<template>
  <div class="domain-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>域名管理</span>
          <el-button type="primary" @click="showTransferDialog = true">
            <el-icon><Plus /></el-icon>
            转入域名
          </el-button>
        </div>
      </template>

      <el-table :data="domainList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="domain_name" label="域名" />
        <el-table-column prop="registrar" label="原注册商" />
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
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button size="small" @click="viewNServers(row)">查看 NS</el-button>
            <el-button
              size="small"
              type="success"
              @click="generateCert(row)"
              :disabled="row.certificate_status === 'issued'"
            >
              生成证书
            </el-button>
            <el-button size="small" @click="refreshStatus(row)">刷新状态</el-button>
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

    <!-- 转入域名对话框 -->
    <el-dialog v-model="showTransferDialog" title="转入域名" width="500px">
      <el-form :model="transferForm" label-width="100px">
        <el-form-item label="域名" required>
          <el-input v-model="transferForm.domain_name" placeholder="例如: example.com" />
        </el-form-item>
        <el-form-item label="原注册商" required>
          <el-input v-model="transferForm.registrar" placeholder="例如: GoDaddy" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showTransferDialog = false">取消</el-button>
        <el-button type="primary" @click="handleTransfer" :loading="transferLoading">
          确认转入
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { domainApi } from '@/api/domain'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const loading = ref(false)
const domainList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)

const showTransferDialog = ref(false)
const transferLoading = ref(false)
const transferForm = ref({
  domain_name: '',
  registrar: '',
})

const showNSDialog = ref(false)
const nsServers = ref([])
const currentDomainId = ref(null)

onMounted(() => {
  loadDomains()
})

const loadDomains = async () => {
  loading.value = true
  try {
    const res = await domainApi.getDomainList({
      page: currentPage.value,
      page_size: pageSize.value,
    })
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
    await domainApi.transferDomain(transferForm.value)
    ElMessage.success('域名转入请求已提交')
    showTransferDialog.value = false
    transferForm.value = { domain_name: '', registrar: '' }
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
    issued: 'success',
    failed: 'danger',
    not_requested: 'info',
  }
  return map[status] || 'info'
}

const getCertificateStatusText = (status) => {
  const map = {
    pending: '验证中',
    issued: '已签发',
    failed: '失败',
    not_requested: '未申请',
  }
  return map[status] || status
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

