export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let value = n
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  const digits = value >= 100 || index === 0 ? 0 : 1
  return `${value.toFixed(digits)} ${units[index]}`
}

export function formatTime(unix: number): string {
  if (!unix) return '尚未采集'
  return new Date(unix * 1000).toLocaleString('zh-CN', { hour12: false })
}

export function statusText(status: string): string {
  if (status === 'online') return '在线'
  if (status === 'offline') return '离线'
  return '未知'
}
