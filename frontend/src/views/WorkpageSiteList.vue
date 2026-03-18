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
            <div class="access-addresses">
              <div v-if="row.deployment_url" class="access-address-row">
                <a :href="row.deployment_url" target="_blank" rel="noreferrer">
                  {{ row.deployment_url }}
                </a>
                <el-button
                  type="primary"
                  link
                  size="small"
                  class="copy-link-btn"
                  title="复制"
                  @click="copyText(row.deployment_url)"
                >
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </div>

              <div v-if="row.custom_domain" class="access-address-row">
                <a :href="customDomainUrl(row.custom_domain)" target="_blank" rel="noreferrer">
                  {{ row.custom_domain }}
                </a>
                <el-button
                  type="primary"
                  link
                  size="small"
                  class="copy-link-btn"
                  title="复制"
                  @click="copyText(row.custom_domain)"
                >
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </div>

              <span v-if="!row.deployment_url && !row.custom_domain">-</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="380" fixed="right">
          <template #default="{ row }">
            <el-button type="info" link size="small" @click="handlePreview(row)">
              预览
            </el-button>
            <el-button
              type="warning"
              link
              size="small"
              :disabled="row.status !== 'deployed'"
              @click="handleViewDeployedHtml(row)"
            >
              部署HTML
            </el-button>
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
            remote
            reserve-keyword
            :remote-method="onZoneSearch"
            :loading="zoneLoading"
            :disabled="!form.cf_account_id"
            @visible-change="onZoneDropdownVisibleChange"
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
            <template #footer>
              <div style="padding: 8px 12px; display: flex; justify-content: center;">
                <el-button
                  v-if="zoneHasMore"
                  size="small"
                  :loading="zoneLoadingMore"
                  :disabled="zoneLoading"
                  @click.stop="loadMoreZones"
                >
                  加载更多
                </el-button>
                <span v-else style="color: #909399; font-size: 12px;">没有更多了</span>
              </div>
            </template>
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

    <el-dialog
      v-model="deployedHtmlDialogVisible"
      title="已部署 index.html 原文（与上次成功上传 Pages 一致）"
      width="90%"
      top="5vh"
      destroy-on-close
      @closed="deployedHtmlContent = ''"
    >
      <div v-loading="deployedHtmlLoading" style="min-height: 200px">
        <el-input
          v-model="deployedHtmlContent"
          type="textarea"
          :rows="22"
          readonly
          placeholder="加载中…"
          class="deployed-html-textarea"
        />
      </div>
      <template #footer>
        <el-button @click="deployedHtmlDialogVisible = false">关闭</el-button>
        <el-button type="primary" :disabled="!deployedHtmlContent" @click="copyDeployedHtml">
          复制全文
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, CopyDocument } from '@element-plus/icons-vue'
import { cfAccountApi } from '@/api/cf_account'
import { getCFAccountZones } from '@/api/cf_zone'
import { workpageTemplateApi } from '@/api/workpage_template'
import { workpageSiteApi } from '@/api/workpage_site'
import { openAuthPreview } from '@/utils/openAuthPreview'

export default {
  name: 'WorkpageSiteList',
  components: { Plus, Search, CopyDocument },
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
    const zoneLoading = ref(false)
    const zoneLoadingMore = ref(false)
    const zoneSearchKeyword = ref('')
    const zonePage = ref(1)
    const zonePerPage = ref(50)
    const zoneHasMore = ref(false)
    let zoneSearchTimer = null

    const deployedHtmlDialogVisible = ref(false)
    const deployedHtmlLoading = ref(false)
    const deployedHtmlContent = ref('')

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

    const mergeZonesById = (base, extra) => {
      const m = new Map()
      ;(base || []).forEach((z) => m.set(z.id, z))
      ;(extra || []).forEach((z) => m.set(z.id, z))
      return Array.from(m.values())
    }

    const updateZoneHasMore = (resp, fetchedCount) => {
      const total = resp?.pagination?.total
      const page = resp?.pagination?.page
      const perPage = resp?.pagination?.per_page
      if (typeof total === 'number' && typeof page === 'number' && typeof perPage === 'number') {
        zoneHasMore.value = page * perPage < total
        return
      }
      zoneHasMore.value = (fetchedCount || 0) >= zonePerPage.value
    }

    const fetchZones = async ({ keyword, page, append } = {}) => {
      if (!form.value.cf_account_id) return
      const reqKeyword = keyword ?? zoneSearchKeyword.value ?? ''
      const reqPage = page ?? zonePage.value ?? 1
      if (append) zoneLoadingMore.value = true
      else zoneLoading.value = true
      try {
        const res = await getCFAccountZones(form.value.cf_account_id, reqPage, zonePerPage.value, reqKeyword || '')
        const zones = res?.zones || []
        if (append) zoneList.value = mergeZonesById(zoneList.value, zones)
        else zoneList.value = zones
        updateZoneHasMore(res, zones.length)
      } catch (e) {
        if (!append) zoneList.value = []
        zoneHasMore.value = false
      } finally {
        zoneLoading.value = false
        zoneLoadingMore.value = false
      }
    }

    const onCfAccountChange = async () => {
      form.value.zone_id = ''
      form.value.main_domain = ''
      zoneList.value = []
      zoneSearchKeyword.value = ''
      zonePage.value = 1
      zoneHasMore.value = false
      if (!form.value.cf_account_id) return
      await fetchZones({ keyword: '', page: 1, append: false })
    }

    const onZoneSearch = (q) => {
      const keyword = String(q || '').trim()
      zoneSearchKeyword.value = keyword
      if (!form.value.cf_account_id) return
      zonePage.value = 1
      if (zoneSearchTimer) clearTimeout(zoneSearchTimer)
      zoneSearchTimer = setTimeout(() => {
        fetchZones({ keyword: zoneSearchKeyword.value, page: 1, append: false })
      }, 300)
    }

    const onZoneDropdownVisibleChange = (visible) => {
      if (!visible) return
      if (!form.value.cf_account_id) return
      if (zoneList.value.length > 0) return
      zonePage.value = 1
      fetchZones({ keyword: zoneSearchKeyword.value, page: 1, append: false })
    }

    const loadMoreZones = () => {
      if (zoneLoading.value || zoneLoadingMore.value) return
      if (!zoneHasMore.value) return
      zonePage.value += 1
      fetchZones({ keyword: zoneSearchKeyword.value, page: zonePage.value, append: true })
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

    const handlePreview = (row) => {
      openAuthPreview(`/cf-workpage-sites/${row.id}/preview`)
    }

    const handleViewDeployedHtml = async (row) => {
      deployedHtmlDialogVisible.value = true
      deployedHtmlContent.value = ''
      deployedHtmlLoading.value = true
      try {
        const res = await workpageSiteApi.getDeployedIndexHtml(row.id)
        deployedHtmlContent.value = res?.html ?? ''
        if (!deployedHtmlContent.value) {
          ElMessage.warning('暂无已保存的部署文件，请先成功部署一次')
        }
      } catch (e) {
        deployedHtmlContent.value = ''
      } finally {
        deployedHtmlLoading.value = false
      }
    }

    const copyDeployedHtml = async () => {
      try {
        await navigator.clipboard.writeText(deployedHtmlContent.value)
        ElMessage.success('已复制到剪贴板')
      } catch {
        ElMessage.error('复制失败，请手动全选复制')
      }
    }

    const copyText = async (text) => {
      if (!text) return
      try {
        await navigator.clipboard.writeText(String(text))
        ElMessage.success('已复制到剪贴板')
      } catch {
        ElMessage.error('复制失败，请手动复制')
      }
    }

    const customDomainUrl = (customDomain) => {
      if (!customDomain) return ''
      const trimmed = String(customDomain).trim()
      if (!trimmed) return ''
      if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) return trimmed
      return `https://${trimmed}`
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
      handlePreview,
      handleViewDeployedHtml,
      copyDeployedHtml,
      copyText,
      customDomainUrl,
      deployedHtmlDialogVisible,
      deployedHtmlLoading,
      deployedHtmlContent,
      handleDelete,
      dialogVisible,
      editId,
      formRef,
      form,
      submitLoading,
      submit,
      resetForm,
      zoneList,
      zoneLoading,
      zoneLoadingMore,
      zoneHasMore,
      onCfAccountChange,
      onZoneSearch,
      onZoneDropdownVisibleChange,
      loadMoreZones,
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
.access-addresses {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.access-address-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 22px;
}
.copy-link-btn {
  padding: 0;
  line-height: 1;
}
.deployed-html-textarea :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.4;
}
</style>
