<template>
  <div class="workpage-template-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>模版管理</span>
          <el-button type="primary" @click="openCreateDialog">
            <el-icon><Plus /></el-icon>
            新增
          </el-button>
        </div>
      </template>

      <div class="filter-section">
        <el-form :inline="true">
          <el-form-item label="关键词">
            <el-input
              v-model="filterKeyword"
              placeholder="模版名称（中文/缅甸文）"
              clearable
              style="width: 220px"
              @keyup.enter="fetchList"
            />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="fetchList">
              <el-icon><Search /></el-icon>
              查询
            </el-button>
          </el-form-item>
        </el-form>
      </div>

      <el-table :data="list" v-loading="loading" stripe>
        <el-table-column prop="id" label="Id" width="80" />
        <el-table-column prop="name_zh" label="模版名称（中文）" min-width="160" show-overflow-tooltip />
        <el-table-column prop="name_my" label="模版名称（缅甸文）" min-width="160" show-overflow-tooltip />
        <el-table-column label="落地页默认语言" width="140">
          <template #default="{ row }">
            <el-tag :type="row.default_lang === 'zh' ? 'primary' : 'success'">
              {{ row.default_lang === 'zh' ? '中文' : '缅甸文' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="openEditDialog(row)">
              编辑
            </el-button>
            <el-button type="danger" link size="small" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.page_size"
        :total="pagination.total"
        :page-sizes="[15, 30, 60, 130]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="fetchList"
        @current-change="fetchList"
        style="margin-top: 16px; justify-content: flex-end;"
      />
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editId ? '编辑模版' : '新增模版'"
      width="900px"
      @close="resetForm"
    >
      <el-form ref="formRef" :model="form" label-width="140px">
        <el-form-item label="模版名称（中文）">
          <el-input v-model="form.name_zh" placeholder="请输入中文名称" clearable />
        </el-form-item>
        <el-form-item label="模版名称（缅甸文）">
          <el-input v-model="form.name_my" placeholder="请输入缅甸文名称" clearable />
        </el-form-item>
        <el-form-item label="落地页默认语言">
          <el-radio-group v-model="form.default_lang">
            <el-radio value="zh">中文</el-radio>
            <el-radio value="my">缅甸文</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-divider content-position="left">表格行（固定 3 列，每行一个下载链接；点击「立即领取」可下载）</el-divider>
        <div class="table-rows-wrap">
          <div style="margin-bottom: 8px; display: flex; align-items: center; gap: 12px;">
            <el-button type="primary" size="small" @click="addRow">
              <el-icon><Plus /></el-icon>
              添加一行
            </el-button>
            <span style="color: #606266; font-size: 13px;">访问页面时自动弹出下载：</span>
            <el-select v-model="autoPopupIndex" placeholder="无" size="small" style="width: 120px" @change="syncAutoPopupToRows">
              <el-option label="无" :value="-1" />
              <el-option v-for="(r, i) in form.rows" :key="i" :label="`第 ${i + 1} 行`" :value="i" />
            </el-select>
          </div>
          <el-table :data="form.rows" border size="small" max-height="320">
            <el-table-column label="列1（中文）" width="100">
              <template #default="{ row }">
                <el-input v-model="row.col1_zh" placeholder="如：最新优惠" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="列1（缅）" width="100">
              <template #default="{ row }">
                <el-input v-model="row.col1_my" placeholder="缅甸文" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="列2（中文）" width="120">
              <template #default="{ row }">
                <el-input v-model="row.col2_zh" placeholder="如：注册送8888" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="列2（缅）" width="120">
              <template #default="{ row }">
                <el-input v-model="row.col2_my" placeholder="缅甸文" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="列3（中文）" width="90">
              <template #default="{ row }">
                <el-input v-model="row.col3_zh" placeholder="如：立即领取" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="列3（缅）" width="90">
              <template #default="{ row }">
                <el-input v-model="row.col3_my" placeholder="缅甸文" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="下载链接" min-width="180">
              <template #default="{ row }">
                <el-input v-model="row.download_url" placeholder="点击按钮时打开的链接" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="70" fixed="right">
              <template #default="{ $index }">
                <el-button type="danger" link size="small" @click="removeRow($index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="submit">
          {{ editId ? '保存' : '创建' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { workpageTemplateApi } from '@/api/workpage_template'

export default {
  name: 'WorkpageTemplateList',
  components: { Plus, Search },
  setup() {
    const loading = ref(false)
    const list = ref([])
    const filterKeyword = ref('')
    const pagination = ref({
      page: 1,
      page_size: 15,
      total: 0
    })

    const dialogVisible = ref(false)
    const editId = ref(null)
    const formRef = ref(null)
    const submitLoading = ref(false)
    const autoPopupIndex = ref(-1) // 访问页自动弹出下载的行索引，-1 表示无
    const form = ref({
      name_zh: '',
      name_my: '',
      default_lang: 'zh',
      rows: [] // 表格行：{ col1_zh, col1_my, col2_zh, col2_my, col3_zh, col3_my, download_url, auto_popup }
    })

    const emptyRow = () => ({
      col1_zh: '',
      col1_my: '',
      col2_zh: '',
      col2_my: '',
      col3_zh: '立即领取',
      col3_my: '',
      download_url: '',
      auto_popup: false
    })

    const addRow = () => {
      form.value.rows = form.value.rows || []
      form.value.rows.push(emptyRow())
    }

    const removeRow = (index) => {
      form.value.rows.splice(index, 1)
      if (autoPopupIndex.value === index) {
        autoPopupIndex.value = -1
      } else if (autoPopupIndex.value > index) {
        autoPopupIndex.value--
      }
    }

    const syncAutoPopupToRows = () => {
      const idx = autoPopupIndex.value
      form.value.rows.forEach((r, i) => {
        r.auto_popup = i === idx
      })
    }

    const fetchList = async () => {
      loading.value = true
      try {
        const params = {
          page: pagination.value.page,
          page_size: pagination.value.page_size
        }
        if (filterKeyword.value) params.keyword = filterKeyword.value.trim()
        const res = await workpageTemplateApi.list(params)
        list.value = res?.data ?? []
        pagination.value.total = res?.pagination?.total ?? 0
      } catch (e) {
        list.value = []
      } finally {
        loading.value = false
      }
    }

    const formatDate = (dateString) => {
      if (!dateString) return '-'
      return new Date(dateString).toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
      })
    }

    const openCreateDialog = () => {
      editId.value = null
      form.value = {
        name_zh: '',
        name_my: '',
        default_lang: 'zh',
        rows: []
      }
      autoPopupIndex.value = -1
      dialogVisible.value = true
    }

    const openEditDialog = async (row) => {
      editId.value = row.id
      form.value = {
        name_zh: row.name_zh || '',
        name_my: row.name_my || '',
        default_lang: row.default_lang || 'zh',
        rows: []
      }
      autoPopupIndex.value = -1
      dialogVisible.value = true
      try {
        const rows = await workpageTemplateApi.getRows(row.id)
        form.value.rows = Array.isArray(rows) ? rows.map(r => ({
          col1_zh: r.col1_zh || '',
          col1_my: r.col1_my || '',
          col2_zh: r.col2_zh || '',
          col2_my: r.col2_my || '',
          col3_zh: r.col3_zh || '立即领取',
          col3_my: r.col3_my || '',
          download_url: r.download_url || '',
          auto_popup: !!r.auto_popup
        })) : []
        const idx = form.value.rows.findIndex(r => r.auto_popup)
        autoPopupIndex.value = idx >= 0 ? idx : -1
      } catch (e) {
        form.value.rows = []
      }
    }

    const resetForm = () => {
      formRef.value?.resetFields?.()
      editId.value = null
      autoPopupIndex.value = -1
    }

    const submit = async () => {
      syncAutoPopupToRows()
      submitLoading.value = true
      try {
        let templateId = editId.value
        if (editId.value) {
          await workpageTemplateApi.update(editId.value, {
            name_zh: form.value.name_zh,
            name_my: form.value.name_my,
            default_lang: form.value.default_lang
          })
          if ((form.value.rows || []).length > 0) {
            await workpageTemplateApi.saveRows(editId.value, form.value.rows.map(r => ({
              col1_zh: r.col1_zh,
              col1_my: r.col1_my,
              col2_zh: r.col2_zh,
              col2_my: r.col2_my,
              col3_zh: r.col3_zh || '立即领取',
              col3_my: r.col3_my,
              download_url: r.download_url || '',
              auto_popup: !!r.auto_popup
            })))
          } else {
            await workpageTemplateApi.saveRows(editId.value, [])
          }
          ElMessage.success('保存成功')
        } else {
          const created = await workpageTemplateApi.create({
            name_zh: form.value.name_zh,
            name_my: form.value.name_my,
            default_lang: form.value.default_lang || 'zh'
          })
          templateId = created?.id
          if (templateId && (form.value.rows || []).length > 0) {
            await workpageTemplateApi.saveRows(templateId, form.value.rows.map(r => ({
              col1_zh: r.col1_zh,
              col1_my: r.col1_my,
              col2_zh: r.col2_zh,
              col2_my: r.col2_my,
              col3_zh: r.col3_zh || '立即领取',
              col3_my: r.col3_my,
              download_url: r.download_url || '',
              auto_popup: !!r.auto_popup
            })))
          }
          ElMessage.success('创建成功')
        }
        dialogVisible.value = false
        fetchList()
      } catch (e) {
        // error shown by request
      } finally {
        submitLoading.value = false
      }
    }

    const handleDelete = async (row) => {
      try {
        await ElMessageBox.confirm(
          `确定要删除模版「${row.name_zh || row.name_my || row.id}」吗？`,
          '确认删除',
          { type: 'warning' }
        )
        await workpageTemplateApi.delete(row.id)
        ElMessage.success('已删除')
        fetchList()
      } catch (e) {
        if (e === 'cancel') return
      }
    }

    onMounted(() => fetchList())

    return {
      loading,
      list,
      filterKeyword,
      pagination,
      fetchList,
      openCreateDialog,
      openEditDialog,
      handleDelete,
      dialogVisible,
      editId,
      formRef,
      form,
      submitLoading,
      submit,
      resetForm,
      formatDate,
      addRow,
      removeRow,
      autoPopupIndex,
      syncAutoPopupToRows
    }
  }
}
</script>

<style scoped>
.workpage-template-list {
  padding: 20px;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.filter-section {
  margin-bottom: 20px;
}
.table-rows-wrap {
  margin-top: 8px;
}
</style>
