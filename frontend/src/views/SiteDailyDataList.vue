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

      <!-- 分组数据统计（与后端渠道分组接口同步，无数据时也可配置分组） -->
      <div class="group-section">
        <div class="group-section-header">
          <span class="group-section-title">渠道分组统计</span>
          <el-button type="primary" plain size="small" :loading="groupsLoading" @click="addGroup">
            <el-icon><Plus /></el-icon>
            新增组
          </el-button>
        </div>
        <p class="group-tip">从下方站点日数据中的渠道选择加入各组，可查看组内渠道明细及组汇总。分组会保存到服务器，与其它「分组」功能区分。</p>
        <div class="group-list">
          <el-card v-for="g in groups" :key="g.id" class="group-card" shadow="never">
            <template #header>
              <div class="group-card-header">
                <el-input
                  v-model="g.name"
                  placeholder="组名称"
                  size="small"
                  style="width: 160px"
                  maxlength="20"
                  show-word-limit
                  @blur="saveGroup(g)"
                />
                <el-select
                  v-model="g.channelCodes"
                  multiple
                  collapse-tags
                  collapse-tags-tooltip
                  placeholder="选择渠道"
                  size="small"
                  style="width: 280px"
                  :title="`已选 ${g.channelCodes.length} 个渠道`"
                  @change="saveGroup(g)"
                >
                  <el-option
                    v-for="ch in availableChannels"
                    :key="ch.channelCode"
                    :label="`${ch.channelName} (${ch.channelCode})`"
                    :value="ch.channelCode"
                  />
                </el-select>
                <el-button type="danger" plain size="small" @click="removeGroup(g.id)">
                  <el-icon><Delete /></el-icon>
                  删除组
                </el-button>
              </div>
            </template>
            <div class="group-table-wrap">
              <template v-if="getGroupChannelRows(g.id).length > 0">
                <el-table :data="getGroupChannelRows(g.id)" size="small" border stripe>
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
                </el-table>
                <div class="group-summary" v-if="groupSummaries[g.id]">
                  <span>组汇总：</span>
                  <span>注册 {{ groupSummaries[g.id].totalReg }}</span>
                  <span>首充 {{ groupSummaries[g.id].totalFirstPay }}</span>
                  <span>充值 {{ formatMoney(groupSummaries[g.id].totalPay) }}</span>
                  <span>提款 {{ formatMoney(groupSummaries[g.id].totalWithdraw) }}</span>
                  <span class="net">充提差 {{ formatMoney(groupSummaries[g.id].totalNet) }}</span>
                </div>
              </template>
              <div v-else class="no-group-data">请在该组中添加渠道，或先执行查询加载数据</div>
            </div>
          </el-card>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { gameStatsApi } from '@/api/gameStats'
import { ElMessage } from 'element-plus'
import { Refresh, Plus, Delete } from '@element-plus/icons-vue'

const loading = ref(false)
const list = ref([])
const total = ref(0)

// 分组统计（与后端渠道分组接口同步）
const groups = ref([])
const groupsLoading = ref(false)

/** 将接口返回的渠道分组转为前端使用的结构 */
function mapChannelGroupFromApi(g) {
  return {
    id: g.id,
    name: g.name,
    channelCodes: Array.isArray(g.channel_codes) ? g.channel_codes : [],
  }
}

/** 从接口加载渠道分组列表 */
async function loadChannelGroups() {
  groupsLoading.value = true
  try {
    const res = await gameStatsApi.getChannelGroups()
    const data = res.data || res
    const arr = Array.isArray(data) ? data : []
    groups.value = arr.map(mapChannelGroupFromApi)
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '加载渠道分组失败')
    groups.value = []
  } finally {
    groupsLoading.value = false
  }
}

/** 从当前列表中收集所有不重复的渠道（用于分组选择） */
const availableChannels = computed(() => {
  const map = new Map()
  for (const site of list.value) {
    if (!site.stats || !Array.isArray(site.stats)) continue
    for (const s of site.stats) {
      if (s.channelCode && !map.has(s.channelCode)) {
        map.set(s.channelCode, { channelCode: s.channelCode, channelName: s.channelName || s.channelCode })
      }
    }
  }
  return Array.from(map.values()).sort((a, b) => (a.channelName || '').localeCompare(b.channelName || ''))
})

/** 新增组（调用接口创建并加入列表） */
async function addGroup() {
  try {
    const res = await gameStatsApi.createChannelGroup({
      name: `组${groups.value.length + 1}`,
      channel_codes: [],
    })
    const g = res.data || res
    groups.value.push(mapChannelGroupFromApi(g))
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '创建渠道分组失败')
  }
}

/** 删除组（调用接口并更新列表） */
async function removeGroup(id) {
  try {
    await gameStatsApi.deleteChannelGroup(id)
    groups.value = groups.value.filter((g) => g.id !== id)
    ElMessage.success('已删除')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '删除渠道分组失败')
  }
}

/** 保存组（名称或渠道变更时调用接口更新） */
async function saveGroup(g) {
  if (g.id == null) return
  try {
    const res = await gameStatsApi.updateChannelGroup(g.id, {
      name: g.name,
      channel_codes: g.channelCodes || [],
    })
    const updated = res.data || res
    const idx = groups.value.findIndex((x) => x.id === g.id)
    if (idx !== -1) groups.value[idx] = mapChannelGroupFromApi(updated)
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存渠道分组失败')
  }
}

/** 将某条 stat 的数值字段转为可累加的数字 */
function num(v) {
  if (v == null || v === '') return 0
  const n = Number(v)
  return Number.isNaN(n) ? 0 : n
}

/** 获取指定组内的渠道聚合数据（按渠道汇总各站点数据） */
function getGroupChannelRows(groupId) {
  const g = groups.value.find((x) => x.id === groupId)
  if (!g || !g.channelCodes || g.channelCodes.length === 0) return []
  const codeSet = new Set(g.channelCodes)
  const byChannel = new Map()
  for (const site of list.value) {
    if (!site.stats || !Array.isArray(site.stats)) continue
    for (const s of site.stats) {
      if (!codeSet.has(s.channelCode)) continue
      const key = s.channelCode
      if (!byChannel.has(key)) {
        byChannel.set(key, {
          channelCode: s.channelCode,
          channelName: s.channelName || s.channelCode,
          regCount: 0,
          firstPayCount: 0,
          chargeUserCount: 0,
          cashUserCount: 0,
          payAmount: 0,
          withdrawAmount: 0,
          netAmount: 0,
        })
      }
      const row = byChannel.get(key)
      row.regCount += num(s.regCount)
      row.firstPayCount += num(s.firstPayCount)
      row.chargeUserCount += num(s.chargeUserCount)
      row.cashUserCount += num(s.cashUserCount)
      row.payAmount += num(s.payAmount)
      row.withdrawAmount += num(s.withdrawAmount)
      row.netAmount += num(s.netAmount)
    }
  }
  return Array.from(byChannel.values()).sort((a, b) => (a.channelName || '').localeCompare(b.channelName || ''))
}

/** 获取指定组的汇总 */
function getGroupSummary(groupId) {
  const rows = getGroupChannelRows(groupId)
  if (rows.length === 0) return null
  return {
    totalReg: rows.reduce((sum, r) => sum + num(r.regCount), 0),
    totalFirstPay: rows.reduce((sum, r) => sum + num(r.firstPayCount), 0),
    totalPay: rows.reduce((sum, r) => sum + num(r.payAmount), 0),
    totalWithdraw: rows.reduce((sum, r) => sum + num(r.withdrawAmount), 0),
    totalNet: rows.reduce((sum, r) => sum + num(r.netAmount), 0),
  }
}

/** 各组汇总（用于模板避免重复计算） */
const groupSummaries = computed(() => {
  const out = {}
  for (const g of groups.value) {
    const s = getGroupSummary(g.id)
    if (s) out[g.id] = s
  }
  return out
})

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
  loadChannelGroups()
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

/* 分组统计 */
.group-section {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid #ebeef5;
}
.group-section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.group-section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}
.group-tip {
  color: #909399;
  font-size: 12px;
  margin-bottom: 16px;
}
.group-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.group-card {
  border: 1px solid #ebeef5;
}
.group-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.group-table-wrap {
  margin-top: 0;
}
.group-summary {
  margin-top: 12px;
  padding: 10px 12px;
  background: #f0f9ff;
  border-radius: 4px;
  font-size: 13px;
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: center;
}
.group-summary .net {
  font-weight: 600;
  color: #409eff;
}
.no-group-data {
  color: #909399;
  font-size: 13px;
  padding: 16px;
  text-align: center;
}
</style>
