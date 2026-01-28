<template>
  <div class="cf-worker-list">
    <el-card class="box-card">
      <template #header>
        <div class="card-header">
          <span>Cloudflare Worker 管理</span>
          <el-button type="primary" @click="showCreateDialog">
            <el-icon><Plus /></el-icon>
            创建 Worker
          </el-button>
        </div>
      </template>

      <!-- 筛选区域 -->
      <el-form :inline="true" :model="searchForm" class="search-form">
        <el-form-item label="CF 账号">
          <el-select
            v-model="searchForm.cf_account_id"
            clearable
            placeholder="请选择 CF 账号"
            @change="loadWorkers"
          >
            <el-option
              v-for="account in cfAccounts"
              :key="account.id"
              :label="account.email"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadWorkers">查询</el-button>
          <el-button @click="resetSearch">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- Worker 列表 -->
      <el-table
        v-loading="loading"
        :data="workers"
        style="width: 100%"
        border
      >
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="CF 账号" width="200">
          <template #default="{ row }">
            {{ row.cf_account?.email || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="worker_name" label="Worker 名称" width="180" />
        <el-table-column label="Worker 域名" width="220">
          <template #default="{ row }">
            <el-link :href="`https://${row.worker_domain}`" target="_blank" type="primary">
              {{ row.worker_domain }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="目标域名" width="220">
          <template #default="{ row }">
            <el-link :href="`https://${row.target_domain}`" target="_blank" type="success">
              {{ row.target_domain }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">
              {{ row.status === 'active' ? '激活' : '未激活' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="150" />
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="showEditDialog(row)">
              编辑
            </el-button>
            <el-button link type="danger" size="small" @click="handleDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.page_size"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadWorkers"
        @current-change="loadWorkers"
        style="margin-top: 20px; justify-content: flex-end;"
      />
    </el-card>

    <!-- 创建/编辑 Worker 对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="600px"
      @close="handleDialogClose"
    >
      <el-form
        ref="workerFormRef"
        :model="workerForm"
        :rules="workerFormRules"
        label-width="120px"
      >
        <el-form-item label="CF 账号" prop="cf_account_id">
          <el-select
            v-model="workerForm.cf_account_id"
            placeholder="请选择 CF 账号"
            :disabled="isEdit"
            style="width: 100%"
            filterable
            @change="handleCFAccountChange"
          >
            <el-option
              v-for="account in cfAccounts"
              :key="account.id"
              :label="account.email"
              :value="account.id"
            />
          </el-select>
          <div class="form-tip" v-if="cfAccounts.length > 0">
            当前可用账号数：{{ cfAccounts.length }}
          </div>
          <div class="form-tip" v-else style="color: #F56C6C;">
            暂无可用账号，请先在「CF 账号管理」中添加账号
          </div>
        </el-form-item>

        <el-form-item label="Worker 名称" prop="worker_name" v-if="!isEdit">
          <el-input
            v-model="workerForm.worker_name"
            placeholder="请输入 Worker 名称，如：my-redirect-worker"
          />
          <div class="form-tip">Worker 脚本的名称，创建后不可修改</div>
        </el-form-item>

        <el-form-item label="Worker 域名" prop="worker_domain" v-if="!isEdit">
          <el-select
            v-model="workerForm.worker_domain"
            placeholder="请选择或输入 Worker 域名"
            style="width: 100%"
            filterable
            allow-create
            clearable
            :loading="loadingWorkerDomains"
            :disabled="!workerForm.cf_account_id"
          >
            <el-option
              v-for="domain in workerDomains"
              :key="domain"
              :label="domain"
              :value="domain"
            />
          </el-select>
          <div class="form-tip" v-if="!workerForm.cf_account_id">
            请先选择 CF 账号
          </div>
          <div class="form-tip" v-else-if="loadingWorkerDomains" style="color: #409EFF;">
            <el-icon class="is-loading"><Loading /></el-icon>
            正在加载域名列表...
          </div>
          <div class="form-tip" v-else-if="workerDomains.length > 0" style="color: #67C23A;">
            已加载 {{ workerDomains.length }} 个可用域名，可从列表选择或手动输入
          </div>
          <div class="form-tip" v-else style="color: #E6A23C;">
            该账号暂无托管域名，请手动输入域名
          </div>
        </el-form-item>

        <el-form-item label="目标域名来源">
          <el-radio-group v-model="domainInputMode" @change="handleDomainModeChange">
            <el-radio label="manual">手动输入</el-radio>
            <el-radio label="select">从 CF 下载链接选择</el-radio>
          </el-radio-group>
        </el-form-item>

        <!-- 手动输入模式 -->
        <template v-if="domainInputMode === 'manual'">
          <el-form-item label="目标域名" prop="target_domain">
            <el-input
              v-model="workerForm.target_domain"
              placeholder="请输入目标跳转域名，如：https://target.example.com"
              clearable
            />
            <div class="form-tip">
              跳转的目标地址（域名 B），支持完整 URL
            </div>
          </el-form-item>
        </template>

        <!-- 从 CF 选择模式 -->
        <template v-else-if="domainInputMode === 'select'">
          <el-form-item label="选择存储桶" required>
            <el-select
              v-model="selectedBucketId"
              placeholder="请选择 R2 存储桶"
              style="width: 100%"
              clearable
              @change="handleBucketChange"
            >
              <el-option
                v-for="bucket in r2Buckets"
                :key="bucket.id"
                :label="`${bucket.bucket_name} (${bucket.cf_account?.email || ''})`"
                :value="bucket.id"
              />
            </el-select>
            <div class="form-tip" v-if="r2Buckets.length > 0">
              当前有 {{ r2Buckets.length }} 个存储桶可选
            </div>
            <div class="form-tip" v-else style="color: #E6A23C;">
              暂无可用存储桶，请先创建 R2 存储桶
            </div>
          </el-form-item>

          <el-form-item label="下载域名" prop="target_domain" required>
            <el-select
              v-model="workerForm.target_domain"
              placeholder="请先选择存储桶"
              style="width: 100%"
              filterable
              clearable
              :disabled="!selectedBucketId"
              :loading="loadingDomains"
              @clear="handleClearDomain"
              @visible-change="handleDomainDropdownVisible"
            >
              <el-option
                v-for="(domain, index) in selectedBucketDomains"
                :key="index"
                :label="domain.url"
                :value="domain.url"
              >
                <div class="domain-option-block">
                  <div class="domain-url">{{ domain.url }}</div>
                  <div class="domain-file-info">
                    文件: {{ domain.fileName }} 
                    <el-tag size="small" style="margin-left: 8px;">{{ domain.domainName }}</el-tag>
                  </div>
                </div>
              </el-option>
              
              <!-- 加载更多提示 -->
              <el-option
                v-if="domainPagination.hasMore && selectedBucketDomains.length > 0"
                :value="null"
                disabled
              >
                <div style="text-align: center; color: #409EFF; cursor: pointer;" @click.stop="loadMoreDomains">
                  <el-icon><RefreshRight /></el-icon>
                  点击加载更多
                </div>
              </el-option>
            </el-select>
            <div class="form-tip" v-if="!selectedBucketId">
              请先选择存储桶
            </div>
            <div class="form-tip" v-else-if="loadingDomains" style="color: #409EFF;">
              <el-icon class="is-loading"><Loading /></el-icon>
              正在加载下载域名...
            </div>
            <div class="form-tip" v-else-if="selectedBucketDomains.length > 0" style="color: #67C23A;">
              已加载 {{ selectedBucketDomains.length }} 个下载链接
              <span v-if="domainPagination.hasMore">，下拉可加载更多</span>
            </div>
            <div class="form-tip" v-else style="color: #E6A23C;">
              该存储桶暂无 APK 文件或下载链接
            </div>
          </el-form-item>
        </template>

        <el-form-item label="状态" prop="status" v-if="isEdit">
          <el-select v-model="workerForm.status" style="width: 100%">
            <el-option label="激活" value="active" />
            <el-option label="未激活" value="inactive" />
          </el-select>
        </el-form-item>

        <el-form-item label="描述" prop="description">
          <el-input
            v-model="workerForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入描述信息"
          />
        </el-form-item>

        <el-alert
          title="功能说明"
          type="info"
          :closable="false"
          style="margin-bottom: 20px"
        >
          <div style="line-height: 1.8;">
            <p>• 访问 Worker 域名时，会根据用户 UA 自动选择跳转方式：</p>
            <p>• 社交/短信 WebView（Telegram/Viber/Line/WhatsApp/微信等）：使用 JavaScript 跳转</p>
            <p>• 普通浏览器（Chrome/Safari 等）：使用 302 跳转</p>
            <p>• 缓存时间：30 分钟（1800 秒）</p>
          </div>
        </el-alert>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">
          {{ isEdit ? '更新' : '创建' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Plus, RefreshRight, Loading } from '@element-plus/icons-vue';
import {
  getWorkerList,
  createWorker,
  updateWorker,
  deleteWorker
} from '@/api/cf_worker';
import { cfAccountApi } from '@/api/cf_account';
import { r2Api } from '@/api/r2';

// 搜索表单
const searchForm = reactive({
  cf_account_id: null
});

// Worker 列表
const workers = ref([]);
const loading = ref(false);

// CF 账号列表
const cfAccounts = ref([]);

// R2 存储桶列表
const r2Buckets = ref([]);

// 当前选择的存储桶对应的下载域名列表
const selectedBucketDomains = ref([]);

// 当前选择的存储桶 ID
const selectedBucketId = ref(null);

// 目标域名输入模式：manual-手动输入, select-从CF选择
const domainInputMode = ref('manual');

// 域名加载状态
const loadingDomains = ref(false);

// Worker 域名列表（从 CF 账号获取）
const workerDomains = ref([]);
const loadingWorkerDomains = ref(false);

// 分页相关
const domainPagination = reactive({
  currentPage: 0,
  pageSize: 20,
  hasMore: true
});

// 分页
const pagination = reactive({
  page: 1,
  page_size: 10,
  total: 0
});

// 对话框
const dialogVisible = ref(false);
const isEdit = ref(false);
const dialogTitle = computed(() => (isEdit.value ? '编辑 Worker' : '创建 Worker'));
const submitting = ref(false);

// Worker 表单
const workerFormRef = ref(null);
const workerForm = reactive({
  cf_account_id: null,
  worker_name: '',
  worker_domain: '',
  target_domain: '',
  status: 'active',
  description: ''
});

// 当前编辑的 Worker ID
const currentWorkerId = ref(null);

// 表单验证规则
const workerFormRules = {
  cf_account_id: [
    { required: true, message: '请选择 CF 账号', trigger: 'change' }
  ],
  worker_name: [
    { required: true, message: '请输入 Worker 名称', trigger: 'blur' },
    { 
      pattern: /^[a-z0-9-]+$/,
      message: 'Worker 名称只能包含小写字母、数字和连字符',
      trigger: 'blur'
    }
  ],
  worker_domain: [
    { required: true, message: '请输入 Worker 域名', trigger: 'blur' }
  ],
  target_domain: [
    { required: true, message: '请输入目标域名', trigger: 'blur' }
  ]
};

// 加载 CF 账号列表
const loadCFAccounts = async () => {
  try {
    // 注意：axios 响应拦截器已经返回了 response.data，所以这里直接获取数组
    const data = await cfAccountApi.getCFAccountList();
    cfAccounts.value = data || [];
    console.log('CF 账号列表加载成功:', cfAccounts.value);
  } catch (error) {
    console.error('加载 CF 账号列表失败:', error);
    ElMessage.error('加载 CF 账号列表失败，请刷新页面重试');
  }
};

// 加载 R2 存储桶列表
const loadR2Buckets = async () => {
  try {
    // 获取所有 R2 存储桶（axios 拦截器已返回 data）
    const buckets = await r2Api.getR2BucketList();
    r2Buckets.value = buckets || [];
    console.log('R2 存储桶加载成功，共', r2Buckets.value.length, '个:', r2Buckets.value);
  } catch (error) {
    console.error('加载 R2 存储桶列表失败:', error);
  }
};

// 处理存储桶选择变化
const handleBucketChange = async (bucketId) => {
  if (!bucketId) {
    selectedBucketDomains.value = [];
    resetDomainPagination();
    return;
  }
  
  // 重置分页和域名列表
  selectedBucketDomains.value = [];
  resetDomainPagination();
  
  // 加载第一页数据
  await loadBucketDomains(bucketId, true);
};

// 重置域名分页
const resetDomainPagination = () => {
  domainPagination.currentPage = 0;
  domainPagination.hasMore = true;
};

// 加载存储桶的下载域名
const loadBucketDomains = async (bucketId, isFirstLoad = false) => {
  if (loadingDomains.value) return;
  
  try {
    loadingDomains.value = true;
    
    console.log('加载存储桶下载域名，bucketId:', bucketId, 'page:', domainPagination.currentPage);
    
    // 获取 APK 文件列表（不使用分页参数，API 会返回所有文件）
    const apkFiles = await r2Api.listApkFiles(bucketId, '');
    const allFiles = apkFiles?.files || apkFiles || [];
    
    console.log('所有 APK 文件:', allFiles);
    
    // 手动分页处理
    const startIndex = domainPagination.currentPage * domainPagination.pageSize;
    const endIndex = startIndex + domainPagination.pageSize;
    const fileList = allFiles.slice(startIndex, endIndex);
    
    console.log('APK 文件列表:', fileList);
    
    if (fileList.length === 0) {
      domainPagination.hasMore = false;
      if (isFirstLoad) {
        ElMessage.info('该存储桶暂无 APK 文件');
      }
      return;
    }
    
    // 对每个 APK 文件获取下载域名
    const domainPromises = fileList.map(async (file) => {
      try {
        // axios 拦截器已返回 response.data，所以 urls 直接就是数组
        const urls = await r2Api.getApkFileUrls(bucketId, file.key || file.file_path);
        const urlList = urls || [];
        
        console.log(`文件 ${file.key} 的下载链接:`, urlList);
        
        // 将每个 URL 转换为下拉选项
        return urlList.map(urlObj => ({
          url: urlObj.url,
          fileName: file.key || file.file_path,
          domainName: urlObj.domain || new URL(urlObj.url).hostname,
          fileSize: file.size,
          lastModified: file.last_modified
        }));
      } catch (error) {
        console.error(`获取文件 ${file.key} 的下载链接失败:`, error);
        return [];
      }
    });
    
    const results = await Promise.all(domainPromises);
    const newDomains = results.flat();
    
    console.log('加载到的下载域名:', newDomains);
    
    // 追加到列表
    selectedBucketDomains.value = [...selectedBucketDomains.value, ...newDomains];
    
    // 更新分页状态
    domainPagination.currentPage++;
    domainPagination.hasMore = fileList.length >= domainPagination.pageSize;
    
    if (isFirstLoad && selectedBucketDomains.value.length === 0) {
      ElMessage.warning('该存储桶的 APK 文件暂无下载链接');
    }
    
  } catch (error) {
    console.error('加载存储桶下载域名失败:', error);
    ElMessage.error('加载下载域名失败: ' + error.message);
  } finally {
    loadingDomains.value = false;
  }
};

// 加载更多域名
const loadMoreDomains = async () => {
  if (!selectedBucketId.value || !domainPagination.hasMore || loadingDomains.value) {
    return;
  }
  
  await loadBucketDomains(selectedBucketId.value, false);
};

// 处理下拉框显示/隐藏
const handleDomainDropdownVisible = (visible) => {
  // 下拉框打开时，如果还有更多数据且当前列表较少，可以预加载
  if (visible && domainPagination.hasMore && selectedBucketDomains.value.length < 10) {
    loadMoreDomains();
  }
};

// 处理清空域名
const handleClearDomain = () => {
  workerForm.target_domain = '';
};

// 处理域名输入模式变化
const handleDomainModeChange = (mode) => {
  console.log('域名输入模式切换为:', mode);
  
  // 切换模式时清空相关数据
  workerForm.target_domain = '';
  selectedBucketId.value = null;
  selectedBucketDomains.value = [];
  
  // 如果切换到选择模式，且还没有加载存储桶，则加载
  if (mode === 'select' && r2Buckets.value.length === 0) {
    loadR2Buckets();
  }
};

// 加载 Worker 列表
const loadWorkers = async () => {
  loading.value = true;
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.page_size,
      ...searchForm
    };

    // axios 拦截器已返回 response.data
    const data = await getWorkerList(params);
    workers.value = data.data || [];
    pagination.total = data.pagination?.total || 0;
  } catch (error) {
    ElMessage.error('加载 Worker 列表失败: ' + (error.response?.data?.error || error.message));
  } finally {
    loading.value = false;
  }
};

// 重置搜索
const resetSearch = () => {
  searchForm.cf_account_id = null;
  pagination.page = 1;
  loadWorkers();
};

// 显示创建对话框
const showCreateDialog = () => {
  console.log('当前 CF 账号列表:', cfAccounts.value);
  
  // 如果账号列表为空，提示用户
  if (!cfAccounts.value || cfAccounts.value.length === 0) {
    ElMessage.warning('暂无可用的 CF 账号，请先添加 CF 账号');
    return;
  }
  
  isEdit.value = false;
  dialogVisible.value = true;
};

// 显示编辑对话框
const showEditDialog = (row) => {
  isEdit.value = true;
  currentWorkerId.value = row.id;
  
  Object.assign(workerForm, {
    cf_account_id: row.cf_account_id,
    worker_name: row.worker_name,
    worker_domain: row.worker_domain,
    target_domain: row.target_domain,
    status: row.status || 'active',
    description: row.description || ''
  });
  
  dialogVisible.value = true;
};

// 处理对话框关闭
const handleDialogClose = () => {
  workerFormRef.value?.resetFields();
  Object.assign(workerForm, {
    cf_account_id: null,
    worker_name: '',
    worker_domain: '',
    target_domain: '',
    status: 'active',
    description: ''
  });
  currentWorkerId.value = null;
  
  // 重置存储桶选择和输入模式
  domainInputMode.value = 'manual';
  selectedBucketId.value = null;
  selectedBucketDomains.value = [];
  resetDomainPagination();
};

// 提交表单
const handleSubmit = async () => {
  try {
    await workerFormRef.value?.validate();
    
    submitting.value = true;
    
    if (isEdit.value) {
      // 更新 Worker
      await updateWorker(currentWorkerId.value, {
        target_domain: workerForm.target_domain,
        status: workerForm.status,
        description: workerForm.description
      });
      ElMessage.success('Worker 更新成功');
    } else {
      // 创建 Worker
      await createWorker(workerForm);
      ElMessage.success('Worker 创建成功');
    }
    
    dialogVisible.value = false;
    loadWorkers();
  } catch (error) {
    if (error.response?.data?.error) {
      ElMessage.error(error.response.data.error);
    } else if (error !== false) {
      // 排除表单验证失败的情况
      ElMessage.error('操作失败: ' + error.message);
    }
  } finally {
    submitting.value = false;
  }
};

// 删除 Worker
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 Worker "${row.worker_name}" 吗？此操作将删除 Worker 脚本和路由绑定。`,
      '确认删除',
      {
        type: 'warning',
        confirmButtonText: '确定',
        cancelButtonText: '取消'
      }
    );
    
    await deleteWorker(row.id);
    ElMessage.success('Worker 删除成功');
    loadWorkers();
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.response?.data?.error || error.message));
    }
  }
};

// 格式化日期
const formatDate = (dateString) => {
  if (!dateString) return '-';
  const date = new Date(dateString);
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  });
};

// 组件挂载时加载数据
onMounted(async () => {
  // 优先加载 CF 账号列表，确保对话框打开时有数据
  await loadCFAccounts();
  
  // 加载 Worker 列表
  loadWorkers();
  
  // R2 存储桶按需加载（切换到选择模式时才加载）
});
</script>

<style scoped>
.cf-worker-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.search-form {
  margin-bottom: 20px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.4;
}

:deep(.el-dialog__body) {
  padding: 20px;
}

.domain-option {
  line-height: 1.5;
}

.domain-name {
  font-size: 14px;
  color: #303133;
}

.domain-desc {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.domain-option-inline {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.domain-name-inline {
  flex: 1;
  font-size: 14px;
}

.domain-status-inline {
  margin-left: 10px;
}

.domain-option-block {
  padding: 4px 0;
}

.domain-url {
  font-size: 14px;
  color: #303133;
  margin-bottom: 4px;
  word-break: break-all;
}

.domain-file-info {
  font-size: 12px;
  color: #909399;
}

:deep(.el-loading-mask) {
  background-color: rgba(255, 255, 255, 0.8);
}

.is-loading {
  animation: rotating 2s linear infinite;
}

@keyframes rotating {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}
</style>
