<template>
  <div class="custom-download-link-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>自定义下载链接管理</span>
          <div style="display: flex; gap: 8px; align-items: center">
            <el-button
              type="danger"
              size="default"
              @click="handleBatchDelete"
              :disabled="selectedLinks.length === 0"
            >
              <el-icon><Delete /></el-icon>
              批量删除
            </el-button>
            <el-button type="primary" @click="openBatchAddDialog">
              <el-icon><Plus /></el-icon>
              批量添加链接
            </el-button>
            <el-button type="success" @click="openAddDialog">
              <el-icon><Plus /></el-icon>
              添加单个链接
            </el-button>
          </div>
        </div>
      </template>

      <!-- 搜索和筛选 -->
      <div style="margin-bottom: 20px; display: flex; gap: 12px; align-items: center;">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索链接、名称或描述..."
          clearable
          style="width: 300px"
          @input="handleSearch"
          @clear="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
        
        <el-select
          v-model="statusFilter"
          placeholder="状态筛选"
          clearable
          style="width: 150px"
          @change="loadLinks"
        >
          <el-option label="全部" value="" />
          <el-option label="启用" value="active" />
          <el-option label="禁用" value="inactive" />
        </el-select>
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

      <!-- 链接列表 -->
      <div v-loading="loading">
        <el-table
          v-if="linkList.length > 0"
          :data="linkList"
          stripe
          border
          @selection-change="handleSelectionChange"
        >
          <el-table-column type="selection" width="55" />
          <el-table-column prop="name" label="名称" width="150">
            <template #default="{ row }">
              <span v-if="row.name">{{ row.name }}</span>
              <span v-else style="color: #c0c4cc; font-size: 12px;">未命名</span>
            </template>
          </el-table-column>
          <el-table-column prop="url" label="链接URL" min-width="300">
            <template #default="{ row }">
              <div style="display: flex; align-items: center; gap: 8px;">
                <a :href="row.url" target="_blank" style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #409EFF; text-decoration: none;" :title="row.url">
                  {{ row.url }}
                </a>
                <el-button
                  size="small"
                  type="text"
                  @click="copyLink(row.url)"
                  style="padding: 0; min-height: auto;"
                >
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="description" label="描述" width="200">
            <template #default="{ row }">
              <span v-if="row.description" :title="row.description" style="display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ row.description }}</span>
              <span v-else style="color: #c0c4cc; font-size: 12px;">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="group_name" label="分组" width="120">
            <template #default="{ row }">
              <el-tag v-if="row.group_name" size="small">{{ row.group_name }}</el-tag>
              <span v-else style="color: #c0c4cc; font-size: 12px;">未分组</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
                {{ row.status === 'active' ? '启用' : '禁用' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="click_count" label="点击次数" width="100">
            <template #default="{ row }">
              <el-tag size="small" type="warning">{{ row.click_count || 0 }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="创建时间" width="160">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="操作" width="200" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="primary" @click="openEditDialog(row)">
                编辑
              </el-button>
              <el-button size="small" type="success" @click="handleClick(row)">
                访问
              </el-button>
              <el-button size="small" type="danger" @click="handleDelete(row)">
                删除
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-empty v-else description="暂无自定义下载链接，请先添加链接" />

        <!-- 分页组件 -->
        <div v-if="total > 0" style="margin-top: 20px; display: flex; justify-content: flex-end">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="total"
            layout="total, sizes, prev, pager, next, jumper"
            @size-change="handleSizeChange"
            @current-change="handleCurrentChange"
          />
        </div>
      </div>
    </el-card>

    <!-- 添加单个链接对话框 -->
    <el-dialog v-model="showAddDialog" title="添加自定义下载链接" width="600px">
      <el-form :model="linkForm" label-width="100px" :rules="linkRules" ref="linkFormRef">
        <el-form-item label="链接URL" prop="url">
          <el-input v-model="linkForm.url" placeholder="请输入下载链接URL" />
        </el-form-item>
        <el-form-item label="链接名称">
          <el-input v-model="linkForm.name" placeholder="请输入链接名称（可选）" />
        </el-form-item>
        <el-form-item label="链接描述">
          <el-input
            v-model="linkForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入链接描述（可选）"
          />
        </el-form-item>
        <el-form-item label="所属分组">
          <el-select v-model="linkForm.group_id" placeholder="请选择分组（可选）" clearable style="width: 100%">
            <el-option
              v-for="group in groups"
              :key="group.id"
              :label="group.name"
              :value="group.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="linkForm.status">
            <el-radio label="active">启用</el-radio>
            <el-radio label="inactive">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAdd" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 批量添加链接对话框 -->
    <el-dialog v-model="showBatchAddDialog" title="批量添加自定义下载链接" width="700px">
      <el-form :model="batchForm" label-width="100px">
        <el-form-item label="下载链接" required>
          <el-input
            v-model="batchForm.urls"
            type="textarea"
            :rows="10"
            placeholder="请输入下载链接，每行一个链接，或使用逗号分隔多个链接&#10;例如：&#10;https://example.com/file1.apk&#10;https://example.com/file2.apk&#10;或&#10;https://example.com/file1.apk, https://example.com/file2.apk"
          />
          <div style="margin-top: 5px; color: #909399; font-size: 12px">
            支持换行符或逗号分隔多个链接
          </div>
        </el-form-item>
        <el-form-item label="所属分组">
          <el-select v-model="batchForm.group_id" placeholder="请选择分组（可选）" clearable style="width: 100%">
            <el-option
              v-for="group in groups"
              :key="group.id"
              :label="group.name"
              :value="group.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showBatchAddDialog = false">取消</el-button>
        <el-button type="primary" @click="handleBatchAdd" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 编辑链接对话框 -->
    <el-dialog v-model="showEditDialog" title="编辑自定义下载链接" width="600px">
      <el-form :model="editForm" label-width="100px" :rules="linkRules" ref="editFormRef">
        <el-form-item label="链接URL" prop="url">
          <el-input v-model="editForm.url" placeholder="请输入下载链接URL" />
        </el-form-item>
        <el-form-item label="链接名称">
          <el-input v-model="editForm.name" placeholder="请输入链接名称（可选）" />
        </el-form-item>
        <el-form-item label="链接描述">
          <el-input
            v-model="editForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入链接描述（可选）"
          />
        </el-form-item>
        <el-form-item label="所属分组">
          <el-select v-model="editForm.group_id" placeholder="请选择分组（可选）" clearable style="width: 100%">
            <el-option
              v-for="group in groups"
              :key="group.id"
              :label="group.name"
              :value="group.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="editForm.status">
            <el-radio label="active">启用</el-radio>
            <el-radio label="inactive">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="handleEdit" :loading="submitting">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Plus, Search, Delete, CopyDocument } from '@element-plus/icons-vue';
import {
  getCustomDownloadLinks,
  createCustomDownloadLink,
  batchCreateCustomDownloadLinks,
  updateCustomDownloadLink,
  deleteCustomDownloadLink,
  batchDeleteCustomDownloadLinks,
  incrementClickCount
} from '@/api/custom_download_link';
import { groupApi } from '@/api/group';

// 数据
const loading = ref(false);
const submitting = ref(false);
const linkList = ref([]);
const groups = ref([]);
const selectedLinks = ref([]);
const currentPage = ref(1);
const pageSize = ref(10);
const total = ref(0);
const totalAll = ref(0);
const searchKeyword = ref('');
const statusFilter = ref('');
const activeGroupId = ref(null);

// 对话框显示状态
const showAddDialog = ref(false);
const showBatchAddDialog = ref(false);
const showEditDialog = ref(false);

// 表单数据
const linkForm = reactive({
  url: '',
  name: '',
  description: '',
  group_id: null,
  status: 'active'
});

const batchForm = reactive({
  urls: '',
  group_id: null
});

const editForm = reactive({
  id: null,
  url: '',
  name: '',
  description: '',
  group_id: null,
  status: 'active'
});

// 表单引用
const linkFormRef = ref(null);
const editFormRef = ref(null);

// 表单验证规则
const linkRules = {
  url: [
    { required: true, message: '请输入链接URL', trigger: 'blur' }
  ]
};

// 加载分组列表
const loadGroups = async () => {
  try {
    const response = await groupApi.getGroupList();
    groups.value = response.data.data || [];
    
    // 加载每个分组的计数
    await loadGroupCounts();
  } catch (error) {
    console.error('加载分组列表失败:', error);
  }
};

// 加载分组计数
const loadGroupCounts = async () => {
  try {
    // 加载全部计数
    const allResponse = await getCustomDownloadLinks({
      page: 1,
      page_size: 1
    });
    totalAll.value = allResponse.data.total || 0;

    // 加载每个分组的计数
    for (const group of groups.value) {
      const response = await getCustomDownloadLinks({
        page: 1,
        page_size: 1,
        group_id: group.id
      });
      group.count = response.data.total || 0;
    }
  } catch (error) {
    console.error('加载分组计数失败:', error);
  }
};

// 加载链接列表
const loadLinks = async () => {
  loading.value = true;
  try {
    const params = {
      page: currentPage.value,
      page_size: pageSize.value
    };
    
    if (activeGroupId.value) {
      params.group_id = activeGroupId.value;
    }
    
    if (searchKeyword.value) {
      params.search = searchKeyword.value;
    }
    
    if (statusFilter.value) {
      params.status = statusFilter.value;
    }
    
    const response = await getCustomDownloadLinks(params);
    linkList.value = response.data.data || [];
    total.value = response.data.total || 0;
  } catch (error) {
    ElMessage.error('加载链接列表失败: ' + (error.response?.data?.error || error.message));
  } finally {
    loading.value = false;
  }
};

// 搜索处理
const handleSearch = () => {
  currentPage.value = 1;
  loadLinks();
};

// 分组切换
const handleGroupChange = () => {
  currentPage.value = 1;
  loadLinks();
};

// 分页处理
const handleSizeChange = () => {
  currentPage.value = 1;
  loadLinks();
};

const handleCurrentChange = () => {
  loadLinks();
};

// 选择处理
const handleSelectionChange = (selection) => {
  selectedLinks.value = selection;
};

// 打开添加对话框
const openAddDialog = () => {
  Object.assign(linkForm, {
    url: '',
    name: '',
    description: '',
    group_id: activeGroupId.value,
    status: 'active'
  });
  showAddDialog.value = true;
};

// 打开批量添加对话框
const openBatchAddDialog = () => {
  Object.assign(batchForm, {
    urls: '',
    group_id: activeGroupId.value
  });
  showBatchAddDialog.value = true;
};

// 打开编辑对话框
const openEditDialog = (row) => {
  Object.assign(editForm, {
    id: row.id,
    url: row.url,
    name: row.name,
    description: row.description,
    group_id: row.group_id,
    status: row.status
  });
  showEditDialog.value = true;
};

// 添加链接
const handleAdd = async () => {
  try {
    await linkFormRef.value.validate();
    submitting.value = true;
    
    await createCustomDownloadLink(linkForm);
    ElMessage.success('添加成功');
    showAddDialog.value = false;
    await loadLinks();
    await loadGroupCounts();
  } catch (error) {
    if (error !== false) { // 不是验证错误
      ElMessage.error('添加失败: ' + (error.response?.data?.error || error.message));
    }
  } finally {
    submitting.value = false;
  }
};

// 批量添加链接
const handleBatchAdd = async () => {
  if (!batchForm.urls.trim()) {
    ElMessage.warning('请输入至少一个链接');
    return;
  }
  
  try {
    submitting.value = true;
    const response = await batchCreateCustomDownloadLinks(batchForm);
    ElMessage.success(`批量添加成功，共添加 ${response.data.count} 个链接`);
    showBatchAddDialog.value = false;
    await loadLinks();
    await loadGroupCounts();
  } catch (error) {
    ElMessage.error('批量添加失败: ' + (error.response?.data?.error || error.message));
  } finally {
    submitting.value = false;
  }
};

// 编辑链接
const handleEdit = async () => {
  try {
    await editFormRef.value.validate();
    submitting.value = true;
    
    const { id, ...data } = editForm;
    await updateCustomDownloadLink(id, data);
    ElMessage.success('更新成功');
    showEditDialog.value = false;
    await loadLinks();
  } catch (error) {
    if (error !== false) { // 不是验证错误
      ElMessage.error('更新失败: ' + (error.response?.data?.error || error.message));
    }
  } finally {
    submitting.value = false;
  }
};

// 删除链接
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个链接吗？', '提示', {
      type: 'warning'
    });
    
    await deleteCustomDownloadLink(row.id);
    ElMessage.success('删除成功');
    await loadLinks();
    await loadGroupCounts();
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + (error.response?.data?.error || error.message));
    }
  }
};

// 批量删除
const handleBatchDelete = async () => {
  if (selectedLinks.value.length === 0) {
    ElMessage.warning('请先选择要删除的链接');
    return;
  }
  
  try {
    await ElMessageBox.confirm(`确定要删除选中的 ${selectedLinks.value.length} 个链接吗？`, '提示', {
      type: 'warning'
    });
    
    const ids = selectedLinks.value.map(link => link.id);
    await batchDeleteCustomDownloadLinks(ids);
    ElMessage.success(`批量删除成功，共删除 ${ids.length} 个链接`);
    selectedLinks.value = [];
    await loadLinks();
    await loadGroupCounts();
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('批量删除失败: ' + (error.response?.data?.error || error.message));
    }
  }
};

// 访问链接（并增加点击次数）
const handleClick = async (row) => {
  try {
    await incrementClickCount(row.id);
    window.open(row.url, '_blank');
    // 刷新列表以更新点击次数
    await loadLinks();
  } catch (error) {
    console.error('更新点击次数失败:', error);
    // 即使失败也打开链接
    window.open(row.url, '_blank');
  }
};

// 复制链接
const copyLink = (url) => {
  navigator.clipboard.writeText(url).then(() => {
    ElMessage.success('链接已复制到剪贴板');
  }).catch(() => {
    ElMessage.error('复制失败');
  });
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
    minute: '2-digit'
  });
};

// 组件挂载时加载数据
onMounted(() => {
  loadGroups();
  loadLinks();
});
</script>

<style scoped>
.custom-download-link-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.el-table {
  margin-top: 20px;
}
</style>
