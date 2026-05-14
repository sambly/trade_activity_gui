<template>
  <div class="control-panel">
    <!-- Кружочек-индикатор статуса -->
    <div 
      class="status-dot"
      :class="statusClass"
      :title="statusTitle"
      @click="handleClick"
    />
    <!-- Кнопка переключения виджета TradingView -->
    <button 
      class="widget-toggle-btn"
      :title="widgetButtonTitle"
      @click="$emit('toggle-widget')"
    >
      📊
    </button>
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

const widgetButtonTitle = computed(() => 'Переключить виджет')

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
  (e: 'toggle-widget'): void
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

.widget-toggle-btn {
  width: 10px;
  height: 10px;
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

.widget-toggle-btn:hover {
  transform: scale(1.2);
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