<template>
  <div class="shell">
    <header v-if="route.path !== '/login'" class="topbar">
      <router-link class="brand" to="/home">DBA</router-link>
      <span class="brand-note">数据库空间监控</span>
      <span class="spacer"></span>
      <button v-if="isLogin" class="ghost" type="button" @click="exit">退出</button>
    </header>
    <main>
      <router-view></router-view>
    </main>
  </div>
</template>

<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { api } from './api'
import { isLogin, logout } from './store'

const route = useRoute()
const router = useRouter()

async function exit() {
  try {
    await api('/logout', { method: 'POST' })
  } catch {
    // 会话已经失效时直接回到登录页。
  }
  logout()
  router.push('/login')
}
</script>

<style>
:root {
  color: #e7ecf3;
  background: #12151b;
  font-family: "Segoe UI", "PingFang SC", "Noto Sans SC", sans-serif;
  font-synthesis: none;
  text-rendering: optimizeLegibility;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  min-height: 100vh;
  background:
    radial-gradient(1200px 500px at 10% -10%, rgba(61, 139, 253, 0.16), transparent 50%),
    #12151b;
}

button, input, textarea { font: inherit; }

a { color: inherit; }

button {
  border: 0;
  border-radius: 10px;
  padding: 10px 16px;
  background: #3d8bfd;
  color: white;
  cursor: pointer;
}

button:disabled {
  opacity: 0.55;
  cursor: wait;
}

button.ghost, button.danger {
  background: transparent;
  border: 1px solid #3a4250;
  color: #d5dbe6;
}

button.danger:hover { border-color: #f07178; color: #f07178; }

.shell { min-height: 100vh; }

.topbar {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 28px;
  border-bottom: 1px solid #262c36;
  background: rgba(18, 21, 27, 0.9);
  position: sticky;
  top: 0;
  z-index: 2;
}

.brand {
  text-decoration: none;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.brand-note { color: #8b95a7; font-size: 13px; }

.spacer { flex: 1; }

main { padding: 28px; }

.page {
  max-width: 1080px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.panel {
  background: #1b2028;
  border: 1px solid #2c3340;
  border-radius: 16px;
  padding: 18px;
}

.muted { color: #8b95a7; }

.error {
  color: #ffb4b4;
  background: rgba(240, 113, 120, 0.12);
  border-radius: 10px;
  padding: 8px 10px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}

.field span { color: #b7c0ce; font-size: 13px; }

input, textarea {
  width: 100%;
  border: 1px solid #343c4a;
  background: #12161d;
  color: #e7ecf3;
  border-radius: 10px;
  padding: 10px 12px;
}

textarea { min-height: 84px; resize: vertical; }

.row { display: flex; gap: 10px; align-items: center; }

.pill {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 12px;
  border: 1px solid transparent;
}

.pill.online { color: #9af0c5; background: rgba(61, 214, 140, 0.12); }
.pill.offline { color: #ffb4b4; background: rgba(240, 113, 120, 0.12); }
.pill.unknown { color: #d5dbe6; background: #2a3140; }
.pill.engine { color: #b9d4ff; background: rgba(61, 139, 253, 0.12); }

@media (max-width: 720px) {
  main, .topbar { padding-left: 14px; padding-right: 14px; }
  .brand-note { display: none; }
}
</style>
