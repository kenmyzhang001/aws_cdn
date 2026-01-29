<template>
  <div class="cf-zone-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>Cloudflare 域名管理</span>
        </div>
      </template>

      <!-- CF 账号选择 -->
      <div class="filter-section">
        <el-form :inline="true">
          <el-form-item label="CF账号">
            <el-select
              v-model="selectedCFAccountId"
              placeholder="请选择 CF 账号"
              style="width: 300px"
              @change="handleAccountChange"
              clearable
            >
              <el-option
                v-for="account in cfAccountList"
                :key="account.id"
                :label="`${account.email} (${account.note || account.account_id})`"
                :value="account.id"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="域名搜索">
            <el-input
              v-model="searchName"
              placeholder="输入域名搜索"
              style="width: 200px"
              clearable
              @clear="fetchZones"
            />
          </el-form-item>

          <el-form-item>
            <el-button type="primary" @click="fetchZones" :disabled="!selectedCFAccountId">
              <el-icon><Search /></el-icon>
              搜索
            </el-button>
          </el-form-item>
        </el-form>
      </div>

      <!-- 域名列表 -->
      <el-table :data="zoneList" v-loading="loading" stripe>
        <el-table-column prop="name" label="域名" width="250" />
        <el-table-column prop="id" label="Zone ID" width="250" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            <el-tag>{{ row.type || 'full' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="名称服务器" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.name_servers && row.name_servers.length">
              {{ row.name_servers.join(', ') }}
            </span>
            <span v-else style="color: #909399">-</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_on) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button
              size="small"
              type="primary"
              @click="handleSetAPKRule(row)"
              :loading="row._settingRule"
            >
              <el-icon><Setting /></el-icon>
              设置APK规则
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 30, 50]"
          :total="totalCount"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 规则设置结果对话框 -->
    <el-dialog
      v-model="showResultDialog"
      title="APK 安全规则设置结果"
      width="600px"
    >
      <div v-if="ruleSetResult">
        <el-alert
          :type="ruleSetResult.stats.failed_count === 0 ? 'success' : 'warning'"
          :title="ruleSetResult.message"
          :closable="false"
          style="margin-bottom: 20px"
        />

        <el-descriptions border :column="1">
          <el-descriptions-item label="成功数量">
            <el-tag type="success">{{ ruleSetResult.stats.success_count }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="失败数量">
            <el-tag :type="ruleSetResult.stats.failed_count > 0 ? 'danger' : 'info'">
              {{ ruleSetResult.stats.failed_count }}
            </el-tag>
          </el-descriptions-item>
        </el-descriptions>

        <div style="margin-top: 20px">
          <h4>规则详情：</h4>
          <el-table :data="ruleSetResult.results" border stripe>
            <el-table-column prop="rule_name" label="规则名称" width="180" />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 'success' ? 'success' : 'danger'">
                  {{ row.status === 'success' ? '成功' : '失败' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="rule_id" label="规则ID" width="150" show-overflow-tooltip />
            <el-table-column prop="message" label="消息" show-overflow-tooltip />
          </el-table>
        </div>
      </div>

      <template #footer>
        <el-button type="primary" @click="showResultDialog = false">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script>
import { ref, onMounted, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, Setting } from '@element-plus/icons-vue'
import { cfAccountApi } from '@/api/cf_account'
import { getCFAccountZones, setZoneAPKSecurityRule } from '@/api/cf_zone'

export default {
  name: 'CfZoneList',
  components: {
    Search,
    Setting
  },
  setup() {
    const loading = ref(false)
    const cfAccountList = ref([])
    const selectedCFAccountId = ref(null)
    const searchName = ref('')
    
    const zoneList = ref([])
    const currentPage = ref(1)
    const pageSize = ref(20)
    const totalCount = ref(0)
    const totalPages = ref(0)

    const showResultDialog = ref(false)
    const ruleSetResult = ref(null)

    // 获取 CF 账号列表
    const fetchCFAccounts = async () => {
      try {
        const res = await cfAccountApi.getCFAccountList()
        cfAccountList.value = res || []
        
        // 如果只有一个账号，自动选中
        if (cfAccountList.value.length === 1) {
          selectedCFAccountId.value = cfAccountList.value[0].id
          fetchZones()
        }
      } catch (error) {
        console.error('获取 CF 账号列表失败:', error)
        ElMessage.error('获取 CF 账号列表失败')
      }
    }

    // 获取域名列表
    const fetchZones = async () => {
      if (!selectedCFAccountId.value) {
        ElMessage.warning('请先选择 CF 账号')
        return
      }

      loading.value = true
      try {
        const res = await getCFAccountZones(
          selectedCFAccountId.value,
          currentPage.value,
          pageSize.value,
          searchName.value
        )
        
        if (res) {
          zoneList.value = res.zones || []
          currentPage.value = res.page || 1
          pageSize.value = res.per_page || 20
          totalPages.value = res.total_pages || 0
          totalCount.value = res.total_count || 0
        }
      } catch (error) {
        console.error('获取域名列表失败:', error)
        ElMessage.error(error.response?.data?.error || '获取域名列表失败')
      } finally {
        loading.value = false
      }
    }

    // 账号切换
    const handleAccountChange = () => {
      currentPage.value = 1
      searchName.value = ''
      if (selectedCFAccountId.value) {
        fetchZones()
      } else {
        zoneList.value = []
        totalCount.value = 0
      }
    }

    // 分页变化
    const handlePageChange = (page) => {
      currentPage.value = page
      fetchZones()
    }

    const handleSizeChange = (size) => {
      pageSize.value = size
      currentPage.value = 1
      fetchZones()
    }

    // 设置 APK 安全规则
    const handleSetAPKRule = async (zone) => {
      try {
        await ElMessageBox.confirm(
          `确定要为域名 ${zone.name} 设置 APK 安全放行规则吗？<br><br>
          <strong>将会设置以下规则：</strong><br>
          1. <strong>WAF VIP下载规则</strong>：对所有 APK/OBB 下载请求跳过所有防火墙检查（最高优先级）<br>
          2. <strong>WAF安全规则</strong>：对威胁评分≤50的 APK 请求豁免限速和机器人检测<br><br>
          <span style="color: #E6A23C;">注意：规则将应用于该域名及其所有子域名</span>`,
          '确认设置',
          {
            confirmButtonText: '确定',
            cancelButtonText: '取消',
            type: 'warning',
            dangerouslyUseHTMLString: true
          }
        )

        // 设置 loading 状态
        zone._settingRule = true
        
        const res = await setZoneAPKSecurityRule(
          selectedCFAccountId.value,
          zone.id,
          zone.name
        )

        zone._settingRule = false

        // 显示结果对话框
        ruleSetResult.value = res
        showResultDialog.value = true

        ElMessage.success('APK 安全规则设置完成')
      } catch (error) {
        zone._settingRule = false
        
        if (error === 'cancel') {
          return
        }
        
        console.error('设置 APK 规则失败:', error)
        ElMessage.error(error.response?.data?.error || '设置 APK 规则失败')
      }
    }

    // 格式化日期
    const formatDate = (dateStr) => {
      if (!dateStr) return '-'
      const date = new Date(dateStr)
      return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
      })
    }

    onMounted(() => {
      fetchCFAccounts()
    })

    return {
      loading,
      cfAccountList,
      selectedCFAccountId,
      searchName,
      zoneList,
      currentPage,
      pageSize,
      totalCount,
      totalPages,
      showResultDialog,
      ruleSetResult,
      fetchCFAccounts,
      fetchZones,
      handleAccountChange,
      handlePageChange,
      handleSizeChange,
      handleSetAPKRule,
      formatDate
    }
  }
}
</script>

<style scoped>
.cf-zone-list {
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

.pagination-container {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}
</style>
