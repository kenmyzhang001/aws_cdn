<template>
  <div class="site-daily-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>站点日数据</span>
          <el-button @click="loadList" :loading="loading">
            <el-icon><Refresh /></el-icon>
            查询
          </el-button>
        </div>
      </template>

      <el-form :inline="true" :model="filters" class="filter-form">
        <el-form-item label="时区">
          <el-select v-model="filters.timezone" placeholder="时区" style="width: 130px" @change="onTimezoneChange">
            <el-option label="缅甸时间" value="Asia/Yangon" />
            <el-option label="北京时间" value="Asia/Shanghai" />
          </el-select>
        </el-form-item>
        <el-form-item label="日期">
          <el-date-picker
            v-model="filters.date"
            type="date"
            placeholder="选择日期"
            value-format="YYYY-MM-DD"
            style="width: 160px"
          />
        </el-form-item>
        <el-form-item label="小时">
          <el-select v-model="filters.hour" placeholder="小时" style="width: 90px">
            <el-option
              v-for="h in 24"
              :key="h - 1"
              :label="String(h - 1).padStart(2, '0') + ' 时'"
              :value="h - 1"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleQuery">查询</el-button>
          <el-button @click="resetToNow">当前小时</el-button>
        </el-form-item>
      </el-form>

      <div class="tip">
        <span>查询时区：{{ filters.timezone === 'Asia/Yangon' ? '缅甸' : '北京' }}时间</span>
        · 数据来源：Redis 缓存 <code>allSitesData_{{ filters.date }}_{{ String(filters.hour).padStart(2, '0') }}</code>
        <span v-if="total > 0">，共 {{ total }} 个站点。</span>
        <span v-else>（该时间点暂无缓存数据）</span>
      </div>

      <template v-if="list.length > 0">
        <el-table :data="list" v-loading="loading" stripe row-key="siteName" style="width: 100%">
          <el-table-column type="expand">
            <template #default="{ row }">
              <div class="expand-content">
                <div v-if="row.stats && row.stats.length" class="channel-table">
                  <div class="expand-title">渠道明细</div>
                  <el-table :data="row.stats" size="small" border>
                    <el-table-column prop="channelCode" label="渠道编码" width="120" />
                    <el-table-column prop="channelName" label="渠道名称" min-width="140" />
                    <el-table-column prop="regCount" label="注册数" width="90" align="right" />
                    <el-table-column prop="firstPayCount" label="首充数" width="90" align="right" />
                    <el-table-column prop="chargeUserCount" label="充值人数" width="100" align="right" />
                    <el-table-column prop="cashUserCount" label="提现人数" width="100" align="right" />
                    <el-table-column prop="payAmount" label="充值金额" width="110" align="right">
                      <template #default="{ row: r }">{{ formatMoney(r.payAmount) }}</template>
                    </el-table-column>
                    <el-table-column prop="withdrawAmount" label="提款金额" width="110" align="right">
                      <template #default="{ row: r }">{{ formatMoney(r.withdrawAmount) }}</template>
                    </el-table-column>
                    <el-table-column prop="netAmount" label="充提差" width="110" align="right">
                      <template #default="{ row: r }">{{ formatMoney(r.netAmount) }}</template>
                    </el-table-column>
                    <el-table-column prop="date" label="日期" width="110" />
                  </el-table>
                </div>
                <div v-else class="no-stats">暂无渠道明细</div>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="siteName" label="站点" min-width="120" />
          <el-table-column label="汇总" align="right">
            <template #default="{ row }">
              <template v-if="row.summary">
                <div class="summary-row">
                  <span>注册: {{ row.summary.totalReg }}</span>
                  <span>首充: {{ row.summary.totalFirstPay }}</span>
                  <span>充值: {{ formatMoney(row.summary.totalPay) }}</span>
                  <span>提款: {{ formatMoney(row.summary.totalWithdraw) }}</span>
                  <span class="net">充提差: {{ formatMoney(row.summary.totalNet) }}</span>
                </div>
              </template>
              <span v-else>-</span>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else-if="!loading" description="暂无数据" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { gameStatsApi } from '@/api/gameStats'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'

const loading = ref(false)
const list = ref([])
const total = ref(0)

/** 获取指定时区下的当前日期与小时（用于查询条件） */
function getNowInTimezone(tz) {
  const now = new Date()
  const s = now.toLocaleString('en-CA', {
    timeZone: tz,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  const [datePart, timePart] = s.split(', ')
  const [year, month, day] = datePart.split('-')
  const hour = parseInt(timePart.split(':')[0], 10)
  return {
    date: `${year}-${month}-${day}`,
    hour,
  }
}

const defaultTimezone = 'Asia/Yangon' // 默认缅甸时间

const filters = ref({
  timezone: defaultTimezone,
  date: getNowInTimezone(defaultTimezone).date,
  hour: getNowInTimezone(defaultTimezone).hour,
})

function onTimezoneChange() {
  const n = getNowInTimezone(filters.value.timezone)
  filters.value.date = n.date
  filters.value.hour = n.hour
}

onMounted(() => {
  loadList()
})

const loadList = async () => {
  loading.value = true
  try {
    const res = await gameStatsApi.getSiteDailyData({
      date: filters.value.date,
      hour: filters.value.hour,
    })
    list.value = res.data || []
    total.value = res.total ?? list.value.length
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '加载站点日数据失败')
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

const handleQuery = () => {
  loadList()
}

const resetToNow = () => {
  const n = getNowInTimezone(filters.value.timezone)
  filters.value.date = n.date
  filters.value.hour = n.hour
  loadList()
}

const formatMoney = (v) => {
  if (v == null || v === '') return '-'
  const n = Number(v)
  if (Number.isNaN(n)) return '-'
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
</script>

<style scoped>
.site-daily-list .card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.site-daily-list .filter-form {
  margin-bottom: 12px;
}
.site-daily-list .tip {
  margin-bottom: 16px;
  color: #606266;
  font-size: 13px;
}
.site-daily-list .tip code {
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}
.expand-content {
  padding: 12px 24px;
  background: #fafafa;
}
.expand-title {
  margin-bottom: 8px;
  font-weight: 600;
  color: #303133;
}
.channel-table {
  max-width: 100%;
  overflow-x: auto;
}
.no-stats {
  color: #909399;
  font-size: 13px;
}
.summary-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 13px;
}
.summary-row .net {
  font-weight: 600;
  color: #409eff;
}
</style>
