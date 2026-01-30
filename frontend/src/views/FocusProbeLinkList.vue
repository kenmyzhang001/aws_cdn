<template>
  <div class="focus-probe-link-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>重点探测链接管理</span>
          <div style="display: flex; gap: 8px; align-items: center">
            <el-button
              type="primary"
              size="default"
              @click="openBatchIntervalDialog"
              :disabled="selectedLinks.length === 0"
            >
              <el-icon><Setting /></el-icon>
              批量设置间隔
            </el-button>
            <el-button
              type="danger"
              size="default"
              @click="handleBatchDelete"
              :disabled="selectedLinks.length === 0"
            >
              <el-icon><Delete /></el-icon>
              批量删除
            </el-button>
            <el-button type="success" @click="openAddDialog">
              <el-icon><Plus /></el-icon>
              添加链接
            </el-button>
          </div>
        </div>
      </template>

      <!-- 统计信息 -->
      <div style="margin-bottom: 20px; display: flex; gap: 20px; padding: 15px; background: #f5f7fa; border-radius: 4px;">
        <div>
          <span style="color: #909399;">总链接数：</span>
          <span style="font-weight: bold; color: #303133;">{{ statistics.total || 0 }}</span>
        </div>
        <div>
          <span style="color: #909399;">已启用：</span>
          <span style="font-weight: bold; color: #67c23a;">{{ statistics.enabled || 0 }}</span>
        </div>
        <div v-if="statistics.by_type">
          <span style="color: #909399;">AWS CDN：</span>
          <span style="font-weight: bold; color: #409eff;">{{ statistics.by_type.download_package || 0 }}</span>
        </div>
        <div v-if="statistics.by_type">
          <span style="color: #909399;">自定义链接：</span>
          <span style="font-weight: bold; color: #e6a23c;">{{ statistics.by_type.custom_download_link || 0 }}</span>
        </div>
        <div v-if="statistics.by_type">
          <span style="color: #909399;">R2文件：</span>
          <span style="font-weight: bold; color: #f56c6c;">{{ statistics.by_type.r2_file || 0 }}</span>
        </div>
      </div>

      <!-- 搜索和筛选 -->
      <div style="margin-bottom: 20px; display: flex; gap: 12px; align-items: center;">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索URL或名称..."
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
          v-model="linkTypeFilter"
          placeholder="链接类型"
          clearable
          style="width: 150px"
          @change="loadLinks"
        >
          <el-option label="全部" value="" />
          <el-option label="AWS CDN" value="download_package" />
          <el-option label="自定义链接" value="custom_download_link" />
          <el-option label="R2文件" value="r2_file" />
        </el-select>

        <el-select
          v-model="enabledFilter"
          placeholder="启用状态"
          clearable
          style="width: 150px"
          @change="loadLinks"
        >
          <el-option label="全部" value="" />
          <el-option label="已启用" value="true" />
          <el-option label="已禁用" value="false" />
        </el-select>

        <el-button type="primary" @click="openBatchIntervalDialogAll">
          <el-icon><Setting /></el-icon>
          统一设置间隔
        </el-button>
      </div>

      <!-- 链接列表 -->
      <div v-loading="loading">
        <el-table
          v-if="linkList.length > 0"
          :data="linkList"
          stripe
          border
          @selection-change="handleSelectionChange"
        >
          <el-table-column type="selection" width="55" />
          <el-table-column prop="name" label="名称" width="180">
            <template #default="{ row }">
              <span v-if="row.name">{{ row.name }}</span>
              <span v-else style="color: #c0c4cc; font-size: 12px;">未命名</span>
            </template>
          </el-table-column>
          <el-table-column prop="url" label="链接URL" min-width="300">
            <template #default="{ row }">
              <div style="display: flex; align-items: center; gap: 8px;">
                <a :href="row.url" target="_blank" style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #409EFF; text-decoration: none;" :title="row.url">
                  {{ row.url }}
                </a>
                <el-button
                  size="small"
                  type="text"
                  @click="copyLink(row.url)"
                  style="padding: 0; min-height: auto;"
                >
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="link_type" label="类型" width="120">
            <template #default="{ row }">
              <el-tag v-if="row.link_type === 'download_package'" type="primary" size="small">AWS CDN</el-tag>
              <el-tag v-else-if="row.link_type === 'custom_download_link'" type="warning" size="small">自定义链接</el-tag>
              <el-tag v-else-if="row.link_type === 'r2_file'" type="danger" size="small">R2文件</el-tag>
              <el-tag v-else type="info" size="small">{{ row.link_type }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="probe_interval_minutes" label="探测间隔" width="120">
            <template #default="{ row }">
              <el-tag size="small">{{ row.probe_interval_minutes }} 分钟</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="enabled" label="状态" width="100">
            <template #default="{ row }">
              <el-switch
                v-model="row.enabled"
                @change="handleToggleEnabled(row)"
              />
            </template>
          </el-table-column>
          <el-table-column prop="last_probe_time" label="最后探测" width="160">
            <template #default="{ row }">
              <span v-if="row.last_probe_time">{{ formatDate(row.last_probe_time) }}</span>
              <span v-else style="color: #c0c4cc; font-size: 12px;">未探测</span>
            </template>
          </el-table-column>
          <el-table-column prop="last_probe_status" label="探测结果" width="120">
            <template #default="{ row }">
              <el-tag v-if="row.last_probe_status === 'success'" type="success" size="small">成功</el-tag>
              <el-tag v-else-if="row.last_probe_status === 'failed'" type="danger" size="small">失败</el-tag>
              <el-tag v-else-if="row.last_probe_status === 'timeout'" type="warning" size="small">超时</el-tag>
              <span v-else style="color: #c0c4cc; font-size: 12px;">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="last_probe_speed_kbps" label="速度" width="120">
            <template #default="{ row }">
              <span v-if="row.last_probe_speed_kbps">{{ row.last_probe_speed_kbps }} KB/s</span>
              <span v-else style="color: #c0c4cc; font-size: 12px;">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="创建时间" width="160">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="primary" @click="openEditDialog(row)">
                编辑
              </el-button>
              <el-button size="small" type="danger" @click="handleDelete(row)">
                删除
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-empty v-else description="暂无重点探测链接，请先添加链接" />

        <!-- 分页组件 -->
        <div v-if="total > 0" style="margin-top: 20px; display: flex; justify-content: flex-end">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="total"
            layout="total, sizes, prev, pager, next, jumper"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </div>
    </el-card>

    <!-- 添加链接对话框 -->
    <el-dialog v-model="showAddDialog" title="添加重点探测链接" width="600px">
      <el-form :model="linkForm" label-width="120px" :rules="linkRules" ref="linkFormRef">
        <el-form-item label="链接类型" prop="link_type">
          <el-select v-model="linkForm.link_type" placeholder="请选择链接类型" style="width: 100%">
            <el-option label="AWS CDN" value="download_package" />
            <el-option label="自定义链接" value="custom_download_link" />
            <el-option label="R2文件" value="r2_file" />
          </el-select>
        </el-form-item>
        <el-form-item label="链接URL" prop="url">
          <el-input v-model="linkForm.url" placeholder="请输入链接URL" />
        </el-form-item>
        <el-form-item label="链接名称">
          <el-input v-model="linkForm.name" placeholder="请输入链接名称（可选）" />
        </el-form-item>
        <el-form-item label="链接描述">
          <el-input
            v-model="linkForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入链接描述（可选）"
          />
        </el-form-item>
        <el-form-item label="探测间隔（分钟）" prop="probe_interval_minutes">
          <el-input-number
            v-model="linkForm.probe_interval_minutes"
            :min="1"
            :max="1440"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="linkForm.enabled" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAdd" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 编辑链接对话框 -->
    <el-dialog v-model="showEditDialog" title="编辑重点探测链接" width="600px">
      <el-form :model="editForm" label-width="120px" :rules="linkRules" ref="editFormRef">
        <el-form-item label="链接类型">
          <el-select v-model="editForm.link_type" disabled style="width: 100%">
            <el-option label="AWS CDN" value="download_package" />
            <el-option label="自定义链接" value="custom_download_link" />
            <el-option label="R2文件" value="r2_file" />
          </el-select>
        </el-form-item>
        <el-form-item label="链接URL" prop="url">
          <el-input v-model="editForm.url" disabled />
        </el-form-item>
        <el-form-item label="链接名称">
          <el-input v-model="editForm.name" placeholder="请输入链接名称（可选）" />
        </el-form-item>
        <el-form-item label="链接描述">
          <el-input
            v-model="editForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入链接描述（可选）"
          />
        </el-form-item>
        <el-form-item label="探测间隔（分钟）" prop="probe_interval_minutes">
          <el-input-number
            v-model="editForm.probe_interval_minutes"
            :min="1"
            :max="1440"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="editForm.enabled" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="handleEdit" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 批量设置间隔对话框 -->
    <el-dialog v-model="showBatchIntervalDialog" :title="batchIntervalAll ? '统一设置探测间隔' : '批量设置探测间隔'" width="500px">
      <el-form label-width="120px">
        <el-form-item label="探测间隔（分钟）">
          <el-input-number
            v-model="batchIntervalMinutes"
            :min="1"
            :max="1440"
            style="width: 100%"
          />
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            {{ batchIntervalAll ? '将更新所有已启用的重点探测链接' : `将更新选中的 ${selectedLinks.length} 个链接` }}
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showBatchIntervalDialog = false">取消</el-button>
        <el-button type="primary" @click="handleBatchUpdateInterval" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, Delete, CopyDocument, Setting } from '@element-plus/icons-vue'
import {
  getFocusProbeLinks,
  createFocusProbeLink,
  updateFocusProbeLink,
  deleteFocusProbeLink,
  batchDeleteFocusProbeLinks,
  batchUpdateProbeInterval,
  toggleFocusProbeLinkEnabled,
  getFocusProbeLinkStatistics
} from '@/api/focus_probe_link'

// 数据
const loading = ref(false)
const submitting = ref(false)
const linkList = ref([])
const selectedLinks = ref([])
const statistics = ref({})
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const searchKeyword = ref('')
const linkTypeFilter = ref('')
const enabledFilter = ref('')

// 对话框显示状态
const showAddDialog = ref(false)
const showEditDialog = ref(false)
const showBatchIntervalDialog = ref(false)
const batchIntervalAll = ref(false)
const batchIntervalMinutes = ref(30)

// 表单数据
const linkForm = reactive({
  link_type: 'download_package',
  url: '',
  name: '',
  description: '',
  probe_interval_minutes: 30,
  enabled: true
})

const editForm = reactive({
  id: null,
  link_type: '',
  url: '',
  name: '',
  description: '',
  probe_interval_minutes: 30,
  enabled: true
})

// 表单引用
const linkFormRef = ref(null)
const editFormRef = ref(null)

// 表单验证规则
const linkRules = {
  link_type: [{ required: true, message: '请选择链接类型', trigger: 'change' }],
  url: [{ required: true, message: '请输入链接URL', trigger: 'blur' }],
  probe_interval_minutes: [
    { required: true, message: '请输入探测间隔', trigger: 'blur' },
    { type: 'number', min: 1, max: 1440, message: '探测间隔必须在1-1440分钟之间', trigger: 'blur' }
  ]
}

// 加载统计信息
const loadStatistics = async () => {
  try {
    statistics.value = await getFocusProbeLinkStatistics()
  } catch (error) {
    console.error('加载统计信息失败:', error)
  }
}

// 加载链接列表
const loadLinks = async () => {
  loading.value = true
  try {
    const params = {
      page: currentPage.value,
      page_size: pageSize.value
    }
    
    if (linkTypeFilter.value) {
      params.link_type = linkTypeFilter.value
    }
    
    if (enabledFilter.value !== '') {
      params.enabled = enabledFilter.value
    }
    
    if (searchKeyword.value) {
      params.search = searchKeyword.value
    }
    
    const response = await getFocusProbeLinks(params)
    linkList.value = response.data || []
    total.value = response.total || 0
  } catch (error) {
    ElMessage.error('加载链接列表失败: ' + (error.response?.data?.error || error.message))
  } finally {
    loading.value = false
  }
}

// 搜索处理
const handleSearch = () => {
  currentPage.value = 1
  loadLinks()
}

// 分页处理
const handleSizeChange = () => {
  currentPage.value = 1
  loadLinks()
}

const handleCurrentChange = () => {
  loadLinks()
}

// 选择处理
const handleSelectionChange = (selection) => {
  selectedLinks.value = selection
}

// 打开添加对话框
const openAddDialog = () => {
  Object.assign(linkForm, {
    link_type: 'download_package',
    url: '',
    name: '',
    description: '',
    probe_interval_minutes: 30,
    enabled: true
  })
  showAddDialog.value = true
}

// 打开编辑对话框
const openEditDialog = (row) => {
  Object.assign(editForm, {
    id: row.id,
    link_type: row.link_type,
    url: row.url,
    name: row.name,
    description: row.description,
    probe_interval_minutes: row.probe_interval_minutes,
    enabled: row.enabled
  })
  showEditDialog.value = true
}

// 打开批量设置间隔对话框
const openBatchIntervalDialog = () => {
  batchIntervalAll.value = false
  batchIntervalMinutes.value = 30
  showBatchIntervalDialog.value = true
}

// 打开统一设置间隔对话框
const openBatchIntervalDialogAll = () => {
  batchIntervalAll.value = true
  batchIntervalMinutes.value = 30
  showBatchIntervalDialog.value = true
}

// 添加链接
const handleAdd = async () => {
  try {
    await linkFormRef.value.validate()
    submitting.value = true
    
    await createFocusProbeLink(linkForm)
    ElMessage.success('添加成功')
    showAddDialog.value = false
    await loadLinks()
    await loadStatistics()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('添加失败: ' + (error.response?.data?.error || error.message))
    }
  } finally {
    submitting.value = false
  }
}

// 编辑链接
const handleEdit = async () => {
  try {
    await editFormRef.value.validate()
    submitting.value = true
    
    const { id, ...data } = editForm
    await updateFocusProbeLink(id, data)
    ElMessage.success('更新成功')
    showEditDialog.value = false
    await loadLinks()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('更新失败: ' + (error.response?.data?.error || error.message))
    }
  } finally {
    submitting.value = false
  }
}

// 删除链接
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个重点探测链接吗？', '提示', {
      type: 'warning'
    })
    
    await deleteFocusProbeLink(row.id)
    ElMessage.success('删除成功')
    await loadLinks()
    await loadStatistics()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.response?.data?.error || error.message))
    }
  }
}

// 批量删除
const handleBatchDelete = async () => {
  if (selectedLinks.value.length === 0) {
    ElMessage.warning('请先选择要删除的链接')
    return
  }
  
  try {
    await ElMessageBox.confirm(`确定要删除选中的 ${selectedLinks.value.length} 个链接吗？`, '提示', {
      type: 'warning'
    })
    
    const ids = selectedLinks.value.map(link => link.id)
    await batchDeleteFocusProbeLinks(ids)
    ElMessage.success(`批量删除成功，共删除 ${ids.length} 个链接`)
    selectedLinks.value = []
    await loadLinks()
    await loadStatistics()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('批量删除失败: ' + (error.response?.data?.error || error.message))
    }
  }
}

// 批量更新探测间隔
const handleBatchUpdateInterval = async () => {
  try {
    submitting.value = true
    
    if (batchIntervalAll.value) {
      await batchUpdateProbeInterval([], batchIntervalMinutes.value, true)
      ElMessage.success('统一设置探测间隔成功')
    } else {
      const ids = selectedLinks.value.map(link => link.id)
      await batchUpdateProbeInterval(ids, batchIntervalMinutes.value, false)
      ElMessage.success(`批量设置探测间隔成功，共更新 ${ids.length} 个链接`)
    }
    
    showBatchIntervalDialog.value = false
    await loadLinks()
  } catch (error) {
    ElMessage.error('设置探测间隔失败: ' + (error.response?.data?.error || error.message))
  } finally {
    submitting.value = false
  }
}

// 切换启用状态
const handleToggleEnabled = async (row) => {
  try {
    await toggleFocusProbeLinkEnabled(row.id)
    ElMessage.success(row.enabled ? '已启用' : '已禁用')
    await loadStatistics()
  } catch (error) {
    // 恢复原状态
    row.enabled = !row.enabled
    ElMessage.error('操作失败: ' + (error.response?.data?.error || error.message))
  }
}

// 复制链接
const copyLink = (url) => {
  navigator.clipboard.writeText(url).then(() => {
    ElMessage.success('链接已复制到剪贴板')
  }).catch(() => {
    ElMessage.error('复制失败')
  })
}

// 格式化日期
const formatDate = (dateString) => {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

// 组件挂载时加载数据
onMounted(() => {
  loadStatistics()
  loadLinks()
  
  // 每30秒刷新一次
  setInterval(() => {
    loadLinks()
    loadStatistics()
  }, 30000)
})
</script>

<style scoped>
.focus-probe-link-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
