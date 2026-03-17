<template>
  <div class="workpage-site-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>站点管理</span>
          <el-button type="primary" @click="openCreateDialog">
            <el-icon><Plus /></el-icon>
            新增
          </el-button>
        </div>
      </template>

      <div class="filter-section">
        <el-form :inline="true">
          <el-form-item label="CF 账号">
            <el-select
              v-model="filterCfAccountId"
              placeholder="全部"
              clearable
              style="width: 280px"
              @change="onFilterChange"
            >
              <el-option
                v-for="a in cfAccountList"
                :key="a.id"
                :label="`${a.email} (${a.note || a.account_id || '-'})`"
                :value="a.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="模版">
            <el-select
              v-model="filterTemplateId"
              placeholder="全部"
              clearable
              style="width: 200px"
              @change="onFilterChange"
            >
              <el-option
                v-for="t in templateList"
                :key="t.id"
                :label="t.name_zh || t.name_my || `#${t.id}`"
                :value="t.id"
              />
            </el-select>
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
        <el-table-column label="CF 账号" width="200">
          <template #default="{ row }">
            <span v-if="row.cf_account">{{ row.cf_account.email }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="模版" width="160">
          <template #default="{ row }">
            <span v-if="row.template">{{ row.template.name_zh || row.template.name_my || `#${row.template_id}` }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="main_domain" label="主域名" width="180" />
        <el-table-column prop="subdomain" label="子域名" width="120">
          <template #default="{ row }">
            {{ row.subdomain || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="访问地址" min-width="180">
          <template #default="{ row }">
            <a v-if="row.deployment_url" :href="row.deployment_url" target="_blank" rel="noreferrer">
              {{ row.deployment_url }}
            </a>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button
              type="success"
              link
              size="small"
              :disabled="row.status === 'deploying'"
              @click="handleDeploy(row)"
            >
              {{ row.status === 'deployed' ? '重新部署' : '部署' }}
            </el-button>
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
      :title="editId ? '编辑站点' : '新增站点'"
      width="520px"
      @close="resetForm"
    >
      <el-form ref="formRef" :model="form" label-width="100px">
        <el-form-item v-if="!editId" label="CF 账号" required>
          <el-select
            v-model="form.cf_account_id"
            placeholder="请选择 CF 账号"
            style="width: 100%"
            @change="onCfAccountChange"
          >
            <el-option
              v-for="a in cfAccountList"
              :key="a.id"
              :label="`${a.email} (${a.note || a.account_id || '-'})`"
              :value="a.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="!editId" label="主域名" required>
          <el-select
            v-model="form.zone_id"
            placeholder="请先选择 CF 账号"
            style="width: 100%"
            filterable
            :disabled="!form.cf_account_id"
          >
            <el-option
              v-for="z in zoneList"
              :key="z.id"
              :label="z.name"
              :value="z.id"
            >
              <span>{{ z.name }}</span>
              <span style="color: #909399; margin-left: 8px; font-size: 12px">{{ z.id }}</span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item v-if="editId" label="主域名">
          <span>{{ form.main_domain }}</span>
        </el-form-item>
        <el-form-item v-if="!editId" label="模版" required>
          <el-select
            v-model="form.template_id"
            placeholder="请选择模版"
            style="width: 100%"
          >
            <el-option
              v-for="t in templateList"
              :key="t.id"
              :label="t.name_zh || t.name_my || `#${t.id}`"
              :value="t.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="子域名">
          <el-input
            v-model="form.subdomain"
            placeholder="留空或输入 www、app 等子域名前缀"
            clearable
          />
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
import { cfAccountApi } from '@/api/cf_account'
import { getCFAccountZones } from '@/api/cf_zone'
import { workpageTemplateApi } from '@/api/workpage_template'
import { workpageSiteApi } from '@/api/workpage_site'

export default {
  name: 'WorkpageSiteList',
  components: { Plus, Search },
  setup() {
    const loading = ref(false)
    const list = ref([])
    const cfAccountList = ref([])
    const templateList = ref([])
    const filterCfAccountId = ref(null)
    const filterTemplateId = ref(null)
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
      cf_account_id: null,
      zone_id: '',
      main_domain: '',
      template_id: null,
      subdomain: ''
    })

    const zoneList = ref([])

    const fetchCFAccounts = async () => {
      try {
        const res = await cfAccountApi.getCFAccountList()
        cfAccountList.value = res || []
      } catch (e) {
        ElMessage.error('获取 CF 账号列表失败')
      }
    }

    const fetchTemplates = async () => {
      try {
        const res = await workpageTemplateApi.list({ page: 1, page_size: 500 })
        templateList.value = res?.data ?? []
      } catch (e) {
        templateList.value = []
      }
    }

    const fetchList = async () => {
      loading.value = true
      try {
        const params = {
          page: pagination.value.page,
          page_size: pagination.value.page_size
        }
        if (filterCfAccountId.value) params.cf_account_id = filterCfAccountId.value
        if (filterTemplateId.value) params.template_id = filterTemplateId.value
        const res = await workpageSiteApi.list(params)
        list.value = res?.data ?? []
        pagination.value.total = res?.pagination?.total ?? 0
      } catch (e) {
        list.value = []
      } finally {
        loading.value = false
      }
    }

    const onFilterChange = () => {
      pagination.value.page = 1
      fetchList()
    }

    const onCfAccountChange = async () => {
      form.value.zone_id = ''
      form.value.main_domain = ''
      zoneList.value = []
      if (!form.value.cf_account_id) return
      try {
        const res = await getCFAccountZones(form.value.cf_account_id, 1, 50, '')
        zoneList.value = res?.zones || []
      } catch (e) {
        zoneList.value = []
      }
    }

    const zoneNameById = (zoneId) => {
      const z = zoneList.value.find((x) => x.id === zoneId)
      return z ? z.name : zoneId
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

    const statusText = (s) => {
      const m = { pending: '待部署', deploying: '部署中', deployed: '已部署', failed: '失败' }
      return m[s] || s
    }

    const statusType = (s) => {
      const m = { pending: 'info', deploying: 'warning', deployed: 'success', failed: 'danger' }
      return m[s] || 'info'
    }

    const openCreateDialog = () => {
      editId.value = null
      form.value = {
        cf_account_id: cfAccountList.value.length === 1 ? cfAccountList.value[0].id : null,
        zone_id: '',
        main_domain: '',
        template_id: templateList.value.length === 1 ? templateList.value[0].id : null,
        subdomain: ''
      }
      zoneList.value = []
      if (form.value.cf_account_id) onCfAccountChange()
      dialogVisible.value = true
    }

    const openEditDialog = (row) => {
      editId.value = row.id
      form.value = {
        main_domain: row.main_domain,
        subdomain: row.subdomain || ''
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
          await workpageSiteApi.update(editId.value, {
            subdomain: form.value.subdomain?.trim() || ''
          })
          ElMessage.success('保存成功')
        } else {
          if (!form.value.cf_account_id) {
            ElMessage.warning('请选择 CF 账号')
            submitLoading.value = false
            return
          }
          if (!form.value.zone_id) {
            ElMessage.warning('请选择主域名')
            submitLoading.value = false
            return
          }
          const mainDomain = zoneNameById(form.value.zone_id)
          if (!mainDomain) {
            ElMessage.warning('无法解析主域名')
            submitLoading.value = false
            return
          }
          if (!form.value.template_id) {
            ElMessage.warning('请选择模版')
            submitLoading.value = false
            return
          }
          const created = await workpageSiteApi.create({
            cf_account_id: form.value.cf_account_id,
            template_id: form.value.template_id,
            zone_id: form.value.zone_id,
            main_domain: mainDomain,
            subdomain: (form.value.subdomain || '').trim()
          })
          ElMessage.success('创建成功，开始部署…')
          const siteId = created?.id
          if (siteId) {
            try {
              await workpageSiteApi.deploy(siteId)
              ElMessage.success('部署已触发')
            } catch (e) {
              // error shown by request
            }
          }
        }
        dialogVisible.value = false
        fetchList()
      } catch (e) {
        // error shown by request
      } finally {
        submitLoading.value = false
      }
    }

    const handleDeploy = async (row) => {
      try {
        await workpageSiteApi.deploy(row.id)
        ElMessage.success('已触发部署')
        fetchList()
      } catch (e) {
        // error shown by request
      }
    }

    const handleDelete = async (row) => {
      try {
        await ElMessageBox.confirm(
          `确定要删除站点「${row.main_domain}${row.subdomain ? '.' + row.subdomain : ''}」吗？`,
          '确认删除',
          { type: 'warning' }
        )
        await workpageSiteApi.delete(row.id)
        ElMessage.success('已删除')
        fetchList()
      } catch (e) {
        if (e === 'cancel') return
      }
    }

    onMounted(() => {
      fetchCFAccounts()
      fetchTemplates().then(() => fetchList())
    })

    return {
      loading,
      list,
      cfAccountList,
      templateList,
      filterCfAccountId,
      filterTemplateId,
      pagination,
      fetchList,
      onFilterChange,
      openCreateDialog,
      openEditDialog,
      handleDeploy,
      handleDelete,
      dialogVisible,
      editId,
      formRef,
      form,
      submitLoading,
      submit,
      resetForm,
      zoneList,
      onCfAccountChange,
      zoneNameById,
      formatDate,
      statusText,
      statusType
    }
  }
}
</script>

<style scoped>
.workpage-site-list {
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
