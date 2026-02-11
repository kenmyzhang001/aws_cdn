<template>
  <div class="fallback-rule-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>兜底规则管理</span>
          <el-button type="primary" @click="openAddDialog">
            <el-icon><Plus /></el-icon>
            新增规则
          </el-button>
        </div>
      </template>

      <div style="margin-bottom: 16px; display: flex; gap: 12px; flex-wrap: wrap;">
        <el-input v-model="filters.channel_code" placeholder="渠道" clearable style="width: 160px" @clear="loadList" />
        <el-select v-model="filters.rule_type" placeholder="规则类型" clearable style="width: 180px" @change="loadList">
          <el-option label="昨日同时段对比" value="yesterday_same_period" />
          <el-option label="指定时刻目标" value="fixed_time_target" />
          <el-option label="每小时/到点累计" value="hourly_increment" />
        </el-select>
        <el-select v-model="filters.enabled" placeholder="启用状态" clearable style="width: 120px" @change="loadList">
          <el-option label="启用" :value="true" />
          <el-option label="禁用" :value="false" />
        </el-select>
        <el-button type="primary" @click="loadList">查询</el-button>
      </div>

      <el-table v-loading="loading" :data="list" stripe border>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="channel_code" label="渠道" width="140" />
        <el-table-column prop="name" label="规则名称" min-width="160" />
        <el-table-column prop="rule_type" label="类型" width="180">
          <template #default="{ row }">
            <el-tag v-if="row.rule_type === 'yesterday_same_period'" size="small">昨日同时段对比</el-tag>
            <el-tag v-else-if="row.rule_type === 'fixed_time_target'" size="small" type="success">指定时刻目标</el-tag>
            <el-tag v-else-if="row.rule_type === 'hourly_increment'" size="small" type="warning">每小时/到点累计</el-tag>
            <span v-else>{{ row.rule_type }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="params_json" label="参数" min-width="200">
          <template #default="{ row }">
            <code style="font-size: 12px;">{{ row.params_json }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="enabled" label="启用" width="80">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="openEditDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="total > 0" style="margin-top: 16px; display: flex; justify-content: flex-end">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          layout="total, sizes, prev, pager, next"
          @size-change="loadList"
          @current-change="loadList"
        />
      </div>
    </el-card>

    <el-dialog v-model="showDialog" :title="isEdit ? '编辑规则' : '新增规则'" width="560px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="渠道" required>
          <el-select
            v-model="form.channel_code"
            placeholder="请选择渠道（可搜索）"
            filterable
            style="width: 100%"
          >
            <el-option v-for="ch in channelList" :key="ch" :label="ch" :value="ch" />
          </el-select>
        </el-form-item>
        <el-form-item label="规则名称" required>
          <el-input v-model="form.name" placeholder="例如：10点前应达100注册" />
        </el-form-item>
        <el-form-item label="规则类型" required>
          <el-select v-model="form.rule_type" placeholder="请选择" style="width: 100%" @change="onRuleTypeChange">
            <el-option label="昨日同时段对比（少 N 即告警）" value="yesterday_same_period" />
            <el-option label="指定时刻目标（如 9/10/11 点应达到）" value="fixed_time_target" />
            <el-option label="每小时/到点累计（从某时到某时累计应达到）" value="hourly_increment" />
          </el-select>
        </el-form-item>
        <el-form-item label="参数 JSON" required>
          <el-input
            v-model="form.params_json"
            type="textarea"
            :rows="4"
            :placeholder="paramsPlaceholder"
          />
          <div class="form-tip">{{ paramsTip }}</div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" @click="submit" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  getFallbackRules,
  createFallbackRule,
  updateFallbackRule,
  deleteFallbackRule,
} from '@/api/fallbackRule'
import { gameStatsApi } from '@/api/gameStats'

const loading = ref(false)
const submitting = ref(false)
const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const channelList = ref([])
const showDialog = ref(false)
const isEdit = ref(false)

const filters = reactive({
  channel_code: '',
  rule_type: '',
  enabled: null,
})

const form = reactive({
  id: null,
  channel_code: '',
  name: '',
  rule_type: 'fixed_time_target',
  params_json: '',
  enabled: true,
})

const paramsPlaceholder = computed(() => {
  if (form.rule_type === 'yesterday_same_period') return '{"max_drop": 10}'
  if (form.rule_type === 'fixed_time_target') return '{"target_hour": 10, "target_reg_count": 100}'
  if (form.rule_type === 'hourly_increment') return '{"start_hour": 0, "target_hour": 10, "target_reg_count": 100}'
  return '{}'
})

const paramsTip = computed(() => {
  if (form.rule_type === 'yesterday_same_period') return 'max_drop: 允许比昨天少的上限，超过则告警'
  if (form.rule_type === 'fixed_time_target') return 'target_hour: 0-23；target_reg_count: 到该时刻累计注册数应达到'
  if (form.rule_type === 'hourly_increment') return 'start_hour/target_hour: 0-23；target_reg_count: 到 target_hour 时累计应达到'
  return ''
})

function onRuleTypeChange() {
  form.params_json = paramsPlaceholder.value
}

async function loadChannels() {
  try {
    const res = await gameStatsApi.getFullChannelNames()
    channelList.value = res?.data || []
  } catch (e) {
    channelList.value = []
  }
}

async function loadList() {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (filters.channel_code) params.channel_code = filters.channel_code
    if (filters.rule_type) params.rule_type = filters.rule_type
    if (filters.enabled !== null && filters.enabled !== '') params.enabled = filters.enabled
    const res = await getFallbackRules(params)
    list.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    ElMessage.error('加载列表失败: ' + (e.response?.data?.error || e.message))
  } finally {
    loading.value = false
  }
}

function openAddDialog() {
  isEdit.value = false
  Object.assign(form, {
    id: null,
    channel_code: '',
    name: '',
    rule_type: 'fixed_time_target',
    params_json: '{"target_hour": 10, "target_reg_count": 100}',
    enabled: true,
  })
  showDialog.value = true
}

function openEditDialog(row) {
  isEdit.value = true
  Object.assign(form, {
    id: row.id,
    channel_code: row.channel_code,
    name: row.name,
    rule_type: row.rule_type,
    params_json: row.params_json,
    enabled: row.enabled,
  })
  showDialog.value = true
}

async function submit() {
  if (!form.channel_code || !form.name || !form.rule_type || !form.params_json) {
    ElMessage.warning('请填写渠道、规则名称、类型和参数')
    return
  }
  try {
    JSON.parse(form.params_json)
  } catch (e) {
    ElMessage.warning('参数必须是合法 JSON')
    return
  }
  submitting.value = true
  try {
    if (isEdit.value) {
      await updateFallbackRule(form.id, {
        channel_code: form.channel_code,
        name: form.name,
        rule_type: form.rule_type,
        params_json: form.params_json,
        enabled: form.enabled,
      })
      ElMessage.success('更新成功')
    } else {
      await createFallbackRule({
        channel_code: form.channel_code,
        name: form.name,
        rule_type: form.rule_type,
        params_json: form.params_json,
        enabled: form.enabled,
      })
      ElMessage.success('创建成功')
    }
    showDialog.value = false
    await loadList()
  } catch (e) {
    ElMessage.error((e.response?.data?.error || e.message) + '')
  } finally {
    submitting.value = false
  }
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm('确定删除该规则？', '提示', { type: 'warning' })
    await deleteFallbackRule(row.id)
    ElMessage.success('删除成功')
    await loadList()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e.response?.data?.error || e.message)
  }
}

onMounted(() => {
  loadChannels()
  loadList()
})
</script>

<style scoped>
.fallback-rule-list {
  padding: 20px;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.form-tip {
  margin-top: 4px;
  font-size: 12px;
  color: #909399;
}
</style>
