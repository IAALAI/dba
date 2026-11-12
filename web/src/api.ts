import { isLogin, token } from './store'

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (token.value) {
    headers.set('Authorization', `Bearer ${token.value}`)
  }
  const res = await fetch(path.startsWith('/api') ? path : `/api${path}`, {
    ...init,
    headers,
  })
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { message: text }
    }
  }
  if (res.status === 401) {
    token.value = ''
    isLogin.value = false
    localStorage.removeItem('token')
    if (!location.hash.startsWith('#/login')) location.hash = '#/login'
    throw new Error(
      typeof data === 'object' && data && 'message' in data
        ? String((data as { message: unknown }).message)
        : '登录已失效',
    )
  }
  if (!res.ok) {
    const message =
      typeof data === 'object' && data && 'message' in data
        ? String((data as { message: unknown }).message)
        : res.statusText
    throw new Error(message || '请求失败')
  }
  return data as T
}
