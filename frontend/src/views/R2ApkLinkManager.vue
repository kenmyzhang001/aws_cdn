<template>
  <div class="r2-apk-link-manager">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>下载包管理</span>
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
          placeholder="搜索文件名或文件路径"
          clearable
          @input="handleSearch"
          style="width: 100%"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <!-- APK 文件列表 - 可展开的表格 -->
      <el-table
        :data="paginatedApkList"
        v-loading="loading"
        stripe
        row-key="file_path"
        :expand-row-keys="expandedRows"
        @expand-change="handleExpandChange"
        style="width: 100%"
        class="apk-table"
      >
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="expand-content">
              <div v-loading="row.loadingUrls" style="min-height: 100px">
                <div v-if="row.urls && row.urls.length > 0">
                  <h4 style="margin-bottom: 15px; color: #303133; font-size: 14px">
                    自定义域名访问链接 (共 {{ row.urls.length }} 个)：
                  </h4>
                  <div class="url-list">
                    <div
                      v-for="(urlItem, index) in row.urls"
                      :key="index"
                      class="url-item"
                    >
                      <div class="domain-row">
                        <span class="domain-label">域名：</span>
                        <el-tag type="primary" size="default" effect="plain">
                          {{ urlItem.domain }}
                        </el-tag>
                        <el-tag
                          v-if="urlItem.isDefault"
                          type="success"
                          size="default"
                          effect="dark"
                        >
                          <el-icon style="margin-right: 4px"><Star /></el-icon>
                          默认APK
                        </el-tag>
                        <el-tag
                          v-else-if="urlItem.defaultFilePath"
                          type="info"
                          size="small"
                          effect="plain"
                        >
                          默认：{{ urlItem.defaultFilePath }}
                        </el-tag>
                      </div>
                      <div class="url-row">
                        <span class="url-label">链接：</span>
                        <div class="url-input-wrapper">
                          <el-input 
                            :value="urlItem.url" 
                            readonly 
                            class="url-input"
                            :title="urlItem.url"
                          >
                            <template #append>
                              <el-button
                                @click="copyToClipboard(urlItem.url, row, index)"
                                :type="urlItem.copied ? 'success' : 'primary'"
                              >
                                <el-icon>
                                  <component :is="urlItem.copied ? Check : DocumentCopy" />
                                </el-icon>
                                {{ urlItem.copied ? '已复制' : '复制' }}
                              </el-button>
                            </template>
                          </el-input>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div v-else-if="!row.loadingUrls">
                  <el-empty description="该存储桶未配置自定义域名" />
                </div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="file_name" label="文件名" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <el-icon><Document /></el-icon>
              <span>{{ row.file_name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="file_path" label="文件路径" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">
            <code style="font-size: 12px">{{ row.file_path }}</code>
          </template>
        </el-table-column>
        <el-table-column label="域名数量" width="120" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.urls" type="success">{{ row.urls.length }}</el-tag>
            <el-tag v-else type="info">-</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" align="center">
          <template #default="{ row }">
            <el-button
              size="small"
              type="primary"
              @click="toggleExpand(row)"
              :loading="row.loadingUrls"
            >
              {{ isExpanded(row) ? '收起' : '查看链接' }}
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
import { Search, Refresh, Document, DocumentCopy, Check, Star } from '@element-plus/icons-vue'

const loading = ref(false)
const bucketList = ref([])
const selectedBucketId = ref(null)
const searchKeyword = ref('')
const currentPage = ref(1)
const pageSize = ref(20)
const expandedRows = ref([])

// APK 文件列表
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

// 加载 APK 文件列表（不加载域名信息）
const loadApkFiles = async () => {
  if (!selectedBucketId.value) {
    ElMessage.warning('请先选择存储桶')
    return
  }

  loading.value = true
  try {
    const files = await r2Api.listApkFiles(selectedBucketId.value, '')
    apkList.value = (files || []).map((file) => ({
      ...file,
      urls: null, // 延迟加载
      loadingUrls: false,
    }))
    // 清空展开的行
    expandedRows.value = []
  } catch (error) {
    ElMessage.error('加载 APK 文件列表失败')
    console.error(error)
  } finally {
    loading.value = false
  }
}

// 加载指定文件的域名链接
const loadFileUrls = async (row) => {
  if (row.urls !== null) {
    // 已经加载过了
    return
  }

  row.loadingUrls = true
  try {
    const urls = await r2Api.getApkFileUrls(selectedBucketId.value, row.file_path)
    // URL编码处理
    row.urls = (urls || []).map((item) => {
      // 对文件路径进行正确编码
      const pathParts = row.file_path.split('/')
      const encodedParts = pathParts.map((part) => encodeURIComponent(part))
      const encodedPath = encodedParts.join('/')
      return {
        domain: item.domain,
        url: `https://${item.domain}/${encodedPath}`,
        isDefault: item.is_default || false,
        defaultFilePath: item.default_file_path || '',
        copied: false,
      }
    })
  } catch (error) {
    ElMessage.error('加载域名链接失败')
    console.error(error)
    row.urls = []
  } finally {
    row.loadingUrls = false
  }
}

// 处理展开/收起
const handleExpandChange = async (row, expandedRowsData) => {
  if (expandedRowsData.includes(row)) {
    // 展开行
    expandedRows.value = [row.file_path]
    await loadFileUrls(row)
  } else {
    // 收起行
    expandedRows.value = []
  }
}

// 切换展开状态
const toggleExpand = async (row) => {
  if (isExpanded(row)) {
    expandedRows.value = []
  } else {
    expandedRows.value = [row.file_path]
    await loadFileUrls(row)
  }
}

// 判断是否展开
const isExpanded = (row) => {
  return expandedRows.value.includes(row.file_path)
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
    return (
      item.file_name.toLowerCase().includes(keyword) ||
      item.file_path.toLowerCase().includes(keyword)
    )
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
  currentPage.value = 1
  expandedRows.value = [] // 搜索时收起所有展开的行
}

// 处理分页大小变化
const handleSizeChange = (val) => {
  pageSize.value = val
  currentPage.value = 1
  expandedRows.value = []
}

// 处理当前页变化
const handleCurrentChange = (val) => {
  currentPage.value = val
  expandedRows.value = []
}

// 复制到剪贴板
const copyToClipboard = async (text, row, index) => {
  try {
    await navigator.clipboard.writeText(text)
    // 更新复制状态
    if (row.urls && row.urls[index]) {
      row.urls[index].copied = true
      setTimeout(() => {
        row.urls[index].copied = false
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
      if (row.urls && row.urls[index]) {
        row.urls[index].copied = true
        setTimeout(() => {
          row.urls[index].copied = false
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

/* 表格样式 */
.apk-table {
  width: 100% !important;
}

.apk-table :deep(.el-table__body) {
  width: 100%;
}

.apk-table :deep(.el-table__expanded-cell) {
  padding: 0 !important;
}

/* 展开内容区域 */
.expand-content {
  padding: 20px 40px;
  width: 100%;
  box-sizing: border-box;
}

/* URL 列表容器 */
.url-list {
  display: flex;
  flex-direction: column;
  gap: 15px;
  width: 100%;
}

/* URL 项容器 */
.url-item {
  padding: 15px;
  background: #f5f7fa;
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  transition: all 0.3s ease;
  width: 100%;
  box-sizing: border-box;
}

.url-item:hover {
  background: #ecf5ff;
  border-color: #d9ecff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}

/* 域名行 */
.domain-row {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
  gap: 10px;
  flex-wrap: wrap;
}

.domain-label {
  font-size: 13px;
  color: #606266;
  font-weight: 500;
  flex-shrink: 0;
}

/* URL 行 */
.url-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  width: 100%;
}

.url-label {
  font-size: 13px;
  color: #606266;
  font-weight: 500;
  flex-shrink: 0;
  padding-top: 8px;
}

/* URL 输入框包装器 */
.url-input-wrapper {
  flex: 1;
  min-width: 0;
  width: 100%;
}

.url-input {
  width: 100%;
  font-family: 'Monaco', 'Menlo', 'Consolas', 'Courier New', monospace;
}

/* 让输入框中的文本更容易阅读 */
.url-input :deep(.el-input__wrapper) {
  width: 100%;
}

.url-input :deep(.el-input__inner) {
  font-size: 12px;
  color: #409eff;
  font-weight: 500;
  width: 100%;
  min-width: 600px;
}

/* 复制按钮样式优化 */
.url-input :deep(.el-input-group__append) {
  padding: 0;
  background-color: #fff;
}

.url-input :deep(.el-input-group__append .el-button) {
  margin: 0;
  border-left: 1px solid #dcdfe6;
  height: 100%;
  white-space: nowrap;
}

/* 默认 APK 标签样式 */
.domain-row .el-tag {
  display: inline-flex;
  align-items: center;
}
</style>
