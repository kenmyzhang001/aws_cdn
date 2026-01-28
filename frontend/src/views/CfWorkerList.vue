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
          >
            <el-option
              v-for="account in cfAccounts"
              :key="account.id"
              :label="account.email"
              :value="account.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="Worker 名称" prop="worker_name" v-if="!isEdit">
          <el-input
            v-model="workerForm.worker_name"
            placeholder="请输入 Worker 名称，如：my-redirect-worker"
          />
          <div class="form-tip">Worker 脚本的名称，创建后不可修改</div>
        </el-form-item>

        <el-form-item label="Worker 域名" prop="worker_domain" v-if="!isEdit">
          <el-input
            v-model="workerForm.worker_domain"
            placeholder="请输入 Worker 域名，如：redirect.example.com"
          />
          <div class="form-tip">用户访问的域名（域名 A），需要在 Cloudflare 中托管</div>
        </el-form-item>

        <el-form-item label="目标域名" prop="target_domain">
          <el-input
            v-model="workerForm.target_domain"
            placeholder="请输入目标跳转域名，如：https://target.example.com"
          />
          <div class="form-tip">
            跳转的目标地址（域名 B），支持完整 URL
          </div>
        </el-form-item>

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
import { Plus } from '@element-plus/icons-vue';
import {
  getWorkerList,
  createWorker,
  updateWorker,
  deleteWorker
} from '@/api/cf_worker';
import { cfAccountApi } from '@/api/cf_account';

// 搜索表单
const searchForm = reactive({
  cf_account_id: null
});

// Worker 列表
const workers = ref([]);
const loading = ref(false);

// CF 账号列表
const cfAccounts = ref([]);

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
    const response = await cfAccountApi.getCFAccountList();
    cfAccounts.value = response.data.data || [];
  } catch (error) {
    console.error('加载 CF 账号列表失败:', error);
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

    const response = await getWorkerList(params);
    workers.value = response.data.data || [];
    pagination.total = response.data.pagination?.total || 0;
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
onMounted(() => {
  loadCFAccounts();
  loadWorkers();
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
</style>
