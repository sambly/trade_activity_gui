<template>
  <div class="control-panel">
    <!-- Кружочек-индикатор статуса -->
    <div 
      class="status-dot"
      :class="statusClass"
      :title="statusTitle"
      @click="handleClick"
    />
    <!-- Кнопка "назад к списку" — видна только когда открыта конкретная позиция -->
    <button
      v-if="viewMode !== 'list'"
      class="back-btn"
      title="Назад к списку"
      @click="$emit('go-back')"
    >
      ⬅
    </button>

    <!-- Кнопка переключения тёмной темы -->
    <button
      class="theme-toggle-btn"
      :title="isDark ? 'Светлая тема' : 'Тёмная тема'"
      @click="toggleTheme"
    >
      {{ isDark ? '☀️' : '🌙' }}
    </button>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useTheme } from '../composables/useTheme'

// Wails импорты
import { GetConnectionStatus} from '../../wailsjs/go/main/App'

const { isDark, toggleTheme } = useTheme()

withDefaults(defineProps<{
  viewMode?: 'list' | 'detail' | 'detail-chart'
}>(), {
  viewMode: 'list'
})

// Реактивные данные
const connectionStatus = ref<'connected' | 'disconnected' | 'error'>('connected')

// Преобразование статусов из Go в компонент
const mapGoStatus = (goStatus: string): 'connected' | 'disconnected' | 'error' => {
  switch (goStatus) {
    case 'connected': return 'connected'
    case 'connected': return 'disconnected'
    case 'error': return 'error'
    default: return 'disconnected'
  }
}

// Вычисляемые свойства
const statusClass = computed(() => `status-${connectionStatus.value}`)

const statusTitle = computed(() => {
  switch (connectionStatus.value) {
    case 'connected': return 'Соединение активно'
    case 'disconnected': return 'Соединение потеряно'
    case 'error': return 'Ошибка соединения'
    default: return 'Неизвестный статус'
  }
})

// Функция обновления статуса из Go
const updateStatusFromGo = async () => {
  try {
    const status = await GetConnectionStatus()
    connectionStatus.value = mapGoStatus(status)
  } catch (error) {
    console.error('Error getting connection status:', error)
    connectionStatus.value = 'error'
  }
}

// Клик по индикатору
const handleClick = async () => {
  await updateStatusFromGo()
}

// Таймер для периодического обновления
let statusTimer: number

onMounted(() => {
  statusTimer = window.setInterval(updateStatusFromGo, 5000)
})

onUnmounted(() => {
  if (statusTimer) {
    clearInterval(statusTimer)
  }
})

// Экспортируем методы для внешнего использования
defineExpose({
  updateStatus: updateStatusFromGo,
  getStatus: () => connectionStatus.value
})

defineEmits<{
  (e: 'go-back'): void
}>()
</script>

<style scoped>
.control-panel {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding-top: 1px;
}

.back-btn {
  width: 8px;
  height: 8px;
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 0;
  font-size: 8px;
  line-height: 1;
  color: var(--color-text);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.2s;
}

.back-btn:hover {
  transform: scale(1.2);
}

.theme-toggle-btn {
  width: 8px;
  height: 8px;
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 0;
  font-size: 8px;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.2s;
}

.theme-toggle-btn:hover {
  transform: scale(1.2);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  cursor: pointer;
  transition: all 0.3s ease;
}

.status-dot:hover {
  transform: scale(1.1);
}

/* Стили для разных статусов */
.status-connected {
  background: #28a745;
  box-shadow: 0 0 6px rgba(40, 167, 69, 0.5);
}

.status-disconnected {
  background: #ffc107;
  box-shadow: 0 0 6px rgba(255, 193, 7, 0.5);
}

.status-error {
  background: #dc3545;
  box-shadow: 0 0 6px rgba(220, 53, 69, 0.5);
}
</style>