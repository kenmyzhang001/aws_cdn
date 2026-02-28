<template>
  <div class="ec2-instance-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>代理管理</span>
          <el-button type="primary" @click="showCreateDialog" :disabled="activeTab !== 'list'">
            <el-icon><Plus /></el-icon>
            创建代理
          </el-button>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <el-tab-pane label="代理列表" name="list">
          <el-form :inline="true" class="search-form">
            <el-form-item label="地区">
              <el-select v-model="searchRegion" clearable placeholder="全部" style="width: 160px" @change="loadList">
                <el-option v-for="r in regionConfigs" :key="r.region" :label="r.region" :value="r.region" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="loadList">查询</el-button>
              <el-button
                type="success"
                :disabled="!list.length || !list.some((r) => r.public_ip)"
                @click="copyAllLinks"
              >
                <el-icon><CopyDocument /></el-icon>
                一键复制所有链接
              </el-button>
            </el-form-item>
          </el-form>
          <el-table v-loading="loading" :data="list" border stripe>
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column prop="name" label="名称" width="120" />
            <el-table-column prop="region" label="地区" width="120" />
            <el-table-column prop="aws_instance_id" label="实例 ID" width="180" show-overflow-tooltip />
            <el-table-column label="链接" min-width="320">
              <template #default="{ row }">
                <div class="link-cell">
                  <div class="link-text" :title="getProxyLink(row)">
                    {{ row.public_ip ? getProxyLink(row) : '暂无IP' }}
                  </div>
                  <el-button
                    link
                    type="primary"
                    size="small"
                    class="copy-btn"
                    :disabled="!row.public_ip"
                    @click="copyLink(row)"
                  >
                    <el-icon><CopyDocument /></el-icon>
                    复制
                  </el-button>
                </div>
              </template>
            </el-table-column>
            <!--el-table-column prop="state" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.state === 'running' ? 'success' : row.state === 'terminated' ? 'info' : 'warning'">
                  {{ row.state || 'pending' }}
                </el-tag>
              </template>
            </el-table-column-->
            <el-table-column prop="note" label="备注" min-width="120" show-overflow-tooltip />
            <el-table-column label="创建时间" width="170">
              <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="160" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="editRow(row)">编辑</el-button>
                <el-button link type="danger" size="small" @click="handleDelete(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            v-model:current-page="pagination.page"
            v-model:page-size="pagination.page_size"
            :total="pagination.total"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            style="margin-top: 16px"
            @size-change="loadList"
            @current-change="loadList"
          />
        </el-tab-pane>
        <el-tab-pane label="回收站" name="deleted">
          <el-table v-loading="loadingDeleted" :data="deletedList" border stripe>
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column prop="name" label="名称" width="120" />
            <el-table-column prop="region" label="地区" width="110" />
            <el-table-column prop="aws_instance_id" label="实例 ID" width="180" show-overflow-tooltip />
            <el-table-column prop="state" label="状态" width="100" />
            <el-table-column label="运行时长(小时)" width="120">
              <template #default="{ row }">
                {{ row.lifetime_hours != null ? Number(row.lifetime_hours).toFixed(2) : '-' }}
              </template>
            </el-table-column>
            <el-table-column label="删除时间" width="170">
              <template #default="{ row }">{{ formatDate(row.deleted_at) }}</template>
            </el-table-column>
            <el-table-column prop="note" label="备注" min-width="120" show-overflow-tooltip />
          </el-table>
          <el-pagination
            v-model:current-page="paginationDeleted.page"
            v-model:page-size="paginationDeleted.page_size"
            :total="paginationDeleted.total"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            style="margin-top: 16px"
            @size-change="loadDeleted"
            @current-change="loadDeleted"
          />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 创建实例 -->
    <el-dialog v-model="createVisible" title="创建实例" width="520px" @close="resetCreateForm">
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="100px">
        <el-form-item label="地区" prop="region">
          <el-select v-model="createForm.region" placeholder="请选择地区" style="width: 100%">
            <el-option
              v-for="r in regionConfigs"
              :key="r.region"
              :label="r.region"
              :value="r.region"
            >
              <span>{{ r.region }}</span>
              <span style="color: #909399; font-size: 12px; margin-left: 8px">AMI: {{ r.ami_id }}</span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="名称" prop="name">
          <el-input v-model="createForm.name" placeholder="实例名称/备注" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="createForm.note" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
        <div style="color: #909399; font-size: 12px">实例规格固定为 t3.micro，AMI 与安全组由所选地区自动匹配。</div>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="submitCreate" :loading="createLoading">创建</el-button>
      </template>
    </el-dialog>

    <!-- 编辑 -->
    <el-dialog v-model="editVisible" title="编辑实例" width="480px" @close="resetEditForm">
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="editForm.name" placeholder="实例名称" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="editForm.note" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" @click="submitEdit" :loading="editLoading">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, CopyDocument } from '@element-plus/icons-vue'
import {
  getRegionConfig,
  getEc2InstanceList,
  getEc2InstanceDeletedList,
  createEc2Instance,
  updateEc2Instance,
  deleteEc2Instance
} from '@/api/ec2_instance'

const activeTab = ref('list')
const loading = ref(false)
const loadingDeleted = ref(false)
const list = ref([])
const deletedList = ref([])
const regionConfigs = ref([])
const searchRegion = ref('')

const pagination = ref({ page: 1, page_size: 10, total: 0 })
const paginationDeleted = ref({ page: 1, page_size: 10, total: 0 })

const createVisible = ref(false)
const createLoading = ref(false)
const createForm = ref({ region: '', name: '', note: '' })
const createFormRef = ref(null)
const createRules = {
  region: [{ required: true, message: '请选择地区', trigger: 'change' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }]
}

const editVisible = ref(false)
const editLoading = ref(false)
const editForm = ref({ id: null, name: '', note: '' })
const editFormRef = ref(null)
const editRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }]
}

const SS_LINK_PREFIX = 'ss://YWVzLTI1Ni1nY206YXdzMjAyMjAxMjE=@'
const SS_LINK_SUFFIX = ':8388/?#HK'

function getProxyLink(row) {
  if (!row.public_ip) return ''
  return `${SS_LINK_PREFIX}${row.public_ip}${SS_LINK_SUFFIX}${row.name || ''}`
}

function copyLink(row) {
  const link = getProxyLink(row)
  if (!link) return
  navigator.clipboard.writeText(link).then(() => {
    ElMessage.success('已复制链接')
  }).catch(() => {
    ElMessage.error('复制失败')
  })
}

function copyAllLinks() {
  const links = list.value.map(getProxyLink).filter(Boolean)
  if (!links.length) {
    ElMessage.warning('当前页暂无链接')
    return
  }
  const text = links.join('\n')
  navigator.clipboard.writeText(text).then(() => {
    ElMessage.success(`已复制 ${links.length} 条链接`)
  }).catch(() => {
    ElMessage.error('复制失败')
  })
}

function formatDate(v) {
  if (!v) return '-'
  const d = new Date(v)
  return isNaN(d.getTime()) ? v : d.toLocaleString()
}

function loadRegionConfig() {
  getRegionConfig().then((res) => {
    regionConfigs.value = res.data || []
  }).catch(() => {})
}

function loadList() {
  loading.value = true
  getEc2InstanceList({
    page: pagination.value.page,
    page_size: pagination.value.page_size,
    region: searchRegion.value || undefined
  })
    .then((res) => {
      list.value = res.data || []
      pagination.value.total = res.pagination?.total ?? 0
    })
    .finally(() => { loading.value = false })
}

function loadDeleted() {
  loadingDeleted.value = true
  getEc2InstanceDeletedList({
    page: paginationDeleted.value.page,
    page_size: paginationDeleted.value.page_size
  })
    .then((res) => {
      deletedList.value = res.data || []
      paginationDeleted.value.total = res.pagination?.total ?? 0
    })
    .finally(() => { loadingDeleted.value = false })
}

function onTabChange(name) {
  if (name === 'deleted') loadDeleted()
  else loadList()
}

function showCreateDialog() {
  createVisible.value = true
}

function resetCreateForm() {
  createForm.value = { region: '', name: '', note: '' }
  createFormRef.value?.clearValidate()
}

function submitCreate() {
  createFormRef.value?.validate((valid) => {
    if (!valid) return
    createLoading.value = true
    createEc2Instance(createForm.value)
      .then(() => {
        ElMessage.success('创建成功')
        createVisible.value = false
        loadList()
      })
      .finally(() => { createLoading.value = false })
  })
}

function editRow(row) {
  editForm.value = { id: row.id, name: row.name, note: row.note || '' }
  editVisible.value = true
}

function resetEditForm() {
  editForm.value = { id: null, name: '', note: '' }
  editFormRef.value?.clearValidate()
}

function submitEdit() {
  editFormRef.value?.validate((valid) => {
    if (!valid) return
    editLoading.value = true
    updateEc2Instance(editForm.value.id, { name: editForm.value.name, note: editForm.value.note })
      .then(() => {
        ElMessage.success('更新成功')
        editVisible.value = false
        loadList()
      })
      .finally(() => { editLoading.value = false })
  })
}

function handleDelete(row) {
  ElMessageBox.confirm(
    '删除将执行：1) 数据库软删除 2) 在 AWS 上终止该实例，并记录运行时长。是否继续？',
    '确认删除',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }
  ).then(() => {
    return deleteEc2Instance(row.id)
  }).then(() => {
    ElMessage.success('已删除并终止实例')
    loadList()
  }).catch((e) => {
    if (e !== 'cancel') ElMessage.error(e?.message || '删除失败')
  })
}

onMounted(() => {
  loadRegionConfig()
  loadList()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.search-form { margin-bottom: 16px; }
.link-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}
.link-text {
  width: 100%;
  word-break: break-all;
  white-space: pre-wrap;
  line-height: 1.4;
  font-size: 12px;
}
.copy-btn {
  flex-shrink: 0;
}
</style>
