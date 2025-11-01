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

// Реактивные данные
const connectionStatus = ref<'connected' | 'disconnected' | 'error'>('connected')

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

// Имитация проверки соединения
const checkConnection = () => {
  const random = Math.random()
  if (random > 0.8) {
    connectionStatus.value = 'error'
  } else if (random > 0.6) {
    connectionStatus.value = 'disconnected'
  } else {
    connectionStatus.value = 'connected'
  }
}

// Клик по индикатору
const handleClick = () => {
  checkConnection() // При клике проверяем соединение
}

// Таймер для периодической проверки
let connectionTimer: number

onMounted(() => {
  connectionTimer = window.setInterval(checkConnection, 30000)
})

onUnmounted(() => {
  if (connectionTimer) {
    clearInterval(connectionTimer)
  }
})

// Экспортируем методы для внешнего использования
defineExpose({
  updateStatus: (status: 'connected' | 'disconnected' | 'error') => {
    connectionStatus.value = status
  },
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