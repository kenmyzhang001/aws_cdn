<template>
  <div class="game-channel-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>渠道名称</span>
          <el-button @click="loadList" :loading="loading">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
      </template>

      <div class="tip">
        数据来源：Redis 集合 <code>game_stats:full_channel_names</code>，共 {{ total }} 个渠道。
      </div>

      <el-table :data="list" v-loading="loading" stripe max-height="560">
        <el-table-column type="index" label="#" width="60" />
        <el-table-column prop="name" label="渠道名称" min-width="200">
          <template #default="{ row }">{{ row }}</template>
        </el-table-column>
      </el-table>
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

onMounted(() => {
  loadList()
})

const loadList = async () => {
  loading.value = true
  try {
    const res = await gameStatsApi.getFullChannelNames()
    list.value = res.data || []
    total.value = res.total ?? list.value.length
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '加载渠道列表失败')
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.game-channel-list .card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.game-channel-list .tip {
  margin-bottom: 16px;
  color: #606266;
  font-size: 13px;
}
.game-channel-list .tip code {
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}
</style>
