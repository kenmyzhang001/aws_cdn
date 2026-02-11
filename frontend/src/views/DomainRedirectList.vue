<template>
  <div class="domain-redirect-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>域名 302 重定向</span>
          <el-button type="primary" @click="openCreateDialog">
            <el-icon><Plus /></el-icon>
            新增重定向
          </el-button>
        </div>
      </template>

      <!-- 筛选 -->
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
          <el-form-item>
            <el-button type="primary" @click="fetchList">
              <el-icon><Search /></el-icon>
              查询
            </el-button>
          </el-form-item>
        </el-form>
      </div>

      <el-table :data="list" v-loading="loading" stripe>
        <el-table-column prop="source_domain" label="主域名（源）" width="200" />
        <el-table-column prop="target_domain" label="目标域名" width="200" />
        <el-table-column label="保留路径" width="100">
          <template #default="{ row }">
            <el-tag :type="row.preserve_path ? 'success' : 'info'">
              {{ row.preserve_path ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="CF 账号" width="180">
          <template #default="{ row }">
            <span v-if="row.cf_account">{{ row.cf_account.email }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">
              {{ row.status === 'active' ? '已启用' : '已禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button type="success" link size="small" @click="handleEnsureDns(row)">
              创建 DNS
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
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="fetchList"
        @current-change="fetchList"
        style="margin-top: 16px; justify-content: flex-end;"
      />
    </el-card>

    <!-- 新增/编辑：仅新增时选 Zone -->
    <el-dialog
      v-model="dialogVisible"
      :title="editId ? '编辑重定向' : '新增重定向'"
      width="520px"
      @close="resetForm"
    >
      <el-form ref="formRef" :model="form" label-width="120px">
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
        <el-form-item v-if="!editId" label="主域名（源）" required>
          <el-select
            v-model="form.zone_id"
            placeholder="请先选择 CF 账号，再输入搜索主域名"
            style="width: 100%"
            filterable
            remote
            :remote-method="remoteZoneSearch"
            :loading="zoneLoading"
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
        <el-form-item v-if="!editId" label="子域名前缀">
          <el-input
            v-model="form.subdomain_prefix"
            placeholder="留空=根域名，或输入 www、app、dl 等"
            clearable
            style="width: 100%"
          />
          <div v-if="form.zone_id" style="margin-top: 4px; font-size: 12px; color: #909399">
            实际源地址：{{ computedSourceHost }}
          </div>
        </el-form-item>
        <el-form-item v-if="editId" label="主域名">
          <span>{{ form.source_domain }}</span>
        </el-form-item>
        <el-form-item label="目标域名" required>
          <el-input
            v-model="form.target_domain"
            placeholder="例如 target.com 或 https://target.com"
            clearable
          />
        </el-form-item>
        <!--el-form-item label="保留路径与参数">
          <el-switch v-model="form.preserve_path" />
          <span style="margin-left: 8px; color: #909399; font-size: 12px">
            开启后，访问 /path?q=1 会重定向到 目标域名/path?q=1
          </span>
        </el-form-item-->
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
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { cfAccountApi } from '@/api/cf_account'
import { getCFAccountZones } from '@/api/cf_zone'
import { domainRedirectApi } from '@/api/domain_redirect'

export default {
  name: 'DomainRedirectList',
  components: { Plus, Search },
  setup() {
    const loading = ref(false)
    const list = ref([])
    const cfAccountList = ref([])
    const filterCfAccountId = ref(null)
    const pagination = ref({
      page: 1,
      page_size: 10,
      total: 0
    })

    const dialogVisible = ref(false)
    const editId = ref(null)
    const formRef = ref(null)
    const submitLoading = ref(false)
    const form = ref({
      cf_account_id: null,
      zone_id: '',
      subdomain_prefix: '',
      source_domain: '',
      target_domain: '',
      preserve_path: false
    })

    // 新增时显示的实际源主机名（根域名 或 前缀.根域名）
    const computedSourceHost = computed(() => {
      const zoneName = zoneNameById(form.value.zone_id)
      if (!zoneName) return '-'
      const prefix = (form.value.subdomain_prefix || '').trim()
      return prefix ? `${prefix}.${zoneName}` : zoneName
    })

    const zoneList = ref([])
    const zoneLoading = ref(false)

    const fetchCFAccounts = async () => {
      try {
        const res = await cfAccountApi.getCFAccountList()
        cfAccountList.value = res || []
      } catch (e) {
        ElMessage.error('获取 CF 账号列表失败')
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
        const res = await domainRedirectApi.list(params)
        list.value = res?.data ?? (Array.isArray(res) ? res : [])
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

    const zoneNameById = (zoneId) => {
      const z = zoneList.value.find((x) => x.id === zoneId)
      return z ? z.name : zoneId
    }

    const onCfAccountChange = async () => {
      form.value.zone_id = ''
      form.value.subdomain_prefix = ''
      form.value.source_domain = ''
      zoneList.value = []
      if (!form.value.cf_account_id) return
      // 预加载一批 zone，用户也可在输入框输入关键词触发远程搜索
      remoteZoneSearch('')
    }

    /** 主域名远程搜索：输入时调用接口按 name 搜索，query 为空时拉取第一页 */
    const remoteZoneSearch = async (query) => {
      if (!form.value.cf_account_id) return
      const q = typeof query === 'string' ? query.trim() : ''
      zoneLoading.value = true
      try {
        const res = await getCFAccountZones(form.value.cf_account_id, 1, 50, q)
        zoneList.value = res?.zones || []
      } catch (e) {
        zoneList.value = []
      } finally {
        zoneLoading.value = false
      }
    }

    const openCreateDialog = () => {
      editId.value = null
      form.value = {
        cf_account_id: cfAccountList.value.length === 1 ? cfAccountList.value[0].id : null,
        zone_id: '',
        subdomain_prefix: '',
        source_domain: '',
        target_domain: '',
        preserve_path: false
      }
      zoneList.value = []
      if (form.value.cf_account_id) onCfAccountChange()
      dialogVisible.value = true
    }

    const openEditDialog = (row) => {
      editId.value = row.id
      form.value = {
        source_domain: row.source_domain,
        target_domain: row.target_domain,
        preserve_path: row.preserve_path
      }
      dialogVisible.value = true
    }

    const resetForm = () => {
      formRef.value?.resetFields?.()
      editId.value = null
    }

    const submit = async () => {
      if (editId.value) {
        if (!form.value.target_domain?.trim()) {
          ElMessage.warning('请填写目标域名')
          return
        }
        submitLoading.value = true
        try {
          await domainRedirectApi.update(editId.value, {
            target_domain: form.value.target_domain.trim(),
            preserve_path: form.value.preserve_path
          })
          ElMessage.success('保存成功')
          dialogVisible.value = false
          fetchList()
        } catch (e) {
          // error already shown by request
        } finally {
          submitLoading.value = false
        }
        return
      }

      if (!form.value.cf_account_id) {
        ElMessage.warning('请选择 CF 账号')
        return
      }
      if (!form.value.zone_id) {
        ElMessage.warning('请选择主域名（源）')
        return
      }
      const zoneName = zoneNameById(form.value.zone_id)
      if (!zoneName) {
        ElMessage.warning('无法解析主域名')
        return
      }
      const prefix = (form.value.subdomain_prefix || '').trim()
      const sourceDomain = prefix ? `${prefix}.${zoneName}` : zoneName
      if (!form.value.target_domain?.trim()) {
        ElMessage.warning('请填写目标域名')
        return
      }

      submitLoading.value = true
      try {
        await domainRedirectApi.create({
          cf_account_id: form.value.cf_account_id,
          zone_id: form.value.zone_id,
          source_domain: sourceDomain,
          target_domain: form.value.target_domain.trim(),
          preserve_path: form.value.preserve_path
        })
        ElMessage.success('创建成功')
        dialogVisible.value = false
        fetchList()
      } catch (e) {
        // error already shown by request
      } finally {
        submitLoading.value = false
      }
    }

    const handleEnsureDns = async (row) => {
      try {
        await domainRedirectApi.ensureDns(row.id)
        ElMessage.success('DNS 记录已创建，稍等片刻后访问 ' + row.source_domain + ' 应可解析')
        fetchList()
      } catch (e) {
        // 错误已由 request 拦截器展示，可提示检查 Token 权限（Zone DNS Edit）
      }
    }

    const handleDelete = async (row) => {
      try {
        await ElMessageBox.confirm(
          `确定要删除重定向「${row.source_domain} → ${row.target_domain}」吗？Cloudflare 上的规则也会被移除。`,
          '确认删除',
          { type: 'warning' }
        )
        await domainRedirectApi.delete(row.id)
        ElMessage.success('已删除')
        fetchList()
      } catch (e) {
        if (e === 'cancel') return
      }
    }

    onMounted(() => {
      fetchCFAccounts().then(() => fetchList())
    })

    return {
      loading,
      list,
      cfAccountList,
      filterCfAccountId,
      fetchList,
      openCreateDialog,
      openEditDialog,
      handleEnsureDns,
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
      onCfAccountChange,
      remoteZoneSearch,
      zoneNameById
    }
  }
}
</script>

<style scoped>
.domain-redirect-list {
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
