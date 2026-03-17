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
      width="520px"
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
    const form = ref({
      name_zh: '',
      name_my: '',
      default_lang: 'zh'
    })

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
      form.value = { name_zh: '', name_my: '', default_lang: 'zh' }
      dialogVisible.value = true
    }

    const openEditDialog = (row) => {
      editId.value = row.id
      form.value = {
        name_zh: row.name_zh || '',
        name_my: row.name_my || '',
        default_lang: row.default_lang || 'zh'
      }
      dialogVisible.value = true
    }

    const resetForm = () => {
      formRef.value?.resetFields?.()
      editId.value = null
    }

    const submit = async () => {
      submitLoading.value = true
      try {
        if (editId.value) {
          await workpageTemplateApi.update(editId.value, {
            name_zh: form.value.name_zh,
            name_my: form.value.name_my,
            default_lang: form.value.default_lang
          })
          ElMessage.success('保存成功')
        } else {
          await workpageTemplateApi.create({
            name_zh: form.value.name_zh,
            name_my: form.value.name_my,
            default_lang: form.value.default_lang || 'zh'
          })
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
      formatDate
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
</style>
