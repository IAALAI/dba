<template>
  <div class="chart">
    <svg v-if="geom" :viewBox="`0 0 ${geom.w} ${geom.h}`" role="img" aria-label="空间占用趋势">
      <line :x1="geom.padL" :y1="geom.top" :x2="geom.padL" :y2="geom.bottom" class="axis" />
      <line :x1="geom.padL" :y1="geom.bottom" :x2="geom.w - 16" :y2="geom.bottom" class="axis" />
      <text :x="geom.padL - 8" :y="geom.top + 4" text-anchor="end">{{ formatBytes(geom.max) }}</text>
      <text :x="geom.padL - 8" :y="geom.bottom" text-anchor="end">{{ formatBytes(geom.min) }}</text>
      <polygon v-if="geom.area" :points="geom.area" class="area" />
      <polyline v-if="geom.line" :points="geom.line" class="line" />
      <circle v-for="(point, index) in geom.coords" :key="index" :cx="point.x" :cy="point.y" r="3.5" class="dot" />
      <text :x="geom.coords[0].x" :y="geom.h - 6">{{ formatTime(geom.coords[0].p.at) }}</text>
      <text
        v-if="geom.coords.length > 1"
        :x="geom.coords[geom.coords.length - 1].x"
        :y="geom.h - 6"
        text-anchor="end"
      >{{ formatTime(geom.coords[geom.coords.length - 1].p.at) }}</text>
    </svg>
    <p v-else class="muted">还没有成功的空间采样。每小时会自动记一次，也可以现在立即采集。</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { StatPoint } from '../model/item'
import { formatBytes, formatTime } from '../format'

const props = defineProps<{ points: StatPoint[] }>()

const geom = computed(() => {
  const points = props.points.filter((point) => point.online)
  if (points.length === 0) return null
  const w = 720
  const h = 240
  const padL = 78
  const top = 16
  const bottom = h - 32
  const innerW = w - padL - 16
  const innerH = bottom - top
  const min = Math.min(...points.map((point) => point.bytes))
  const max = Math.max(...points.map((point) => point.bytes))
  const span = Math.max(max - min, 1)
  const coords = points.map((point, index) => {
    const x = padL + (points.length === 1 ? innerW / 2 : (index / (points.length - 1)) * innerW)
    const y = max === min ? top + innerH / 2 : top + (1 - (point.bytes - min) / span) * innerH
    return { x, y, p: point }
  })
  const line = coords.length > 1 ? coords.map((point) => `${point.x},${point.y}`).join(' ') : ''
  const area = line
    ? `${coords[0].x},${bottom} ${line} ${coords[coords.length - 1].x},${bottom}`
    : ''
  return { w, h, padL, top, bottom, min, max, coords, line, area }
})
</script>

<style scoped>
.chart { min-height: 180px; }

svg { width: 100%; height: auto; }

text { fill: #8b95a7; font-size: 12px; }

.axis { stroke: #343c4a; }

.area { fill: rgba(61, 139, 253, 0.18); }

.line { fill: none; stroke: #3d8bfd; stroke-width: 2.5; }

.dot { fill: #d6e6ff; }
</style>
