<template>
  <div class="r2-apk-link-manager">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>APK 链接管理</span>
          <div style="display: flex; gap: 10px; align-items: center">
            <el-select
              v-model="selectedBucketId"
              placeholder="选择存储桶"
              style="width: 200px"
              @change="handleBucketChange"
            >
              <el-option
                v-for="bucket in bucketList"
                :key="bucket.id"
                :label="bucket.bucket_name"
                :value="bucket.id"
              />
            </el-select>
            <el-button @click="loadApkFiles" :loading="loading">
              <el-icon><Refresh /></el-icon>
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <!-- 搜索栏 -->
      <div style="margin-bottom: 20px">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索文件名、域名或完整链接"
          clearable
          @input="handleSearch"
          style="width: 100%"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <!-- APK 文件列表 -->
      <el-table :data="paginatedApkList" v-loading="loading" stripe>
        <el-table-column prop="fileName" label="文件名" min-width="250" show-overflow-tooltip>
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <el-icon><Document /></el-icon>
              <span>{{ row.fileName }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="domain" label="域名" width="200" show-overflow-tooltip />
        <el-table-column label="访问链接" min-width="400">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <el-input
                :value="row.fullUrl"
                readonly
                style="flex: 1"
                ref="urlInputRef"
              >
                <template #append>
                  <el-button
                    @click="copyToClipboard(row.fullUrl)"
                    :icon="row.copied ? 'Check' : 'DocumentCopy'"
                    :type="row.copied ? 'success' : 'primary'"
                  >
                    {{ row.copied ? '已复制' : '复制' }}
                  </el-button>
                </template>
              </el-input>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="filePath" label="文件路径" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <code style="font-size: 12px">{{ row.filePath }}</code>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button
              size="small"
              type="primary"
              @click="copyToClipboard(row.fullUrl)"
              :icon="row.copied ? 'Check' : 'DocumentCopy'"
            >
              {{ row.copied ? '已复制' : '复制' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div style="margin-top: 20px; display: flex; justify-content: flex-end">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="filteredApkList.length"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { r2Api } from '@/api/r2'
import { ElMessage } from 'element-plus'
import { Search, Refresh, Document, DocumentCopy, Check } from '@element-plus/icons-vue'

const loading = ref(false)
const bucketList = ref([])
const selectedBucketId = ref(null)
const searchKeyword = ref('')
const currentPage = ref(1)
const pageSize = ref(20)

// APK 文件列表（包含完整链接信息）
const apkList = ref([])

// 加载存储桶列表
const loadBucketList = async () => {
  try {
    const res = await r2Api.getR2BucketList()
    bucketList.value = res || []
    if (bucketList.value.length > 0 && !selectedBucketId.value) {
      selectedBucketId.value = bucketList.value[0].id
      loadApkFiles()
    }
  } catch (error) {
    ElMessage.error('加载存储桶列表失败')
  }
}

// 加载 APK 文件
const loadApkFiles = async () => {
  if (!selectedBucketId.value) {
    ElMessage.warning('请先选择存储桶')
    return
  }

  loading.value = true
  try {
    // 1. 获取存储桶的自定义域名列表
    const domainRes = await r2Api.getR2CustomDomainList(selectedBucketId.value)
    const domains = domainRes || []

    if (domains.length === 0) {
      ElMessage.warning('该存储桶未配置自定义域名，无法生成访问链接')
      apkList.value = []
      return
    }

    // 2. 获取文件列表
    const filesRes = await r2Api.listFiles(selectedBucketId.value, '')
    // 后端返回的是数组，不是对象
    const files = Array.isArray(filesRes) ? filesRes : (filesRes.files || [])

    // 3. 过滤出 APK 文件
    const apkFiles = files.filter((file) => {
      // 排除目录（以 / 结尾的）
      if (file.endsWith('/')) return false
      // 检查是否是 APK 文件
      return file.toLowerCase().endsWith('.apk')
    })

    // 4. 为每个 APK 文件生成访问链接
    const apkListWithUrls = []
    for (const file of apkFiles) {
      // 为每个域名生成一个链接
      for (const domain of domains) {
        // 构建完整 URL
        // 文件路径需要正确编码：路径分隔符 / 保持不变，其他特殊字符需要编码
        const pathParts = file.split('/')
        const encodedParts = pathParts.map((part) => encodeURIComponent(part))
        const encodedPath = encodedParts.join('/')
        const fullUrl = `https://${domain.domain}/${encodedPath}`

        apkListWithUrls.push({
          fileName: file.split('/').pop(), // 文件名
          filePath: file, // 完整路径
          domain: domain.domain,
          fullUrl: fullUrl,
          copied: false, // 是否已复制
        })
      }
    }

    apkList.value = apkListWithUrls
  } catch (error) {
    ElMessage.error('加载 APK 文件列表失败')
    console.error(error)
  } finally {
    loading.value = false
  }
}

// 处理存储桶切换
const handleBucketChange = () => {
  currentPage.value = 1
  searchKeyword.value = ''
  loadApkFiles()
}

// 搜索过滤
const filteredApkList = computed(() => {
  if (!searchKeyword.value) {
    return apkList.value
  }

  const keyword = searchKeyword.value.toLowerCase()
  return apkList.value.filter((item) => {
    // 文件名搜索
    if (item.fileName.toLowerCase().includes(keyword)) {
      return true
    }
    // 域名搜索
    if (item.domain.toLowerCase().includes(keyword)) {
      return true
    }
    // 完整链接搜索
    if (item.fullUrl.toLowerCase().includes(keyword)) {
      return true
    }
    // 文件路径搜索
    if (item.filePath.toLowerCase().includes(keyword)) {
      return true
    }
    return false
  })
})

// 分页数据
const paginatedApkList = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredApkList.value.slice(start, end)
})

// 处理搜索
const handleSearch = () => {
  currentPage.value = 1 // 搜索时重置到第一页
}

// 处理分页大小变化
const handleSizeChange = (val) => {
  pageSize.value = val
  currentPage.value = 1
}

// 处理当前页变化
const handleCurrentChange = (val) => {
  currentPage.value = val
}

// 复制到剪贴板
const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    // 更新复制状态
    const item = apkList.value.find((item) => item.fullUrl === text)
    if (item) {
      item.copied = true
      setTimeout(() => {
        item.copied = false
      }, 2000)
    }
    ElMessage.success('链接已复制到剪贴板')
  } catch (error) {
    // 降级方案：使用传统方法
    const textArea = document.createElement('textarea')
    textArea.value = text
    textArea.style.position = 'fixed'
    textArea.style.opacity = '0'
    document.body.appendChild(textArea)
    textArea.select()
    try {
      document.execCommand('copy')
      const item = apkList.value.find((item) => item.fullUrl === text)
      if (item) {
        item.copied = true
        setTimeout(() => {
          item.copied = false
        }, 2000)
      }
      ElMessage.success('链接已复制到剪贴板')
    } catch (err) {
      ElMessage.error('复制失败，请手动复制')
    }
    document.body.removeChild(textArea)
  }
}

onMounted(() => {
  loadBucketList()
})
</script>

<style scoped>
.r2-apk-link-manager {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

code {
  background-color: #f5f5f5;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
}
</style>
