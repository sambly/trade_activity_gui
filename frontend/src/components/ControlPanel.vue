<template>
  <div class="control-panel">
    <!-- Просто кружочек-индикатор -->
    <div 
      class="status-dot"
      :class="statusClass"
      :title="statusTitle"
      @click="handleClick"
    />
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

// Wails импорты
import { GetConnectionStatus} from '../../wailsjs/go/main/App'

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
</script>

<style scoped>
.control-panel {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: flex-start;
  justify-content: center;
}

.status-dot {
  width: 10px;
  height: 10px;
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