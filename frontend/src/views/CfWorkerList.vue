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
            style="width: 300px"
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
        <el-form-item label="域名搜索">
          <el-input
            v-model="searchForm.domain"
            placeholder="Worker 域名或目标域名"
            clearable
            style="width: 220px"
            @keyup.enter="loadWorkers"
          />
        </el-form-item>
        <el-form-item label="业务模式">
          <el-select
            v-model="searchForm.business_mode"
            clearable
            placeholder="全部"
            style="width: 120px"
            @change="loadWorkers"
          >
            <el-option label="下载" value="下载" />
            <el-option label="推广" value="推广" />
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
        <el-table-column label="Worker 域名" width="260">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px;">
              <el-link :href="`https://${row.worker_domain}`" target="_blank" type="primary">
                {{ row.worker_domain }}
              </el-link>
              <el-button
                link
                type="primary"
                :icon="DocumentCopy"
                size="small"
                @click="copyToClipboard(row.worker_domain, 'Worker 域名')"
                title="复制域名"
              />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="业务模式" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.business_mode === '下载'" type="primary">下载</el-tag>
            <el-tag v-else-if="row.business_mode === '推广'" type="success">推广</el-tag>
            <el-tag v-else type="info">{{ row.business_mode || '推广' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="轮播模式" width="100">
          <template #default="{ row }">
            <el-tag v-if="!row.mode || row.mode === 'single'" type="info">单链接</el-tag>
            <el-tag v-else-if="row.mode === 'time'" type="warning">时间轮播</el-tag>
            <el-tag v-else-if="row.mode === 'random'" type="success">随机</el-tag>
            <el-tag v-else-if="row.mode === 'probe'" type="primary">探针</el-tag>
            <el-tag v-else type="info">{{ row.mode }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="目标" width="220">
          <template #default="{ row }">
            <template v-if="row.targets && row.targets.length > 1">
              <el-tooltip :content="row.targets.join('\n')" placement="top" max-width="400">
                <span>{{ row.target_domain || row.targets[0] }}</span>
              </el-tooltip>
              <el-tag size="small" style="margin-left: 4px;">共 {{ row.targets.length }} 个</el-tag>
            </template>
            <template v-else>
              <span>{{ row.target_domain || (row.targets && row.targets[0]) || '-' }}</span>
              <el-link v-if="row.target_domain || (row.targets && row.targets[0])" :href="targetHref(row.target_domain || row.targets[0])" target="_blank" type="success" style="margin-left: 4px;">打开</el-link>
            </template>
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

        <el-form-item label="业务模式">
          <el-radio-group v-model="workerForm.business_mode">
            <el-radio label="推广">推广</el-radio>
            <el-radio label="下载">下载</el-radio>
          </el-radio-group>
          <div class="form-tip">用于区分 Worker 用途：推广或下载</div>
        </el-form-item>
        <el-form-item label="轮播模式">
          <el-radio-group v-model="workerForm.mode" @change="handleWorkerModeChange">
            <el-radio label="single">单链接(无兜底)</el-radio>
            <el-radio label="time">时间轮播(有兜底)</el-radio>
            <el-radio label="random">随机(有兜底)</el-radio>
            <el-radio label="probe">探针模式(有兜底)</el-radio>
          </el-radio-group>
          <div class="form-tip">
            单链接：固定跳转一个地址，无兜底；时间轮播：按天数轮换，有兜底；随机：每次随机选一个，有兜底；探针：选最快可用地址，有兜底
          </div>
        </el-form-item>

        <el-form-item label="Worker 域名" prop="worker_domain" v-if="!isEdit">
          <div style="display: flex; align-items: flex-start; gap: 10px;">
            <!-- 子域名前缀输入框 -->
            <div style="flex: 0 0 220px;">
              <el-input
                v-model="workerForm.worker_domain_prefix"
                placeholder="可选：如 www, api, cdn"
                clearable
                :disabled="!workerForm.cf_account_id"
                @input="updateWorkerDomain"
              >
                <template #append>.</template>
              </el-input>
              <div class="form-tip" style="margin-top: 4px; white-space: nowrap;">
                子域名前缀（可选）
              </div>
            </div>
            
            <!-- 基础域名选择框 -->
            <div style="flex: 1; min-width: 0;">
              <el-select
                v-model="workerForm.worker_base_domain"
                placeholder="选择或输入基础域名（必填）"
                style="width: 100%"
                filterable
                allow-create
                clearable
                default-first-option
                :loading="loadingWorkerDomains"
                :disabled="!workerForm.cf_account_id"
                :filter-method="filterWorkerDomains"
                @change="updateWorkerDomain"
              >
                <template #empty>
                  <div style="padding: 10px; text-align: center; color: #909399;">
                    <div v-if="workerDomainSearchQuery">
                      未找到匹配的域名
                      <div style="margin-top: 8px;">
                        <el-button size="small" type="primary" @click="useCustomDomain">
                          使用 "{{ workerDomainSearchQuery }}" 作为域名
                        </el-button>
                      </div>
                    </div>
                    <div v-else>
                      暂无可用域名，请输入完整域名
                    </div>
                  </div>
                </template>
                
                <el-option
                  v-for="domain in filteredWorkerDomains"
                  :key="domain"
                  :label="domain"
                  :value="domain"
                >
                  <div style="display: flex; justify-content: space-between; align-items: center;">
                    <span>{{ domain }}</span>
                    <el-tag size="small" type="success">已托管</el-tag>
                  </div>
                </el-option>
                
                <!-- 加载更多选项 -->
                <el-option
                  v-if="workerZonesPagination.hasMore && !workerDomainSearchQuery"
                  :value="'__load_more__'"
                  disabled
                  style="background-color: #f5f7fa; cursor: pointer !important;"
                >
                  <div style="text-align: center; padding: 5px 0;">
                    <el-button 
                      type="primary" 
                      size="small"
                      @click.stop="loadMoreWorkerZones"
                      :loading="loadingWorkerDomains"
                      style="width: 90%;"
                    >
                      <span v-if="!loadingWorkerDomains">
                        加载更多域名 ({{ workerDomains.length }}/{{ workerZonesPagination.totalCount }})
                      </span>
                      <span v-else>加载中...</span>
                    </el-button>
                  </div>
                </el-option>
              </el-select>
              <div class="form-tip" style="margin-top: 4px;">
                基础域名（必填）
              </div>
            </div>
          </div>
          
          <!-- 完整域名预览 -->
          <div v-if="workerForm.worker_domain" style="margin-top: 10px; padding: 10px 14px; background: #f0f9ff; border: 1px solid #91caff; border-radius: 6px;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <el-icon color="#1890ff" :size="16"><Link /></el-icon>
              <span style="color: #1890ff; font-weight: 500;">完整域名:</span>
              <span style="color: #262626; font-family: 'Monaco', 'Menlo', monospace; font-size: 14px; font-weight: 500;">{{ workerForm.worker_domain }}</span>
            </div>
          </div>
          
          <div class="form-tip" v-if="!workerForm.cf_account_id">
            请先选择 CF 账号
          </div>
          <div class="form-tip" v-else-if="loadingWorkerDomains" style="color: #409EFF;">
            <el-icon class="is-loading"><Loading /></el-icon>
            正在加载域名列表...
          </div>
          <div class="form-tip" v-else-if="workerDomains.length > 0" style="color: #67C23A;">
            已加载 {{ workerDomains.length }}/{{ workerZonesPagination.totalCount }} 个托管域名
            <span v-if="filteredWorkerDomains.length < workerDomains.length">
              （搜索结果: {{ filteredWorkerDomains.length }} 个）
            </span>
            <el-button 
              v-if="workerZonesPagination.hasMore" 
              type="primary" 
              link 
              size="small"
              @click="loadMoreWorkerZones"
              :loading="loadingWorkerDomains"
              style="margin-left: 8px;"
            >
              加载更多 (第 {{ workerZonesPagination.page + 1 }}/{{ workerZonesPagination.totalPages }} 页)
            </el-button>
          </div>
          <div class="form-tip" v-else style="color: #E6A23C;">
            该账号暂无托管域名，请手动输入完整域名
          </div>
        </el-form-item>

        <!-- 单链接：目标域名来源 -->
        <template v-if="workerForm.mode === 'single'">
          <el-form-item label="目标域名来源">
            <el-radio-group v-model="domainInputMode" @change="handleDomainModeChange">
              <el-radio label="manual">手动输入</el-radio>
              <el-radio label="select">从 CF 下载链接选择</el-radio>
            </el-radio-group>
          </el-form-item>

          <template v-if="domainInputMode === 'manual'">
            <el-form-item label="目标域名" prop="target_domain">
              <el-input
                v-model="workerForm.target_domain"
                placeholder="请输入目标跳转域名，如：https://target.example.com"
                clearable
              />
              <div class="form-tip">跳转的目标地址，支持完整 URL</div>
            </el-form-item>
          </template>
        </template>

        <!-- 单链接：从 CF 选择 -->
        <template v-if="workerForm.mode === 'single' && domainInputMode === 'select'">
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

        <!-- 多目标：时间轮播 / 随机 / 探针 -->
        <template v-if="workerForm.mode && workerForm.mode !== 'single'">
          <el-form-item label="目标链接列表" prop="targets_text" required>
            <el-input
              v-model="workerForm.targets_text"
              type="textarea"
              :rows="6"
              placeholder="每行一个完整 URL，例如：&#10;https://cdn1.example.com/file.apk&#10;https://cdn2.example.com/file.apk"
            />
            <div class="form-tip">多个目标将参与轮播或探针选择；每行一个以 http:// 或 https:// 开头的 URL</div>
          </el-form-item>
          <el-form-item label="兜底链接">
            <el-input
              v-model="workerForm.fallback_url"
              placeholder="可选：当所有目标都不可用时跳转的地址"
              clearable
            />
          </el-form-item>
          <template v-if="workerForm.mode === 'time'">
            <el-form-item label="轮换天数" prop="rotate_days">
              <el-input-number v-model="workerForm.rotate_days" :min="1" :max="365" placeholder="每 N 天轮换一个目标" />
              <div class="form-tip">每多少天轮换到下一个目标，例如 7 表示每 7 天换一个</div>
            </el-form-item>
            <el-form-item label="基准日期">
              <el-date-picker
                v-model="workerForm.base_date"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="可选，默认今天"
                clearable
                style="width: 100%"
              />
            </el-form-item>
          </template>
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
            <p>• 单链接：固定跳转一个地址；时间轮播/随机/探针：多目标参与轮播或探活选最快。</p>
            <p>• 社交/短信 WebView（Telegram/Viber/Line/WhatsApp/微信等）：使用 JavaScript 跳转；普通浏览器：302 跳转。</p>
            <p>• 探活：时间轮播与随机模式会先探活再选可用目标；探针模式使用探测接口返回最快可用地址。</p>
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
import { ref, reactive, onMounted, computed, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Plus, RefreshRight, Loading, Link, DocumentCopy } from '@element-plus/icons-vue';
import {
  getWorkerList,
  createWorker,
  updateWorker,
  deleteWorker,
  checkWorkerDomain
} from '@/api/cf_worker';
import { cfAccountApi } from '@/api/cf_account';
import { r2Api } from '@/api/r2';

// 搜索表单
const searchForm = reactive({
  cf_account_id: null,
  domain: '',
  business_mode: ''
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

// 过滤后的 Worker 域名列表
const filteredWorkerDomains = ref([]);

// Worker 域名搜索查询
const workerDomainSearchQuery = ref('');

// Worker 域名分页状态
const workerZonesPagination = reactive({
  page: 1,
  perPage: 50,
  totalPages: 0,
  totalCount: 0,
  hasMore: false
});

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
  worker_domain_prefix: '',
  worker_base_domain: '',
  target_domain: '',
  mode: 'single',
  business_mode: '推广',  // 业务模式：下载、推广
  targets_text: '',      // 多目标时每行一个 URL
  fallback_url: '',
  rotate_days: 7,
  base_date: '',
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
  worker_domain: [
    { required: true, message: '请输入 Worker 域名', trigger: 'blur' }
  ],
  target_domain: [
    {
      validator: (rule, value, cb) => {
        if (workerForm.mode === 'single' && !(value || '').trim()) return cb(new Error('请输入目标域名'));
        cb();
      },
      trigger: 'blur'
    }
  ],
  targets_text: [
    {
      validator: (rule, value, cb) => {
        if (workerForm.mode && workerForm.mode !== 'single') {
          const lines = (value || '').trim().split(/\n/).map(s => s.trim()).filter(s => s && (s.startsWith('http://') || s.startsWith('https://')));
          if (lines.length === 0) return cb(new Error('请至少输入一个以 http:// 或 https:// 开头的目标链接'));
        }
        cb();
      },
      trigger: 'blur'
    }
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

// 处理 CF 账号选择变化
const handleCFAccountChange = async (cfAccountId) => {
  console.log('CF 账号变化:', cfAccountId);
  
  // 清空 Worker 域名和搜索状态
  workerForm.worker_domain = '';
  workerForm.worker_domain_prefix = '';
  workerForm.worker_base_domain = '';
  workerDomains.value = [];
  filteredWorkerDomains.value = [];
  workerDomainSearchQuery.value = '';
  
  // 重置分页状态
  workerZonesPagination.page = 1;
  workerZonesPagination.totalPages = 0;
  workerZonesPagination.totalCount = 0;
  workerZonesPagination.hasMore = false;
  
  if (!cfAccountId) {
    return;
  }
  
  // 加载该账号下的域名列表（第一页）
  await loadCFAccountZones(cfAccountId, false);
};

// 监听 CF 账号变化，自动重新加载域名列表
watch(() => workerForm.cf_account_id, async (newAccountId, oldAccountId) => {
  // 当 CF 账号变化时（且不是初始化时的空值），重新加载域名列表
  if (newAccountId !== oldAccountId && newAccountId && dialogVisible.value && !isEdit.value) {
    console.log('检测到 CF 账号变化，自动重新加载域名列表');
    
    // 清空 Worker 域名和搜索状态
    workerForm.worker_domain = '';
    workerForm.worker_domain_prefix = '';
    workerForm.worker_base_domain = '';
    workerDomains.value = [];
    filteredWorkerDomains.value = [];
    workerDomainSearchQuery.value = '';
    
    // 重置分页状态
    workerZonesPagination.page = 1;
    workerZonesPagination.totalPages = 0;
    workerZonesPagination.totalCount = 0;
    workerZonesPagination.hasMore = false;
    
    // 加载新账号的域名列表
    await loadCFAccountZones(newAccountId, false);
  }
});

// 加载 CF 账号的域名列表
// isLoadMore: 是否为加载更多（true则追加，false则替换）
const loadCFAccountZones = async (cfAccountId, isLoadMore = false) => {
  if (loadingWorkerDomains.value) return;
  
  try {
    loadingWorkerDomains.value = true;
    
    const page = isLoadMore ? workerZonesPagination.page + 1 : 1;
    
    console.log('开始加载 CF 账号域名列表, cfAccountId:', cfAccountId, 'page:', page);
    
    const result = await cfAccountApi.getCFAccountZones(cfAccountId, {
      page: page,
      per_page: workerZonesPagination.perPage
    });
    
    console.log('CF 账号域名列表响应:', result);
    
    // 兼容旧格式（数组）和新格式（带分页信息的对象）
    let zoneList = [];
    if (Array.isArray(result)) {
      // 旧格式：直接返回数组
      console.warn('检测到旧格式API响应，建议重启后端服务');
      zoneList = result;
      workerZonesPagination.page = 1;
      workerZonesPagination.totalPages = 1;
      workerZonesPagination.totalCount = result.length;
      workerZonesPagination.hasMore = false;
    } else {
      // 新格式：带分页信息的对象
      zoneList = result.zones || [];
      workerZonesPagination.page = result.page || page;
      workerZonesPagination.totalPages = result.total_pages || 0;
      workerZonesPagination.totalCount = result.total_count || 0;
      workerZonesPagination.hasMore = workerZonesPagination.page < workerZonesPagination.totalPages;
    }
    
    // 提取域名名称
    const newDomains = zoneList.map(zone => zone.name || zone);
    
    if (isLoadMore) {
      // 追加到现有列表
      workerDomains.value = [...workerDomains.value, ...newDomains];
    } else {
      // 替换列表
      workerDomains.value = newDomains;
    }
    
    // 更新过滤列表
    if (!workerDomainSearchQuery.value) {
      filteredWorkerDomains.value = [...workerDomains.value];
    } else {
      // 重新应用搜索过滤
      filterWorkerDomains(workerDomainSearchQuery.value);
    }
    
    if (!isLoadMore) {
      if (workerDomains.value.length === 0) {
        ElMessage.info('该 CF 账号暂无托管域名');
      } else {
        const moreMsg = workerZonesPagination.hasMore ? `，还有更多域名可加载` : '';
        ElMessage.success(`已加载 ${workerDomains.value.length}/${workerZonesPagination.totalCount} 个托管域名${moreMsg}`);
      }
    } else {
      ElMessage.success(`已加载 ${newDomains.length} 个域名（总计 ${workerDomains.value.length}/${workerZonesPagination.totalCount}）`);
    }
  } catch (error) {
    console.error('加载 CF 账号域名失败:', error);
    ElMessage.error('加载域名列表失败: ' + error.message);
    if (!isLoadMore) {
      workerDomains.value = [];
      filteredWorkerDomains.value = [];
    }
  } finally {
    loadingWorkerDomains.value = false;
  }
};

// 加载更多域名
const loadMoreWorkerZones = async () => {
  if (!workerForm.cf_account_id || !workerZonesPagination.hasMore) {
    return;
  }
  await loadCFAccountZones(workerForm.cf_account_id, true);
};

// 更新完整的 Worker 域名（组合前缀和基础域名）
const updateWorkerDomain = () => {
  const prefix = workerForm.worker_domain_prefix?.trim();
  const baseDomain = workerForm.worker_base_domain?.trim();
  
  if (!baseDomain) {
    workerForm.worker_domain = '';
    return;
  }
  
  if (prefix) {
    // 有前缀：组合成 prefix.baseDomain
    workerForm.worker_domain = `${prefix}.${baseDomain}`;
  } else {
    // 无前缀：直接使用基础域名
    workerForm.worker_domain = baseDomain;
  }
  
  console.log('更新 Worker 域名:', workerForm.worker_domain);
};

// 过滤 Worker 域名
const filterWorkerDomains = (query) => {
  workerDomainSearchQuery.value = query;
  
  if (!query) {
    filteredWorkerDomains.value = [...workerDomains.value];
    return;
  }
  
  const lowerQuery = query.toLowerCase();
  filteredWorkerDomains.value = workerDomains.value.filter(domain => {
    return domain.toLowerCase().includes(lowerQuery);
  });
  
  console.log('域名搜索:', query, '结果数:', filteredWorkerDomains.value.length);
};

// 使用自定义域名
const useCustomDomain = () => {
  if (workerDomainSearchQuery.value) {
    workerForm.worker_base_domain = workerDomainSearchQuery.value;
    workerDomainSearchQuery.value = '';
    updateWorkerDomain();
  }
};

// 处理域名输入模式变化
const handleDomainModeChange = (mode) => {
  workerForm.target_domain = '';
  selectedBucketId.value = null;
  selectedBucketDomains.value = [];
  if (mode === 'select' && r2Buckets.value.length === 0) loadR2Buckets();
};

// 处理轮播模式变化
const handleWorkerModeChange = (mode) => {
  if (mode !== 'single') {
    workerForm.target_domain = '';
    domainInputMode.value = 'manual';
  }
  if (mode === 'single') {
    workerForm.targets_text = '';
    workerForm.fallback_url = '';
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
  searchForm.domain = '';
  searchForm.business_mode = '';
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
  const targets = row.targets && Array.isArray(row.targets) ? row.targets : (row.target_domain ? [row.target_domain] : []);
  Object.assign(workerForm, {
    cf_account_id: row.cf_account_id,
    worker_name: row.worker_name,
    worker_domain: row.worker_domain,
    worker_domain_prefix: '',
    worker_base_domain: '',
    target_domain: row.target_domain || (targets[0] || ''),
    mode: row.mode || 'single',
    business_mode: row.business_mode || '推广',
    targets_text: targets.join('\n'),
    fallback_url: row.fallback_url || '',
    rotate_days: row.rotate_days || 7,
    base_date: row.base_date || '',
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
    worker_domain_prefix: '',
    worker_base_domain: '',
    target_domain: '',
    mode: 'single',
    business_mode: '推广',
    targets_text: '',
    fallback_url: '',
    rotate_days: 7,
    base_date: '',
    status: 'active',
    description: ''
  });
  currentWorkerId.value = null;
  domainInputMode.value = 'manual';
  selectedBucketId.value = null;
  selectedBucketDomains.value = [];
  workerDomains.value = [];
  filteredWorkerDomains.value = [];
  workerDomainSearchQuery.value = '';
  resetDomainPagination();
};

const targetHref = (u) => {
  if (!u) return '#';
  return u.startsWith('http') ? u : 'https://' + u;
};

// 根据域名 + 时间自动生成 Worker 名称（仅小写字母、数字、下划线、连字符）
const generateWorkerName = (domain) => {
  if (!domain || !domain.trim()) return '';
  const safe = domain.trim()
    .replace(/\./g, '_')
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '') || 'worker';
  const time = new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '');
  return `${safe}_${time}`;
};

// 解析多目标文本为数组
const parseTargetsText = (text) => {
  return (text || '').trim().split(/\n/).map(s => s.trim()).filter(s => s && (s.startsWith('http://') || s.startsWith('https://')));
};

// 提交表单
const handleSubmit = async () => {
  try {
    await workerFormRef.value?.validate();
    const isSingle = workerForm.mode === 'single';
    if (isSingle && !(workerForm.target_domain || '').trim()) {
      ElMessage.error('请输入目标域名');
      return;
    }
    if (!isSingle) {
      const targets = parseTargetsText(workerForm.targets_text);
      if (targets.length === 0) {
        ElMessage.error('请至少输入一个以 http:// 或 https:// 开头的目标链接');
        return;
      }
    }

    submitting.value = true;

    const payload = {
      status: workerForm.status,
      description: workerForm.description,
      mode: workerForm.mode || 'single',
      business_mode: workerForm.business_mode || '推广',
      fallback_url: workerForm.fallback_url || '',
      rotate_days: workerForm.rotate_days || 0,
      base_date: workerForm.base_date || ''
    };
    if (isSingle) {
      payload.target_domain = workerForm.target_domain.trim();
    } else {
      payload.targets = parseTargetsText(workerForm.targets_text);
    }

    if (isEdit.value) {
      await updateWorker(currentWorkerId.value, payload);
      ElMessage.success('Worker 更新成功');
    } else {
      const workerName = generateWorkerName(workerForm.worker_domain);
      if (!workerName) {
        ElMessage.error('请先填写 Worker 域名');
        submitting.value = false;
        return;
      }
      const createPayload = {
        cf_account_id: workerForm.cf_account_id,
        worker_name: workerName,
        worker_domain: workerForm.worker_domain,
        description: workerForm.description,
        mode: payload.mode,
        business_mode: payload.business_mode,
        fallback_url: payload.fallback_url,
        rotate_days: payload.rotate_days,
        base_date: payload.base_date
      };
      if (isSingle) {
        createPayload.target_domain = workerForm.target_domain.trim();
      } else {
        createPayload.targets = payload.targets;
      }
      try {
        const check = await checkWorkerDomain(workerForm.worker_domain);
        const res = check?.data ?? check;
        if (res && res.available === false) {
          const usedBy = res.used_by === 'domain_redirect' ? '域名302重定向' : 'Cloudflare Worker';
          ElMessage.error(`域名 ${workerForm.worker_domain} 已被「${usedBy}」使用${res.ref_name ? `（${res.ref_name}）` : ''}，请先删除后再创建`);
          submitting.value = false;
          return;
        }
      } catch (e) {
        // 检查接口失败不阻塞，由创建时后端再次校验
      }
      await createWorker(createPayload);
      ElMessage.success('Worker 创建成功');
    }

    dialogVisible.value = false;
    loadWorkers();
  } catch (error) {
    if (error.response?.data?.error) {
      ElMessage.error(error.response.data.error);
    } else if (error !== false) {
      ElMessage.error('操作失败: ' + (error.message || ''));
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

// 复制到剪贴板
const copyToClipboard = async (text, label = '内容') => {
  try {
    await navigator.clipboard.writeText(text);
    ElMessage.success(`${label}已复制到剪贴板`);
  } catch (error) {
    // 降级方案：使用传统方法
    try {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      ElMessage.success(`${label}已复制到剪贴板`);
    } catch (fallbackError) {
      console.error('复制失败:', fallbackError);
      ElMessage.error('复制失败，请手动复制');
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
