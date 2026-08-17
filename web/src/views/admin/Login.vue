<!-- web/src/views/admin/Login.vue -->
<template>
  <div class="login-container">
    <div class="login-box">
      <h2>EPay 管理后台</h2>
      <a-form :model="form" @submit="handleSubmit" layout="vertical">
        <a-form-item field="username" label="用户名" :rules="[{ required: true, message: '请输入用户名' }]">
          <a-input v-model="form.username" placeholder="请输入用户名" size="large" />
        </a-form-item>
        <a-form-item field="password" label="密码" :rules="[{ required: true, message: '请输入密码' }]">
          <a-input-password v-model="form.password" placeholder="请输入密码" size="large" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" long size="large" :loading="loading">
            登录
          </a-button>
        </a-form-item>
      </a-form>
    </div>
    <BeianFooter />
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { adminLogin } from '@/api/admin'
import BeianFooter from '@/components/BeianFooter.vue'

const router = useRouter()
const route = useRoute()
const loading = ref(false)

const form = reactive({
  username: '',
  password: '',
})

const handleSubmit = async () => {
  loading.value = true
  try {
    const res = await adminLogin(form)
    localStorage.setItem('admin_token', res.data.token)
    localStorage.setItem('token', res.data.token)
    Message.success('登录成功')
    const redirect = route.query.redirect as string || '/admin/dashboard'
    router.push(redirect)
  } catch (e) {
    // error handled by interceptor
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.login-box {
  width: 400px;
  padding: 40px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
}
.login-box h2 {
  text-align: center;
  margin-bottom: 32px;
  color: #1d2129;
}
</style>
