<template>
  <div class="page">
    <router-link class="back" to="/home">← 返回列表</router-link>
    <p v-if="loadError" class="error">{{ loadError }}</p>
    <template v-else-if="item">
      <section class="panel head">
        <div>
          <h1>{{ item.name }}</h1>
          <p class="endpoint">{{ item.endpoint }}</p>
          <div class="meta">
            <span class="pill engine">{{ item.type_name }}</span>
            <span class="pill" :class="item.status">{{ statusText(item.status) }}</span>
          </div>
        </div>
        <div class="figure">
          <strong>{{ formatBytes(item.bytes) }}</strong>
          <span class="muted">最近采集 {{ formatTime(item.last_at) }}</span>
        </div>
      </section>

      <section class="panel">
        <div class="section-title">
          <h2>最近趋势</h2>
          <button type="button" class="ghost" :disabled="probing" @click="probe">{{ probing ? '采集中…' : '立即采集' }}</button>
        </div>
        <p v-if="actionError" class="error">{{ actionError }}</p>
        <TrendChart :points="points" />
        <div v-if="item.options.length" class="chips">
          <span v-for="option in item.options" :key="option">{{ option }}</span>
        </div>
        <p v-if="item.error" class="warn">{{ item.error }}</p>
      </section>

      <section class="panel">
        <h2>采样记录</h2>
        <p v-if="points.length === 0" class="muted">还没有记录。服务每小时自动采集一次。</p>
        <table v-else>
          <thead>
            <tr>
              <th>时间</th>
              <th>状态</th>
              <th>占用</th>
              <th>说明</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="point in reversed" :key="point.at">
              <td>{{ formatTime(point.at) }}</td>
              <td>{{ point.online ? '在线' : '离线' }}</td>
              <td>{{ formatBytes(point.bytes) }}</td>
              <td>{{ point.error || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <details class="panel">
        <summary>修改连接</summary>
        <label class="field">
          <span>名称</span>
          <input v-model="editName" />
        </label>
        <label class="field">
          <span>连接串</span>
          <textarea v-model="editDsn" spellcheck="false"></textarea>
        </label>
        <p v-if="saveError" class="error">{{ saveError }}</p>
        <button type="button" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存并重新检查' }}</button>
      </details>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import TrendChart from '../components/TrendChart.vue'
import type { StatPoint, Target } from '../model/item'
import { api } from '../api'
import { formatBytes, formatTime, statusText } from '../format'

const route = useRoute()
const item = ref<Target | null>(null)
const points = ref<StatPoint[]>([])
const loadError = ref('')
const actionError = ref('')
const saveError = ref('')
const probing = ref(false)
const saving = ref(false)
const editName = ref('')
const editDsn = ref('')

const reversed = computed(() => [...points.value].reverse())

async function load() {
  loadError.value = ''
  const id = String(route.params.id || '')
  try {
    const [detail, history] = await Promise.all([
      api<Target>(`/item/${id}`),
      api<StatPoint[]>(`/item/${id}/stats?limit=168`),
    ])
    item.value = { ...detail, options: detail.options ?? [] }
    points.value = Array.isArray(history) ? history : []
    editName.value = detail.name
    editDsn.value = detail.dsn || ''
  } catch (err) {
    item.value = null
    loadError.value = err instanceof Error ? err.message : '加载失败'
  }
}

async function probe() {
  if (!item.value) return
  probing.value = true
  actionError.value = ''
  try {
    item.value = await api<Target>(`/item/${item.value.id}/probe`, { method: 'POST' })
    points.value = await api<StatPoint[]>(`/item/${item.value.id}/stats?limit=168`)
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : '采集失败'
  } finally {
    probing.value = false
  }
}

async function save() {
  if (!item.value) return
  saving.value = true
  saveError.value = ''
  try {
    item.value = await api<Target>(`/item/${item.value.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: editName.value.trim(), dsn: editDsn.value.trim() }),
    })
    editName.value = item.value.name
    editDsn.value = item.value.dsn || editDsn.value
    points.value = await api<StatPoint[]>(`/item/${item.value.id}/stats?limit=168`)
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : '保存失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
watch(() => route.params.id, load)
</script>

<style scoped>
.back { color: #9eb6db; text-decoration: none; }

.head, .section-title, .meta {
  display: flex;
  gap: 12px;
  align-items: center;
}

.head, .section-title { justify-content: space-between; align-items: flex-start; }

h1, h2 { margin: 0; }

h2 { font-size: 18px; }

.endpoint { margin: 6px 0; overflow-wrap: anywhere; }

.figure {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.figure strong { font-size: 32px; font-variant-numeric: tabular-nums; }

.chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }

.chips span {
  font-size: 12px;
  color: #c5cedb;
  background: #12161d;
  border-radius: 999px;
  padding: 3px 8px;
}

.warn { color: #ffb4b4; overflow-wrap: anywhere; }

table { width: 100%; border-collapse: collapse; font-size: 14px; }

th, td {
  text-align: left;
  padding: 8px 6px;
  border-bottom: 1px solid #2c3340;
  vertical-align: top;
}

th { color: #8b95a7; font-weight: 600; }

details summary { cursor: pointer; margin-bottom: 12px; }

@media (max-width: 720px) {
  .head { flex-direction: column; }
  .figure { align-items: flex-start; }
}
</style>
