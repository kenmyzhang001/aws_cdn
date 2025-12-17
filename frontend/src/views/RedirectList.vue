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
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="source_domain" label="源域名" />
        <el-table-column label="目标 URL" min-width="200">
          <template #default="{ row }">
            <el-tag
              v-for="target in row.targets"
              :key="target.id"
              style="margin-right: 5px; margin-bottom: 5px"
            >
              {{ target.target_url }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="cloudfront_id" label="CloudFront ID" />
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button size="small" @click="viewDetails(row)">详情</el-button>
            <el-button size="small" type="success" @click="addTarget(row)">添加目标</el-button>
            <el-button size="small" type="warning" @click="bindCloudFront(row)">绑定 CloudFront</el-button>
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
    <el-dialog v-model="showCreateDialog" title="创建重定向规则" width="600px">
      <el-form :model="createForm" label-width="100px">
        <el-form-item label="源域名" required>
          <el-input v-model="createForm.source_domain" placeholder="例如: example.com" />
        </el-form-item>
        <el-form-item label="目标 URL" required>
          <el-input
            v-model="targetUrlInput"
            placeholder="输入目标 URL，按回车添加"
            @keyup.enter="addTargetUrl"
          />
          <div style="margin-top: 10px">
            <el-tag
              v-for="(url, index) in createForm.target_urls"
              :key="index"
              closable
              @close="removeTargetUrl(index)"
              style="margin-right: 5px; margin-top: 5px"
            >
              {{ url }}
            </el-tag>
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

    <!-- 绑定 CloudFront 对话框 -->
    <el-dialog v-model="showBindDialog" title="绑定 CloudFront" width="500px">
      <el-form :model="bindForm" label-width="120px">
        <el-form-item label="Distribution ID" required>
          <el-input v-model="bindForm.distribution_id" placeholder="CloudFront Distribution ID" />
        </el-form-item>
        <el-form-item label="域名" required>
          <el-input v-model="bindForm.domain_name" placeholder="要绑定的域名" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showBindDialog = false">取消</el-button>
        <el-button type="primary" @click="handleBind" :loading="bindLoading">
          绑定
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { redirectApi } from '@/api/redirect'
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

const showBindDialog = ref(false)
const bindLoading = ref(false)
const bindForm = ref({
  distribution_id: '',
  domain_name: '',
})
const bindRuleId = ref(null)

onMounted(() => {
  loadRedirects()
})

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

const addTargetUrl = () => {
  if (targetUrlInput.value.trim()) {
    createForm.value.target_urls.push(targetUrlInput.value.trim())
    targetUrlInput.value = ''
  }
}

const removeTargetUrl = (index) => {
  createForm.value.target_urls.splice(index, 1)
}

const handleCreate = async () => {
  if (!createForm.value.source_domain || createForm.value.target_urls.length === 0) {
    ElMessage.warning('请填写完整信息')
    return
  }

  createLoading.value = true
  try {
    await redirectApi.createRedirectRule(createForm.value)
    ElMessage.success('重定向规则创建成功')
    showCreateDialog.value = false
    createForm.value = { source_domain: '', target_urls: [] }
    targetUrlInput.value = ''
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

const bindCloudFront = (row) => {
  bindRuleId.value = row.id
  bindForm.value.distribution_id = row.cloudfront_id || ''
  bindForm.value.domain_name = row.source_domain
  showBindDialog.value = true
}

const handleBind = async () => {
  if (!bindForm.value.distribution_id || !bindForm.value.domain_name) {
    ElMessage.warning('请填写完整信息')
    return
  }

  bindLoading.value = true
  try {
    await redirectApi.bindDomainToCloudFront(bindRuleId.value, bindForm.value)
    ElMessage.success('绑定成功')
    showBindDialog.value = false
    loadRedirects()
  } catch (error) {
    // 错误已在拦截器中处理
  } finally {
    bindLoading.value = false
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


