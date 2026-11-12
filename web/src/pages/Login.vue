<template>
  <div class="login-container">
    <h1>DBA</h1>
    <p class="muted">使用 Access Key 和验证器上的 6 位验证码登录。首次启动时，密钥写在服务器的 data/bootstrap.txt，加入验证器后请删掉该文件。</p>
    <label for="access-key">Access Key</label>
    <input id="access-key" v-model="accessKey" type="password" autocomplete="off" spellcheck="false" placeholder="dba_..." @keyup.enter="checkLogin" />
    <label for="code">验证码</label>
    <input id="code" v-model="code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="6 位数字" @keyup.enter="checkLogin" />
    <p v-if="error" class="error">{{ error }}</p>
    <button type="button" :disabled="pending" @click="checkLogin">{{ pending ? '正在验证…' : '进入' }}</button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import router from '../router'
import { isLogin, setToken } from '../store'

const accessKey = ref('')
const code = ref('')
const error = ref('')
const pending = ref(false)

async function checkLogin() {
  error.value = ''
  pending.value = true
  try {
    const res = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        access_key: accessKey.value.trim(),
        code: code.value.replace(/\s/g, ''),
      }),
    })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) {
      throw new Error(typeof data.message === 'string' ? data.message : '登录失败')
    }
    if (typeof data.token !== 'string' || !data.token) {
      throw new Error('登录失败')
    }
    setToken(data.token)
    isLogin.value = true
    accessKey.value = ''
    code.value = ''
    router.push('/home')
  } catch (err) {
    isLogin.value = false
    error.value = err instanceof Error ? err.message : '登录失败'
  } finally {
    pending.value = false
  }
}
</script>

<style scoped>
.login-container {
  width: min(440px, calc(100vw - 32px));
  margin: 12vh auto 0;
  padding: 28px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: #1b2028;
  border: 1px solid #2c3340;
  border-radius: 18px;
}

h1 { margin: 0; }

label { color: #b7c0ce; font-size: 13px; }

p { margin: 0; }

input {
  width: 100%;
  border: 1px solid #343c4a;
  background: #12161d;
  color: #e7ecf3;
  border-radius: 10px;
  padding: 10px 12px;
}
</style>
