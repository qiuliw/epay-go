<!-- web/src/views/merchant/Login.vue -->
<template>
  <div class="login-container">
    <div class="login-box">
      <h2>EPay 商户中心</h2>
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
        <div class="register-link">
          还没有账号？<router-link to="/merchant/register">立即注册</router-link>
        </div>
      </a-form>
    </div>
    <BeianFooter />
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { login } from '@/api/merchant'
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
    const res = await login(form)
    localStorage.setItem('merchant_token', res.data.token)
    localStorage.setItem('token', res.data.token)
    Message.success('登录成功')
    const redirect = route.query.redirect as string || '/merchant/dashboard'
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
  background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
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
.register-link {
  text-align: center;
  margin-top: 16px;
  color: #86909c;
}
.register-link a {
  color: #165dff;
}
</style>
