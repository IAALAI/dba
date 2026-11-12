import { ref } from 'vue'

export const token = ref(localStorage.getItem('token') || '')
export const isLogin = ref(Boolean(token.value))

export function setToken(value: string) {
  token.value = value
  if (value) localStorage.setItem('token', value)
  else localStorage.removeItem('token')
}

export function logout() {
  setToken('')
  isLogin.value = false
}
