import { ref } from 'vue'

const STORAGE_KEY = 'theme'

// Реактивное состояние объявлено на уровне модуля (не внутри useTheme()), поэтому все компоненты,
// вызывающие useTheme(), делят один и тот же isDark — простой способ получить общее состояние
// темы без Pinia/Vuex.
const isDark = ref(localStorage.getItem(STORAGE_KEY) === 'dark')

const applyTheme = () => {
  document.documentElement.classList.toggle('dark', isDark.value)
}

applyTheme()

export function useTheme() {
  const toggleTheme = () => {
    isDark.value = !isDark.value
    localStorage.setItem(STORAGE_KEY, isDark.value ? 'dark' : 'light')
    applyTheme()
  }

  return { isDark, toggleTheme }
}
