<template>
  <div class="cloudfront-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>CloudFront 管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建分发
          </el-button>
        </div>
      </template>

      <el-table :data="distributionList" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="200" />
        <el-table-column prop="domain_name" label="域名" />
        <el-table-column label="别名" min-width="200">
          <template #default="{ row }">
            <el-tag
              v-for="(alias, index) in row.aliases"
              :key="index"
              style="margin-right: 5px; margin-bottom: 5px"
            >
              {{ alias }}
            </el-tag>
            <span v-if="!row.aliases || row.aliases.length === 0" style="color: #c0c4cc">
              无
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="enabled" label="启用状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">
              {{ row.enabled ? '已启用' : '已禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button size="small" @click="viewDetails(row)">详情</el-button>
            <el-button size="small" type="warning" @click="editDistribution(row)">
              编辑
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="deleteDistribution(row)"
              :disabled="row.status === 'InProgress'"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 创建分发对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建 CloudFront 分发" width="600px" @close="resetCreateForm">
      <el-form :model="createForm" label-width="120px">
        <el-form-item label="域名" required>
          <el-input v-model="createForm.domain_name" placeholder="例如: example.com" />
        </el-form-item>
        <el-form-item label="证书 ARN" required>
          <el-input
            v-model="createForm.certificate_arn"
            placeholder="例如: arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"
          />
          <div style="color: #909399; font-size: 12px; margin-top: 5px">
            必须是已通过验证的 ACM 证书 ARN
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
    <el-dialog v-model="showDetailDialog" title="CloudFront 分发详情" width="700px">
      <el-descriptions :column="1" border v-if="currentDistribution">
        <el-descriptions-item label="ID">
          {{ currentDistribution.id }}
        </el-descriptions-item>
        <el-descriptions-item label="域名">
          {{ currentDistribution.domain_name }}
        </el-descriptions-item>
        <el-descriptions-item label="别名">
          <el-tag
            v-for="(alias, index) in currentDistribution.aliases"
            :key="index"
            style="margin-right: 5px"
          >
            {{ alias }}
          </el-tag>
          <span v-if="!currentDistribution.aliases || currentDistribution.aliases.length === 0">
            无
          </span>
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(currentDistribution.status)">
            {{ currentDistribution.status }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="启用状态">
          <el-tag :type="currentDistribution.enabled ? 'success' : 'info'">
            {{ currentDistribution.enabled ? '已启用' : '已禁用' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="证书 ARN">
          {{ currentDistribution.certificate_arn || '未配置' }}
        </el-descriptions-item>
        <el-descriptions-item label="备注">
          {{ currentDistribution.comment || '无' }}
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <!-- 编辑分发对话框 -->
    <el-dialog v-model="showEditDialog" title="编辑 CloudFront 分发" width="600px">
      <el-form :model="editForm" label-width="120px">
        <el-form-item label="别名">
          <el-input
            type="textarea"
            v-model="aliasesInput"
            :rows="3"
            placeholder="每行一个别名，例如：&#10;example.com&#10;www.example.com"
            @blur="parseAliases"
          />
          <div style="margin-top: 10px">
            <el-tag
              v-for="(alias, index) in editForm.aliases"
              :key="index"
              closable
              @close="removeAlias(index)"
              style="margin-right: 5px; margin-bottom: 5px"
            >
              {{ alias }}
            </el-tag>
          </div>
        </el-form-item>
        <el-form-item label="证书 ARN">
          <el-input
            v-model="editForm.certificate_arn"
            placeholder="留空则不更新证书"
          />
        </el-form-item>
        <el-form-item label="启用状态">
          <el-switch v-model="editForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="handleUpdate" :loading="updateLoading">
          更新
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { cloudfrontApi } from '@/api/cloudfront'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'

const loading = ref(false)
const distributionList = ref([])

const showCreateDialog = ref(false)
const createLoading = ref(false)
const createForm = ref({
  domain_name: '',
  certificate_arn: '',
})

const showDetailDialog = ref(false)
const currentDistribution = ref(null)

const showEditDialog = ref(false)
const updateLoading = ref(false)
const editForm = ref({
  aliases: [],
  certificate_arn: '',
  enabled: true,
})
const aliasesInput = ref('')
const currentDistributionId = ref(null)

onMounted(() => {
  loadDistributions()
})

const loadDistributions = async () => {
  loading.value = true
  try {
    const res = await cloudfrontApi.getDistributionList()
    distributionList.value = res.data || []
  } catch (error) {
    ElMessage.error('加载 CloudFront 分发列表失败')
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.value = {
    domain_name: '',
    certificate_arn: '',
  }
}

const handleCreate = async () => {
  if (!createForm.value.domain_name || !createForm.value.certificate_arn) {
    ElMessage.warning('请填写完整信息')
    return
  }

  createLoading.value = true
  try {
    await cloudfrontApi.createDistribution(createForm.value)
    ElMessage.success('CloudFront 分发创建成功')
    showCreateDialog.value = false
    resetCreateForm()
    loadDistributions()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    createLoading.value = false
  }
}

const viewDetails = async (row) => {
  try {
    const res = await cloudfrontApi.getDistribution(row.id)
    currentDistribution.value = res
    showDetailDialog.value = true
  } catch (error) {
    ElMessage.error('获取详情失败')
  }
}

const editDistribution = async (row) => {
  currentDistributionId.value = row.id
  try {
    const res = await cloudfrontApi.getDistribution(row.id)
    editForm.value = {
      aliases: res.aliases || [],
      certificate_arn: res.certificate_arn || '',
      enabled: res.enabled !== false,
    }
    aliasesInput.value = (res.aliases || []).join('\n')
    showEditDialog.value = true
  } catch (error) {
    ElMessage.error('获取分发信息失败')
  }
}

const parseAliases = () => {
  if (!aliasesInput.value.trim()) {
    return
  }

  const aliases = aliasesInput.value
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)

  editForm.value.aliases = [...new Set(aliases)] // 去重
  aliasesInput.value = editForm.value.aliases.join('\n')
}

const removeAlias = (index) => {
  editForm.value.aliases.splice(index, 1)
  aliasesInput.value = editForm.value.aliases.join('\n')
}

const handleUpdate = async () => {
  updateLoading.value = true
  try {
    // 解析别名
    if (aliasesInput.value.trim()) {
      parseAliases()
    }

    const updateData = {
      aliases: editForm.value.aliases,
      enabled: editForm.value.enabled,
    }

    // 如果证书ARN有值，才更新
    if (editForm.value.certificate_arn) {
      updateData.certificate_arn = editForm.value.certificate_arn
    }

    await cloudfrontApi.updateDistribution(currentDistributionId.value, updateData)
    ElMessage.success('更新成功')
    showEditDialog.value = false
    loadDistributions()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    updateLoading.value = false
  }
}

const deleteDistribution = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 CloudFront 分发 "${row.id}" 吗？此操作不可恢复。`,
      '提示',
      {
        type: 'warning',
      }
    )
    await cloudfrontApi.deleteDistribution(row.id)
    ElMessage.success('删除成功')
    loadDistributions()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

const getStatusType = (status) => {
  const statusMap = {
    InProgress: 'warning',
    Deployed: 'success',
    Disabled: 'info',
  }
  return statusMap[status] || 'info'
}
</script>

<style scoped>
.cloudfront-list {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

