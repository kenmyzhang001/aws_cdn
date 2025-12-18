<template>
  <div class="login-container">
    <el-card class="login-card">
      <template #header>
        <div class="card-header">
          <h2>AWS CDN 管理平台</h2>
          <p>请登录您的账户</p>
        </div>
      </template>
      <el-form
        ref="loginFormRef"
        :model="loginForm"
        :rules="loginRules"
        label-width="80px"
      >
        <el-form-item label="用户名" prop="username">
          <el-input
            v-model="loginForm.username"
            placeholder="请输入用户名"
            clearable
          />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="loginForm.password"
            type="password"
            placeholder="请输入密码"
            show-password
            clearable
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item label="验证码" prop="otpCode" v-if="needOTP">
          <el-input
            v-model="loginForm.otpCode"
            placeholder="请输入谷歌验证码（如已启用）"
            clearable
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            @click="handleLogin"
            style="width: 100%"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import request from '@/api/request'

const router = useRouter()
const loginFormRef = ref(null)
const loading = ref(false)
const needOTP = ref(false)

const loginForm = reactive({
  username: '',
  password: '',
  otpCode: '',
})

const loginRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
  ],
}

const handleLogin = async () => {
  if (!loginFormRef.value) return
  
  await loginFormRef.value.validate(async (valid) => {
    if (!valid) return

    loading.value = true
    try {
      const response = await request.post('/auth/login', {
        username: loginForm.username,
        password: loginForm.password,
        otp_code: loginForm.otpCode || undefined,
      })

      // 保存token到localStorage
      if (response.token) {
        localStorage.setItem('token', response.token)
        localStorage.setItem('username', response.username || loginForm.username)
        ElMessage.success('登录成功')
        // 跳转到首页
        router.push('/domains')
      }
    } catch (error) {
      // 如果错误信息提示需要OTP，显示OTP输入框
      if (error.response?.data?.error?.includes('验证码') || 
          error.response?.data?.error?.includes('OTP') ||
          error.response?.data?.error?.includes('两步验证')) {
        needOTP.value = true
      }
      // 错误消息已经在request拦截器中显示了
    } finally {
      loading.value = false
    }
  })
}
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
}

.card-header {
  text-align: center;
}

.card-header h2 {
  margin: 0 0 8px 0;
  color: #303133;
}

.card-header p {
  margin: 0;
  color: #909399;
  font-size: 14px;
}
</style>

