<template>
  <article class="card" @click="open">
    <header>
      <h2>{{ item.name }}</h2>
      <span class="pill" :class="item.status">{{ statusText(item.status) }}</span>
    </header>
    <div class="meta">
      <span class="pill engine">{{ item.type_name }}</span>
      <span class="endpoint">{{ item.endpoint }}</span>
    </div>
    <p class="size">{{ formatBytes(item.bytes) }}</p>
    <p class="muted">最近采集 {{ formatTime(item.last_at) }}</p>
    <div v-if="item.options.length" class="chips">
      <span v-for="option in item.options" :key="option">{{ option }}</span>
    </div>
    <p v-if="item.error" class="warn">{{ item.error }}</p>
    <div class="actions">
      <button type="button" class="ghost" @click.stop="open">查看趋势</button>
      <button type="button" class="danger" @click.stop="remove">删除</button>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import type { Target } from '../model/item'
import { formatBytes, formatTime, statusText } from '../format'
import { api } from '../api'

const props = defineProps<{ item: Target }>()
const emit = defineEmits<{ removed: [] }>()
const router = useRouter()

function open() {
  router.push(`/target/${props.item.id}`)
}

async function remove() {
  if (!window.confirm(`停止监控「${props.item.name}」？`)) return
  try {
    await api(`/item/${props.item.id}`, { method: 'DELETE' })
    emit('removed')
  } catch (err) {
    window.alert(err instanceof Error ? err.message : '删除失败')
  }
}
</script>

<style scoped>
.card {
  background: #1b2028;
  border: 1px solid #2c3340;
  border-radius: 16px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  cursor: pointer;
}

header, .meta, .actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

header { justify-content: space-between; }

h2 { margin: 0; font-size: 18px; }

.endpoint {
  color: #d5dbe6;
  overflow-wrap: anywhere;
}

.size {
  margin: 4px 0 0;
  font-size: 28px;
  font-variant-numeric: tabular-nums;
}

.muted, .warn { margin: 0; }

.warn { color: #ffb4b4; font-size: 13px; overflow-wrap: anywhere; }

.chips { display: flex; flex-wrap: wrap; gap: 6px; }

.chips span {
  font-size: 12px;
  color: #c5cedb;
  background: #12161d;
  border-radius: 999px;
  padding: 3px 8px;
}

.actions { margin-top: 6px; }
</style>
