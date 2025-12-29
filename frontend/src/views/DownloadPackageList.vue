<template>
  <div class="download-package-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>下载域名管理</span>
          <div style="display: flex; gap: 8px; align-items: center">
            <el-button
              type="primary"
              size="default"
              :icon="CopyDocument"
              @click="copyAllFileList"
              title="复制所有文件列表"
            >
              复制所有文件列表
            </el-button>
            <el-button type="primary" @click="openAddDomainDialog">
              <el-icon><Plus /></el-icon>
              添加下载域名
            </el-button>
          </div>
        </div>
      </template>

      <!-- 搜索框 -->
      <div style="margin-bottom: 20px">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索域名..."
          clearable
          style="width: 300px"
          @input="handleSearch"
          @clear="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <!-- 分组Tab -->
      <el-tabs v-model="activeGroupId" @tab-change="handleGroupChange" style="margin-bottom: 20px">
        <el-tab-pane :label="`全部 (${totalAll})`" :name="null"></el-tab-pane>
        <el-tab-pane
          v-for="group in groups"
          :key="group.id"
          :label="`${group.name} (${group.count || 0})`"
          :name="group.id"
        ></el-tab-pane>
      </el-tabs>

      <!-- 域名列表 -->
      <div v-loading="loading">
        <el-table v-if="domainList.length > 0" :data="domainList" stripe border>
          <el-table-column prop="domain_name" label="域名" min-width="200">
            <template #default="{ row }">
              <span class="domain-name">{{ row.domain_name }}</span>
            </template>
          </el-table-column>
          <el-table-column label="文件数量" width="120">
            <template #default="{ row }">
              <el-tag size="small">{{ row.file_count || 0 }} 个文件</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="120">
            <template #default="{ row }">
              <el-tag v-if="row.status" :type="getStatusType(row.status)" size="small">
                {{ getStatusText(row.status) }}
              </el-tag>
              <span v-else style="color: #909399; font-size: 12px">-</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="200" fixed="right">
            <template #default="{ row }">
              <el-button
                size="small"
                type="primary"
                @click="viewDomainDetails(row)"
              >
                查看详情
              </el-button>
              <el-button
                size="small"
                type="success"
                @click="openAddFileDialog(row)"
              >
                添加文件
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-empty v-else description="暂无下载域名，请先添加下载域名" />

        <!-- 分页组件 -->
        <div v-if="totalDomains > 0" style="margin-top: 20px; display: flex; justify-content: flex-end">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="totalDomains"
            layout="total, sizes, prev, pager, next, jumper"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </div>
    </el-card>

    <!-- 添加下载域名对话框 -->
    <el-dialog v-model="showAddDomainDialog" title="添加下载域名" width="600px">
      <el-form :model="addDomainForm" label-width="120px" :rules="addDomainRules" ref="addDomainFormRef">
        <el-form-item label="DNS提供商" required>
          <el-radio-group v-model="addDomainForm.dns_provider" @change="loadAvailableDomains">
            <el-radio label="cloudflare">Cloudflare</el-radio>
            <el-radio label="aws">AWS Route53</el-radio>
          </el-radio-group>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            选择域名托管商，将影响证书验证和DNS记录的创建方式
          </div>
        </el-form-item>
        <el-form-item label="选择域名" prop="domain_id" required>
          <el-select
            v-model="addDomainForm.domain_id"
            placeholder="请选择已签发证书且未被使用的域名"
            style="width: 100%"
            filterable
          >
            <el-option
              v-for="domain in filteredAvailableDomains"
              :key="domain.id"
              :label="domain.domain_name"
              :value="domain.id"
            >
              <div>
                <span>{{ domain.domain_name }}</span>
                <el-tag
                  v-if="domain.dns_provider !== 'cloudflare'"
                  :type="domain.certificate_status === 'issued' ? 'success' : 'warning'"
                  size="small"
                  style="margin-left: 8px"
                >
                  {{ getCertificateStatusText(domain.certificate_status) }}
                </el-tag>
                <el-tag
                  v-if="domain.dns_provider"
                  size="small"
                  :type="domain.dns_provider === 'cloudflare' ? 'warning' : 'primary'"
                  style="margin-left: 5px"
                >
                  {{ domain.dns_provider === 'cloudflare' ? 'Cloudflare' : 'AWS' }}
                </el-tag>
              </div>
            </el-option>
          </el-select>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            AWS域名需要证书已签发，Cloudflare域名只需要域名状态为已完成，且未被重定向和下载包使用
          </div>
        </el-form-item>
        <el-form-item label="上传第一个APK文件" prop="file">
          <el-upload
            ref="addDomainUploadRef"
            :auto-upload="false"
            :on-change="handleAddDomainFileChange"
            :limit="1"
            :file-list="addDomainFileList"
          >
            <template #trigger>
              <el-button type="primary">选择APK文件</el-button>
            </template>
          </el-upload>
          <div v-if="addDomainSelectedFile" style="margin-top: 10px">
            <div>文件名: {{ addDomainSelectedFile.name }}</div>
            <div>文件大小: {{ formatFileSize(addDomainSelectedFile.size) }}</div>
          </div>
          <div v-if="addDomainLoading" style="margin-top: 15px">
            <el-progress
              v-if="addDomainUploadProgress > 0"
              :percentage="addDomainUploadProgress"
              :status="addDomainUploadProgress === 100 ? 'success' : null"
              :stroke-width="8"
            />
            <div v-else style="text-align: center; color: #909399; font-size: 12px">
              准备上传...
            </div>
            <div v-if="addDomainUploadProgress > 0" style="text-align: center; margin-top: 5px; color: #909399; font-size: 12px">
              上传中... {{ addDomainUploadProgress }}%
            </div>
          </div>
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            可选：可以先添加域名，稍后再上传文件
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDomainDialog = false" :disabled="addDomainLoading">取消</el-button>
        <el-button type="primary" @click="handleAddDomain" :loading="addDomainLoading">
          {{ addDomainLoading ? '上传中...' : '确定' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 添加文件对话框（用于已有域名） -->
    <el-dialog v-model="showAddFileDialog" title="添加APK文件" width="600px">
      <el-form :model="addFileForm" label-width="120px" :rules="addFileRules" ref="addFileFormRef">
        <el-form-item label="域名">
          <el-input v-model="addFileForm.domain_name" disabled />
        </el-form-item>
        <el-form-item label="选择APK文件" prop="file" required>
          <el-upload
            ref="addFileUploadRef"
            :auto-upload="false"
            :on-change="handleAddFileChange"
            :on-remove="handleAddFileRemove"
            :file-list="addFileList"
            accept=".apk"
            multiple
          >
            <template #trigger>
              <el-button type="primary">选择APK文件</el-button>
            </template>
          </el-upload>
          <div v-if="addFileSelectedFiles && addFileSelectedFiles.length > 0" style="margin-top: 10px">
            <div v-for="(file, index) in addFileSelectedFiles" :key="index" style="margin-bottom: 8px; padding: 8px; background: #f5f7fa; border-radius: 4px">
              <div>文件名: {{ file.name }}</div>
              <div>文件大小: {{ formatFileSize(file.size) }}</div>
              <div v-if="addFileUploadProgressMap[file.name] !== undefined" style="margin-top: 5px">
                <el-progress
                  :percentage="addFileUploadProgressMap[file.name]"
                  :status="addFileUploadProgressMap[file.name] === 100 ? 'success' : null"
                  :stroke-width="6"
                />
                <div style="text-align: center; margin-top: 3px; color: #909399; font-size: 12px">
                  {{ addFileUploadProgressMap[file.name] === 100 ? '上传完成' : `上传中... ${addFileUploadProgressMap[file.name]}%` }}
                </div>
              </div>
            </div>
          </div>
          <div v-if="addFileLoading && addFileSelectedFiles.length === 0" style="margin-top: 15px">
            <div style="text-align: center; color: #909399; font-size: 12px">
              准备上传...
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddFileDialog = false" :disabled="addFileLoading">取消</el-button>
        <el-button type="primary" @click="handleAddFile" :loading="addFileLoading">
          {{ addFileLoading ? '上传中...' : '开始上传' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 域名详情对话框 -->
    <el-dialog v-model="showDetailDialog" title="域名详情" width="900px">
      <div v-loading="detailLoading" v-if="currentDomain">
        <el-descriptions :column="2" border style="margin-bottom: 20px">
          <el-descriptions-item label="域名">{{ currentDomain.domain_name }}</el-descriptions-item>
          <el-descriptions-item label="文件数量">
            <el-tag size="small">{{ currentDomain.file_count || 0 }} 个文件</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="域名状态" v-if="currentDomain.domain_status">
            <el-tag :type="getDomainStatusType(currentDomain.domain_status)" size="small">
              {{ getDomainStatusText(currentDomain.domain_status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="证书状态" v-if="currentDomain.certificate_status">
            <el-tag
              :type="currentDomain.certificate_status === 'issued' ? 'success' : 'warning'"
              size="small"
            >
              {{ getCertificateStatusText(currentDomain.certificate_status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront状态" v-if="currentDomain.cloudfront_status">
            <el-tag :type="getCloudFrontStatusType(currentDomain.cloudfront_status)" size="small">
              {{ getCloudFrontStatusText(currentDomain.cloudfront_status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront启用" v-if="currentDomain.cloudfront_id">
            <el-tag
              :type="currentDomain.cloudfront_enabled ? 'success' : 'danger'"
              size="small"
            >
              {{ currentDomain.cloudfront_enabled ? '已启用' : '已禁用' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront源路径" v-if="currentDomain.cloudfront_origin_path_current || currentDomain.cloudfront_origin_path_expected" :span="2">
            <div style="display: flex; flex-direction: column; gap: 4px; font-size: 12px;">
              <div>
                <span style="color: #909399;">期望:</span>
                <code style="background: #f5f7fa; padding: 2px 4px; border-radius: 2px; margin-left: 4px; font-size: 11px;">
                  {{ currentDomain.cloudfront_origin_path_expected || '(空)' }}
                </code>
              </div>
              <div>
                <span style="color: #909399;">实际:</span>
                <code 
                  :style="{
                    background: currentDomain.cloudfront_origin_path_current === currentDomain.cloudfront_origin_path_expected ? '#f0f9ff' : '#fef0f0',
                    padding: '2px 4px',
                    borderRadius: '2px',
                    marginLeft: '4px',
                    fontSize: '11px',
                    color: currentDomain.cloudfront_origin_path_current === currentDomain.cloudfront_origin_path_expected ? '#67c23a' : '#f56c6c'
                  }"
                >
                  {{ currentDomain.cloudfront_origin_path_current || '(空)' }}
                </code>
                <el-tag 
                  v-if="currentDomain.cloudfront_origin_path_current !== currentDomain.cloudfront_origin_path_expected" 
                  type="danger" 
                  size="small" 
                  style="margin-left: 4px"
                >
                  不匹配
                </el-tag>
              </div>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="S3 Policy" v-if="currentDomain.s3_bucket_policy_configured !== undefined">
            <el-tag :type="currentDomain.s3_bucket_policy_configured ? 'success' : 'danger'" size="small">
              {{ currentDomain.s3_bucket_policy_configured ? '已配置' : '未配置' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront ID" v-if="currentDomain.cloudfront_id">
            <span style="font-size: 12px; color: #606266; font-family: monospace;">
              {{ currentDomain.cloudfront_id }}
            </span>
          </el-descriptions-item>
        </el-descriptions>

        <!-- 文件列表 -->
        <div v-if="currentDomain.files && currentDomain.files.length > 0">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
            <h3 style="margin: 0">文件列表</h3>
            <div style="display: flex; gap: 8px; align-items: center">
              <el-button
                type="primary"
                size="small"
                :icon="CopyDocument"
                @click="copyFileList"
                title="复制文件列表"
              >
                复制文件列表
              </el-button>
              <el-button
                v-if="selectedFiles.length > 0"
                type="danger"
                size="small"
                @click="handleBatchDelete"
                :loading="batchDeleting"
              >
                批量删除 ({{ selectedFiles.length }})
              </el-button>
            </div>
          </div>
          <el-table 
            :data="currentDomain.files" 
            stripe 
            size="small" 
            border
            @selection-change="handleSelectionChange"
          >
            <el-table-column type="selection" width="55" />
            <el-table-column prop="file_name" label="文件名" min-width="200">
              <template #default="{ row }">
                <el-icon style="margin-right: 4px"><Document /></el-icon>
                {{ row.file_name }}
              </template>
            </el-table-column>
            <el-table-column prop="file_size" label="文件大小" width="120">
              <template #default="{ row }">
                {{ formatFileSize(row.file_size) }}
              </template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="getStatusType(row.status)" size="small">
                  {{ getStatusText(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="下载URL"  width="250" >
              <template #default="{ row }">
                <div v-if="row.download_url" style="display: flex; align-items: center; gap: 8px;">
                  <el-link
                    :href="row.download_url"
                    target="_blank"
                    type="primary"
                    style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
                  >
                    {{ row.download_url }}
                  </el-link>
                  <el-button
                    size="small"
                    :icon="CopyDocument"
                    circle
                    @click="copyDownloadUrl(row)"
                    title="复制链接"
                  />
                </div>
                <span v-else style="color: #909399">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="创建时间" width="180">
              <template #default="{ row }">
                {{ formatDate(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column label="操作" width="200" fixed="right">
              <template #default="{ row }">
                <el-button
                  size="small"
                  type="primary"
                  :loading="row.checking"
                  @click="checkPackage(row)"
                >
                  检查
                </el-button>
                <el-button
                  size="small"
                  type="danger"
                  @click="handleDelete(row)"
                >
                  删除
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
        <el-empty
          v-else
          description="该域名下暂无文件"
          :image-size="80"
        />
      </div>
      <template #footer>
        <el-button @click="showDetailDialog = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 检查状态对话框 -->
    <el-dialog v-model="showCheckDialog" title="检查下载包状态" width="700px">
      <div v-if="checkStatus">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="下载包记录">
            <el-tag :type="checkStatus.package_exists ? 'success' : 'danger'">
              {{ checkStatus.package_exists ? '存在' : '不存在' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="S3文件">
            <el-tag :type="checkStatus.s3_file_exists ? 'success' : 'danger'">
              {{ checkStatus.s3_file_exists ? '存在' : '不存在' }}
            </el-tag>
            <span v-if="checkStatus.s3_file_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.s3_file_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront分发">
            <el-tag :type="checkStatus.cloudfront_exists ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_exists ? '存在' : '不存在' }}
            </el-tag>
            <span v-if="checkStatus.cloudfront_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.cloudfront_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront启用状态">
            <el-tag :type="checkStatus.cloudfront_enabled ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_enabled ? '已启用' : '已禁用' }}
            </el-tag>
            <span v-if="checkStatus.cloudfront_enabled_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.cloudfront_enabled_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="CloudFront源路径">
            <el-tag :type="checkStatus.cloudfront_origin_path_match ? 'success' : 'danger'">
              {{ checkStatus.cloudfront_origin_path_match ? '匹配' : '不匹配' }}
            </el-tag>
            <div v-if="checkStatus.cloudfront_origin_path_current || checkStatus.cloudfront_origin_path_expected" style="margin-top: 5px; font-size: 12px; color: #606266">
              <div v-if="checkStatus.cloudfront_origin_path_current">
                当前: <code style="background: #f5f7fa; padding: 2px 4px; border-radius: 2px">{{ checkStatus.cloudfront_origin_path_current || '(空)' }}</code>
              </div>
              <div v-if="checkStatus.cloudfront_origin_path_expected" style="margin-top: 3px">
                期望: <code style="background: #f5f7fa; padding: 2px 4px; border-radius: 2px">{{ checkStatus.cloudfront_origin_path_expected || '(空)' }}</code>
              </div>
            </div>
            <span v-if="checkStatus.cloudfront_origin_path_error" style="color: #f56c6c; margin-left: 10px; display: block; margin-top: 5px">
              {{ checkStatus.cloudfront_origin_path_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="Route53 DNS记录">
            <el-tag :type="checkStatus.route53_dns_configured ? 'success' : 'danger'">
              {{ checkStatus.route53_dns_configured ? '已配置' : '未配置' }}
            </el-tag>
            <span v-if="checkStatus.route53_dns_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.route53_dns_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="下载URL可访问">
            <el-tag :type="checkStatus.download_url_accessible ? 'success' : 'danger'">
              {{ checkStatus.download_url_accessible ? '可访问' : '不可访问' }}
            </el-tag>
            <span v-if="checkStatus.download_url_error" style="color: #f56c6c; margin-left: 10px">
              {{ checkStatus.download_url_error }}
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="证书">
            <el-tag :type="checkStatus.certificate_found ? 'success' : 'danger'">
              {{ checkStatus.certificate_found ? '已找到' : '未找到' }}
            </el-tag>
            <span v-if="checkStatus.certificate_arn" style="margin-left: 10px; font-size: 12px; color: #909399">
              {{ checkStatus.certificate_arn }}
            </span>
          </el-descriptions-item>
        </el-descriptions>

        <div v-if="checkStatus.issues && checkStatus.issues.length > 0" style="margin-top: 20px">
          <h4 style="color: #f56c6c; margin-bottom: 10px">发现的问题：</h4>
          <ul>
            <li v-for="(issue, index) in checkStatus.issues" :key="index" style="color: #f56c6c; margin-bottom: 5px">
              {{ issue }}
            </li>
          </ul>
        </div>

        <div v-else style="margin-top: 20px">
          <el-alert type="success" :closable="false">所有检查项均正常</el-alert>
        </div>
      </div>
      <template #footer>
        <el-button @click="showCheckDialog = false">关闭</el-button>
        <el-button
          v-if="checkStatus && checkStatus.can_fix"
          type="primary"
          @click="handleFix"
          :loading="fixLoading"
        >
          修复
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Document, CopyDocument, Search } from '@element-plus/icons-vue'
import request from '@/api/request'
import { domainApi } from '@/api/domain'
import { downloadPackageApi } from '@/api/download_package'
import { groupApi } from '@/api/group'
import { uploadFile } from '@/utils/upload'

const loading = ref(false)
const domainList = ref([]) // 域名列表，只包含基本域名信息
const totalAll = ref(0)

// 分页相关
const currentPage = ref(1)
const pageSize = ref(10)
const totalDomains = ref(0)

// 详情相关
const showDetailDialog = ref(false)
const detailLoading = ref(false)
const currentDomain = ref(null)

const activeGroupId = ref(null)
const groups = ref([])
const searchKeyword = ref('')
let searchTimer = null

// 添加下载域名
const showAddDomainDialog = ref(false)
const addDomainLoading = ref(false)
const addDomainUploadProgress = ref(0)
const addDomainForm = ref({
  dns_provider: 'cloudflare', // 默认使用Cloudflare
  domain_id: '',
  file_name: '',
  file: null,
})
const addDomainFormRef = ref(null)
const addDomainFileList = ref([])
const addDomainSelectedFile = ref(null)
const addDomainUploadRef = ref(null)
const availableDomains = ref([]) // 可用于添加的域名列表

// 添加文件到已有域名
const showAddFileDialog = ref(false)
const addFileLoading = ref(false)
const addFileForm = ref({
  domain_id: '',
  domain_name: '',
  file: null,
})
const addFileFormRef = ref(null)
const addFileList = ref([])
const addFileSelectedFiles = ref([])
const addFileUploadProgressMap = ref({})
const addFileUploadRef = ref(null)

// 检查修复相关
const showCheckDialog = ref(false)
const checkStatus = ref(null)
const checkPackageId = ref(null)
const fixLoading = ref(false)

// 批量删除相关
const selectedFiles = ref([])
const batchDeleting = ref(false)

const addDomainRules = {
  domain_id: [{ required: true, message: '请选择下载域名', trigger: 'change' }],
}

const addFileRules = {
  file: [
    {
      required: true,
      message: '请选择APK文件',
      trigger: 'change',
      validator: (rule, value, callback) => {
        if (!addFileSelectedFiles.value || addFileSelectedFiles.value.length === 0) {
          callback(new Error('请选择APK文件'))
        } else {
          callback()
        }
      },
    },
  ],
}

// 加载下载域名列表（只加载域名基本信息）
const loadDomains = async () => {
  loading.value = true
  try {
    // 1. 获取所有下载包（只获取基本信息，不查询状态）
    // 为了正确分页域名，需要获取所有下载包来统计域名
    const packageParams = {
      page: 1,
      page_size: 10000, // 获取足够多的数据以统计所有域名
    }
    if (activeGroupId.value !== null) {
      packageParams.group_id = activeGroupId.value
    }
    if (searchKeyword.value && searchKeyword.value.trim()) {
      packageParams.search = searchKeyword.value.trim()
    }
    const packageResponse = await request.get('/download-packages', {
      params: packageParams,
    })
    const allPackages = packageResponse.data || []

    // 2. 按域名组织数据（只统计文件数量，不加载详细信息）
    // 下载包已经包含 domain_name，无需额外获取域名列表
    const domainMap = {}

    allPackages.forEach((pkg) => {
      if (!pkg.domain_id || !pkg.domain_name) {
        return
      }
      
      if (!domainMap[pkg.domain_id]) {
        domainMap[pkg.domain_id] = {
          id: pkg.domain_id,
          domain_name: pkg.domain_name,
          file_count: 0,
          status: null, // 将使用最新下载包的状态
        }
      }
      
      domainMap[pkg.domain_id].file_count++
      // 使用最新下载包的状态（按创建时间，如果status更"重要"则更新）
      if (!domainMap[pkg.domain_id].status || 
          (pkg.status && getStatusPriority(pkg.status) > getStatusPriority(domainMap[pkg.domain_id].status))) {
        domainMap[pkg.domain_id].status = pkg.status
      }
    })

    // 3. 将域名列表转换为数组并按域名名称排序
    const allDomains = Object.values(domainMap).sort((a, b) => {
      return a.domain_name.localeCompare(b.domain_name)
    })

    // 4. 设置总数
    totalDomains.value = allDomains.length

    // 5. 分页处理
    const start = (currentPage.value - 1) * pageSize.value
    const end = start + pageSize.value
    domainList.value = allDomains.slice(start, end)
  } catch (error) {
    ElMessage.error('加载下载域名列表失败: ' + (error.response?.data?.error || error.message))
  } finally {
    loading.value = false
  }
}

// 查看域名详情（加载详细信息）
const viewDomainDetails = async (domain) => {
  showDetailDialog.value = true
  detailLoading.value = true
  currentDomain.value = null
  selectedFiles.value = [] // 清空选择
  
  try {
    // 1. 获取域名下的所有下载包
    const packageResponse = await request.get('/download-packages/by-domain', {
      params: { domain_id: domain.id },
    })
    const packages = packageResponse.data || []

    // 2. 获取域名信息
    const domainResponse = await domainApi.getDomain(domain.id)
    const domainInfo = domainResponse

    // 3. 如果有下载包，获取第一个下载包的详细信息（包含状态检测）
    let packageDetail = null
    if (packages.length > 0) {
      try {
        packageDetail = await downloadPackageApi.getDownloadPackage(packages[0].id)
      } catch (error) {
        console.error('获取下载包详情失败:', error)
      }
    }

    // 4. 构建域名详情对象
    currentDomain.value = {
      id: domainInfo.id,
      domain_name: domainInfo.domain_name,
      domain_status: packageDetail?.domain_status || domainInfo.status,
      certificate_status: domainInfo.certificate_status,
      file_count: packages.length,
      files: packages,
      cloudfront_id: packages.length > 0 ? packages[0].cloudfront_id : null,
      cloudfront_domain: packages.length > 0 ? packages[0].cloudfront_domain : null,
      cloudfront_status: packageDetail?.cloudfront_status || (packages.length > 0 ? packages[0].cloudfront_status : null),
      cloudfront_enabled: packageDetail?.cloudfront_enabled !== undefined ? packageDetail.cloudfront_enabled : (packages.length > 0 ? packages[0].cloudfront_enabled : false),
      cloudfront_origin_path_current: packageDetail?.cloudfront_origin_path_current,
      cloudfront_origin_path_expected: packageDetail?.cloudfront_origin_path_expected,
      s3_bucket_policy_configured: packageDetail?.s3_bucket_policy_configured,
    }
  } catch (error) {
    ElMessage.error('加载域名详情失败: ' + (error.response?.data?.error || error.message))
    showDetailDialog.value = false
  } finally {
    detailLoading.value = false
  }
}

// 加载可用域名列表（用于添加新域名）
const loadAvailableDomains = async () => {
  try {
    // 使用轻量级接口，不查询证书状态，提升性能
    const response = await domainApi.getDomainListForSelect({ dns_provider: addDomainForm.value.dns_provider })
    // 过滤：AWS域名需要证书已签发，Cloudflare域名只需要域名状态为completed
    const allAvailable = (response || []).filter((d) => {
      // 必须未被重定向使用且未被下载包使用
      if (d.used_by_redirect || d.used_by_download_package) {
        return false
      }
      // Cloudflare域名：只需要域名状态为completed
      if (d.dns_provider === 'cloudflare') {
        return d.status === 'completed'
      }
      // AWS域名：需要证书已签发（使用数据库中的证书状态，不查询AWS）
      return d.certificate_status === 'issued'
    })
    
    // 排除已经在下载域名列表中的域名
    const existingDomainIds = new Set(domainList.value.map((d) => d.id))
    availableDomains.value = allAvailable.filter((d) => !existingDomainIds.has(d.id))
  } catch (error) {
    console.error('加载域名列表失败:', error)
  }
}

// 根据DNS提供商过滤可用域名
const filteredAvailableDomains = computed(() => {
  if (!addDomainForm.value.dns_provider) {
    return availableDomains.value
  }
  // 只显示匹配所选DNS提供商的域名
  return availableDomains.value.filter(
    (d) => d.dns_provider === addDomainForm.value.dns_provider
  )
})

// 打开添加下载域名对话框
const openAddDomainDialog = async () => {
  // 重置表单
  addDomainForm.value = {
    dns_provider: 'cloudflare',
    domain_id: '',
    file_name: '',
    file: null,
  }
  addDomainFileList.value = []
  addDomainSelectedFile.value = null
  
  // 加载可用域名列表
  await loadAvailableDomains()
  
  // 显示对话框
  showAddDomainDialog.value = true
}

// 处理添加域名时的文件选择
const handleAddDomainFileChange = (file) => {
  addDomainSelectedFile.value = file.raw
  addDomainForm.value.file_name = file.name
  addDomainForm.value.file = file.raw
}

// 添加下载域名
const handleAddDomain = async () => {
  if (!addDomainFormRef.value) return

  // 先进行表单验证
  try {
    await addDomainFormRef.value.validate()
  } catch (error) {
    return
  }

  // 如果没有选择文件，只添加域名（不创建下载包）
  if (!addDomainSelectedFile.value) {
    ElMessage.success('域名已添加，您可以稍后上传文件')
    showAddDomainDialog.value = false
    addDomainForm.value = {
      dns_provider: 'cloudflare',
      domain_id: '',
      file_name: '',
      file: null,
    }
    addDomainFileList.value = []
    addDomainSelectedFile.value = null
    loadDomains()
    loadAvailableDomains()
    return
  }

  // 如果有文件，上传文件（会自动创建域名关联）
  addDomainLoading.value = true
  addDomainUploadProgress.value = 0

  try {
    const formData = new FormData()
    formData.append('domain_id', addDomainForm.value.domain_id)
    formData.append('file_name', addDomainForm.value.file_name)
    formData.append('file', addDomainSelectedFile.value)

    await uploadFile(
      '/download-packages',
      formData,
      { timeout: 600000 },
      (progress) => {
        addDomainUploadProgress.value = progress
      }
    )

    ElMessage.success('上传成功，正在处理中...')
    showAddDomainDialog.value = false
    addDomainForm.value = {
      dns_provider: 'cloudflare',
      domain_id: '',
      file_name: '',
      file: null,
    }
    addDomainFileList.value = []
    addDomainSelectedFile.value = null
    addDomainUploadProgress.value = 0
    loadDomains()
    loadAvailableDomains()
  } catch (error) {
    ElMessage.error('上传失败: ' + (error.response?.data?.error || error.message))
  } finally {
    addDomainLoading.value = false
    addDomainUploadProgress.value = 0
  }
}

// 显示添加文件对话框
const openAddFileDialog = (domain) => {
  addFileForm.value = {
    domain_id: domain.id,
    domain_name: domain.domain_name,
    file: null,
  }
  addFileList.value = []
  addFileSelectedFiles.value = []
  addFileUploadProgressMap.value = {}
  showAddFileDialog.value = true
}

// 处理添加文件选择
const handleAddFileChange = (file, fileList) => {
  // 更新选中的文件列表
  addFileSelectedFiles.value = fileList.map(f => f.raw)
  // 手动触发表单验证
  if (addFileFormRef.value) {
    addFileFormRef.value.validateField('file')
  }
}

// 处理文件移除
const handleAddFileRemove = (file, fileList) => {
  // 更新选中的文件列表
  addFileSelectedFiles.value = fileList.map(f => f.raw)
  // 移除对应的进度
  if (addFileUploadProgressMap.value[file.name] !== undefined) {
    delete addFileUploadProgressMap.value[file.name]
  }
  // 手动触发表单验证
  if (addFileFormRef.value) {
    addFileFormRef.value.validateField('file')
  }
}

// 添加文件到已有域名
const handleAddFile = async () => {
  if (!addFileFormRef.value) return

  // 先进行表单验证
  try {
    await addFileFormRef.value.validate()
  } catch (error) {
    return
  }

  // 验证通过后，再次检查文件
  if (!addFileSelectedFiles.value || addFileSelectedFiles.value.length === 0) {
    ElMessage.warning('请选择文件')
    return
  }

  addFileLoading.value = true
  addFileUploadProgressMap.value = {}
  
  // 初始化所有文件的进度为0
  addFileSelectedFiles.value.forEach(file => {
    addFileUploadProgressMap.value[file.name] = 0
  })

  const totalFiles = addFileSelectedFiles.value.length
  let successCount = 0
  let failCount = 0
  const errors = []

  // 依次上传每个文件
  for (let i = 0; i < addFileSelectedFiles.value.length; i++) {
    const file = addFileSelectedFiles.value[i]
    
    try {
      const formData = new FormData()
      formData.append('domain_id', addFileForm.value.domain_id)
      formData.append('file_name', file.name)
      formData.append('file', file)

      await uploadFile(
        '/download-packages',
        formData,
        { timeout: 600000 },
        (progress) => {
          addFileUploadProgressMap.value[file.name] = progress
        }
      )

      addFileUploadProgressMap.value[file.name] = 100
      successCount++
    } catch (error) {
      const errorMsg = error.response?.data?.error || error.message
      errors.push(`${file.name}: ${errorMsg}`)
      failCount++
      // 即使失败也标记为100，表示该文件处理完成（虽然失败了）
      addFileUploadProgressMap.value[file.name] = 100
    }
  }

  // 显示结果
  if (successCount === totalFiles) {
    ElMessage.success(`所有文件上传成功，正在处理中...`)
    showAddFileDialog.value = false
    addFileForm.value = {
      domain_id: '',
      domain_name: '',
      file: null,
    }
    addFileList.value = []
    addFileSelectedFiles.value = []
    addFileUploadProgressMap.value = {}
    loadDomains()
  } else if (successCount > 0) {
    ElMessage.warning(`${successCount}个文件上传成功，${failCount}个文件上传失败`)
    errors.forEach(err => {
      ElMessage.error(err)
    })
    // 不关闭对话框，让用户看到失败的文件
  } else {
    ElMessage.error('所有文件上传失败')
    errors.forEach(err => {
      ElMessage.error(err)
    })
  }

  addFileLoading.value = false
}

// 处理表格选择变化
const handleSelectionChange = (selection) => {
  selectedFiles.value = selection
}

// 删除下载包
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个下载包吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })

    await request.delete(`/download-packages/${row.id}`)
    ElMessage.success('删除成功')
    
    // 刷新主列表
    loadDomains()
    
    // 如果详情对话框打开，刷新文件列表
    if (showDetailDialog.value && currentDomain.value) {
      await viewDomainDetails(currentDomain.value)
    }
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.response?.data?.error || error.message))
    }
  }
}

// 批量删除下载包
const handleBatchDelete = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请选择要删除的文件')
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${selectedFiles.value.length} 个文件吗？`,
      '批量删除确认',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )

    batchDeleting.value = true
    let successCount = 0
    let failCount = 0
    const errors = []

    // 使用for循环依次删除每个文件
    for (const file of selectedFiles.value) {
      try {
        await request.delete(`/download-packages/${file.id}`)
        successCount++
      } catch (error) {
        failCount++
        const errorMsg = error.response?.data?.error || error.message
        errors.push(`${file.file_name}: ${errorMsg}`)
      }
    }

    // 显示结果
    if (successCount > 0) {
      ElMessage.success(`成功删除 ${successCount} 个文件`)
    }
    if (failCount > 0) {
      ElMessage.error(`删除失败 ${failCount} 个文件`)
      errors.forEach(err => {
        ElMessage.error(err)
      })
    }

    // 清空选择
    selectedFiles.value = []
    
    // 刷新主列表
    loadDomains()
    
    // 如果详情对话框打开，刷新文件列表
    if (showDetailDialog.value && currentDomain.value) {
      await viewDomainDetails(currentDomain.value)
    }
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('批量删除失败: ' + (error.response?.data?.error || error.message))
    }
  } finally {
    batchDeleting.value = false
  }
}

// 复制下载链接
const copyDownloadUrl = (row) => {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(row.download_url).then(() => {
      ElMessage.success('链接已复制到剪贴板')
    })
  } else {
    // 降级方案
    const textarea = document.createElement('textarea')
    textarea.value = row.download_url
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
    ElMessage.success('链接已复制到剪贴板')
  }
}

// 复制文件列表
const copyFileList = () => {
  if (!currentDomain.value || !currentDomain.value.files || currentDomain.value.files.length === 0) {
    ElMessage.warning('没有可复制的文件')
    return
  }

  // 构建文件列表文本，格式：文件名: 下载URL
  const fileListText = currentDomain.value.files
    .map((file) => {
      if (file.download_url) {
        return `${file.download_url}`
      } else {
        return `${file.file_name}: (无下载URL)`
      }
    })
    .join('\n')

  // 复制到剪贴板
  if (navigator.clipboard) {
    navigator.clipboard.writeText(fileListText).then(() => {
      ElMessage.success(`已复制 ${currentDomain.value.files.length} 个文件信息到剪贴板`)
    })
  } else {
    // 降级方案
    const textarea = document.createElement('textarea')
    textarea.value = fileListText
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
    ElMessage.success(`已复制 ${currentDomain.value.files.length} 个文件信息到剪贴板`)
  }
}

// 复制所有文件列表（主列表）
const copyAllFileList = async () => {
  try {
    // 获取所有下载包
    const packageParams = {
      page: 1,
      page_size: 1000,
    }
    if (activeGroupId.value !== null) {
      packageParams.group_id = activeGroupId.value
    }
    const packageResponse = await request.get('/download-packages', {
      params: packageParams,
    })
    const allPackages = packageResponse.data || []

    if (allPackages.length === 0) {
      ElMessage.warning('没有可复制的文件')
      return
    }

    // 构建文件列表文本，格式：文件名: 下载URL
    const fileListText = allPackages
      .map((file) => {
        if (file.download_url) {
          return `${file.download_url}`
        } else {
          return `${file.file_name}: (无下载URL)`
        }
      })
      .join('\n')

    // 复制到剪贴板
    if (navigator.clipboard) {
      navigator.clipboard.writeText(fileListText).then(() => {
        ElMessage.success(`已复制 ${allPackages.length} 个文件信息到剪贴板`)
      })
    } else {
      // 降级方案
      const textarea = document.createElement('textarea')
      textarea.value = fileListText
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      ElMessage.success(`已复制 ${allPackages.length} 个文件信息到剪贴板`)
    }
  } catch (error) {
    ElMessage.error('获取文件列表失败: ' + (error.response?.data?.error || error.message))
  }
}

// 检查下载包
const checkPackage = async (row) => {
  row.checking = true
  checkPackageId.value = row.id
  try {
    const res = await downloadPackageApi.checkDownloadPackage(row.id)
    checkStatus.value = res
    showCheckDialog.value = true
  } catch (error) {
    ElMessage.error('检查失败: ' + (error.response?.data?.error || error.message))
  } finally {
    row.checking = false
  }
}

// 修复下载包
const fixPackage = async (row) => {
  try {
    await ElMessageBox.confirm(
      '确定要修复这个下载包吗？修复将重新创建CloudFront分发和DNS记录。',
      '提示',
      {
        type: 'warning',
      }
    )
    row.fixing = true

    // 如果还没有检查状态，先检查一下
    if (!checkStatus.value || checkPackageId.value !== row.id) {
      await checkPackage(row)
    }

    await downloadPackageApi.fixDownloadPackage(row.id)
    ElMessage.success('修复成功')

    // 重新检查状态
    if (checkPackageId.value === row.id) {
      const res = await downloadPackageApi.checkDownloadPackage(row.id)
      checkStatus.value = res
    }

    loadDomains()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('修复失败: ' + (error.response?.data?.error || error.message))
    }
  } finally {
    row.fixing = false
  }
}

// 在检查对话框中点击修复
const handleFix = async () => {
  if (!checkPackageId.value) return
  
  // 创建一个简单的对象，包含 id 和 fixing 属性
  const row = {
    id: checkPackageId.value,
    fixing: false,
  }
  
  await fixPackage(row)
}

// 格式化文件大小
const formatFileSize = (bytes) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i]
}

// 格式化日期
const formatDate = (dateString) => {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString('zh-CN')
}

// 获取状态类型
const getStatusType = (status) => {
  const statusMap = {
    pending: 'info',
    uploading: 'warning',
    processing: 'warning',
    completed: 'success',
    failed: 'danger',
  }
  return statusMap[status] || 'info'
}

// 获取状态文本
const getStatusText = (status) => {
  const statusMap = {
    pending: '待处理',
    uploading: '上传中',
    processing: '处理中',
    completed: '已完成',
    failed: '失败',
  }
  return statusMap[status] || status
}

// 获取状态优先级（用于选择最重要的状态显示）
const getStatusPriority = (status) => {
  const priorityMap = {
    failed: 5,
    processing: 4,
    uploading: 3,
    pending: 2,
    completed: 1,
  }
  return priorityMap[status] || 0
}

// 获取CloudFront状态类型
const getCloudFrontStatusType = (status) => {
  const statusMap = {
    InProgress: 'warning',
    Deployed: 'success',
    Disabled: 'info',
  }
  return statusMap[status] || 'info'
}

// 获取CloudFront状态文本
const getCloudFrontStatusText = (status) => {
  const statusTextMap = {
    InProgress: '部署中',
    Deployed: '已部署',
    Disabled: '已禁用',
  }
  return statusTextMap[status] || status || '未知'
}

// 获取证书状态文本
const getCertificateStatusText = (status) => {
  const statusMap = {
    pending: '待签发',
    issued: '已签发',
    failed: '失败',
    pending_validation: '验证中',
  }
  return statusMap[status] || status || '未知'
}

// 获取域名状态类型
const getDomainStatusType = (status) => {
  const statusMap = {
    pending: 'info',
    in_progress: 'warning',
    completed: 'success',
    failed: 'danger',
  }
  return statusMap[status] || 'info'
}

// 获取域名状态文本
const getDomainStatusText = (status) => {
  const statusTextMap = {
    pending: '待转入',
    in_progress: '转入中',
    completed: '已完成',
    failed: '失败',
  }
  return statusTextMap[status] || status || '未知'
}

const loadGroups = async () => {
  try {
    // 使用优化接口，一次性获取分组列表和统计信息
    const res = await groupApi.getGroupListWithStats()
    groups.value = res
    // 设置每个分组的下载包数量
    for (const group of groups.value) {
      group.count = group.download_package_count || 0
    }
    // 计算全部数量
    totalAll.value = groups.value.reduce((sum, group) => sum + (group.download_package_count || 0), 0)
  } catch (error) {
    console.error('加载分组列表失败:', error)
    // 降级到普通接口
    try {
      const res = await groupApi.getGroupList()
      groups.value = res
      for (const group of groups.value) {
        group.count = 0
      }
      totalAll.value = 0
    } catch (fallbackError) {
      console.error('加载分组列表失败（降级方案）:', fallbackError)
    }
  }
}

const handleGroupChange = () => {
  currentPage.value = 1 // 切换分组时重置到第一页
  loadDomains()
}

const handleSearch = () => {
  // 清除之前的定时器
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  // 设置新的定时器，300ms后执行搜索
  searchTimer = setTimeout(() => {
    currentPage.value = 1 // 搜索时重置到第一页
    loadDomains()
  }, 300)
}

// 处理分页大小变化
const handleSizeChange = (newSize) => {
  pageSize.value = newSize
  currentPage.value = 1 // 改变每页数量时重置到第一页
  loadDomains()
}

// 处理当前页变化
const handleCurrentChange = (newPage) => {
  currentPage.value = newPage
  loadDomains()
}

onMounted(() => {
  loadGroups()
  loadDomains()
  loadAvailableDomains()
  // 每30秒刷新一次状态
  setInterval(() => {
    loadDomains()
  }, 30000)
})
</script>

<style scoped>
.download-package-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.domain-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.domain-card {
  margin-bottom: 0;
}

.domain-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.domain-info {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.domain-name {
  font-weight: 600;
  font-size: 16px;
}

.domain-actions {
  display: flex;
  gap: 8px;
}
</style>

