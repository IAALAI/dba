<template>
  <div class="page">
    <section class="panel">
      <h1>添加监控</h1>
      <p class="muted">直接粘贴带参数的连接串。密码只保存在服务端，列表里不会显示。</p>
      <label class="field">
        <span>名称</span>
        <input v-model="name" placeholder="例如 主库" />
      </label>
      <label class="field">
        <span>连接串</span>
        <textarea v-model="dsn" spellcheck="false" placeholder="mysql://user:password@host:3306/dbname?ssl-mode=REQUIRED&charset=utf8mb4"></textarea>
      </label>
      <p class="hint">也支持 postgres://user:password@host:5432/dbname?sslmode=require。ssl-mode、超时、charset 这类参数会一起解析。</p>
      <p v-if="error" class="error">{{ error }}</p>
      <div class="row">
        <button type="button" :disabled="saving" @click="add">{{ saving ? '正在保存并检查…' : '添加并检查' }}</button>
        <button type="button" class="ghost" :disabled="refreshing" @click="refresh">{{ refreshing ? '采集中…' : '立即采集全部' }}</button>
      </div>
    </section>

    <p v-if="loadError" class="error">{{ loadError }}</p>
    <p v-else-if="items.length === 0" class="muted">还没有监控目标。</p>
    <div v-else class="grid">
      <ItemCard v-for="item in items" :key="item.id" :item="item" @removed="load" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import ItemCard from '../components/item_card.vue'
import type { Target } from '../model/item'
import { api } from '../api'

const items = ref<Target[]>([])
const name = ref('')
const dsn = ref('')
const error = ref('')
const loadError = ref('')
const saving = ref(false)
const refreshing = ref(false)

async function load() {
  loadError.value = ''
  try {
    const data = await api<Target[]>('/list')
    items.value = (Array.isArray(data) ? data : []).map((item) => ({ ...item, options: item.options ?? [] }))
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : '加载失败'
  }
}

async function add() {
  error.value = ''
  saving.value = true
  try {
    await api('/item', {
      method: 'POST',
      body: JSON.stringify({ name: name.value.trim(), dsn: dsn.value.trim() }),
    })
    name.value = ''
    dsn.value = ''
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '添加失败'
  } finally {
    saving.value = false
  }
}

async function refresh() {
  refreshing.value = true
  error.value = ''
  try {
    const data = await api<Target[]>('/refresh', { method: 'POST' })
    items.value = (Array.isArray(data) ? data : []).map((item) => ({ ...item, options: item.options ?? [] }))
  } catch (err) {
    error.value = err instanceof Error ? err.message : '采集失败'
  } finally {
    refreshing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
h1 { margin: 0 0 8px; font-size: 22px; }

.hint, .muted { margin: 0 0 12px; color: #8b95a7; font-size: 13px; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}
</style>
