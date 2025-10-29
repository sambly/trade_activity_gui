<template>
   <main class="compact-container">
    <!-- Сводная строка -->
    <div class="summary-row">
      <div class="summary-item">
        <span>{{ positions.length }} [</span>
        <span class="color-green">{{ profitableCount }}</span>
        <span>/</span>
        <span class="color-red">{{ lossCount  }}</span>
        <span>]</span>
      </div>
      <div class="summary-item">
        <span>${{ formatNumber(totalValue) }}</span>
      </div>
      <div class="summary-item" :class="getPnlClass(totalUnrealisedPnl)">
        <span>${{ formatPnl(totalUnrealisedPnl) }}</span>
      </div>
    </div>

    <!-- Список позиций -->
    <div class="positions-list">
      <div 
        v-for="position in positions" 
        :key="position.Symbol + position.Side"
        class="position-item"
      >
        <div class="position-symbol">{{ getShortSymbol(position.Symbol) }}</div>
        <div class="position-size">{{ formatCompactNumber(position.CurrentValue) }}</div>
        <div class="position-pnl" :class="getPnlClass(position.UnrealisedPnl)">
          {{ formatPnl(position.UnrealisedPnl) }}
        </div>
      </div>
      
      <div v-if="positions.length === 0" class="no-positions">
        Нет сделок
      </div>
    </div>

  </main>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted} from 'vue'
import { GetPositions} from '../../wailsjs/go/main/App'
import type { exchange } from '../../wailsjs/go/models'

type Position = exchange.Position

const positions = ref<Position[]>([])
let intervalId: number | null = null

// Компьютеды для сводной информации
const profitableCount = computed(() => {
  return positions.value.filter(p => p.UnrealisedPnl > 0).length
})

const lossCount = computed(() => {
  return positions.value.filter(p => p.UnrealisedPnl < 0).length
})

const totalValue = computed(() => {
  return positions.value.reduce((sum, position) => sum + Math.abs(position.CurrentValue), 0)
})

const totalUnrealisedPnl = computed(() => {
  return positions.value.reduce((sum, position) => sum + position.UnrealisedPnl, 0)
})

const loadPositions = async () => {
  try {
    const result = await GetPositions()
    const rawPositions = result || []
    
    // Сортируем позиции по времени создания (от новых к старым)
    positions.value = rawPositions.sort((a, b) => {
      const timeA = parseInt(a.CreatedTime) || 0
      const timeB = parseInt(b.CreatedTime) || 0
      return timeB - timeA // от новых к старым
    })
    
  } catch (error) {
    console.error('Ошибка при загрузке позиций:', error)
    positions.value = []
  }
}

const getPnlClass = (pnl: number) => {
  if (pnl > 0) return 'color-green'
  if (pnl < 0) return 'color-red'
  return ''
}

// Сокращение символа (если нужно)
const getShortSymbol = (symbol: string) => {
  if (!symbol) return ''
  // Убираем USDT если он есть в конце
  return symbol.replace(/USDT$/, '')
}

// Форматирование чисел в компактном виде
const formatNumber = (value: number) => {
  if (value === undefined || value === null) return '0'
  return new Intl.NumberFormat('ru-RU', {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2
  }).format(value)
}

const formatCompactNumber = (value: number) => {
  if (value === undefined || value === null) return '0'
  const absValue = Math.abs(value)
  
  if (absValue >= 1000) {
    return (value / 1000).toFixed(1) + 'k'
  }
  
  return new Intl.NumberFormat('ru-RU', {
    minimumFractionDigits: 0,
    maximumFractionDigits: absValue < 1 ? 4 : 2
  }).format(value)
}

const formatPnl = (pnl: number) => {
  return formatNumber(pnl)
}

const formatCompactPnl = (pnl: number) => {
  const absPnl = Math.abs(pnl)
  let formattedPnl: string
  
  if (absPnl >= 1000) {
    formattedPnl = (pnl / 1000).toFixed(1) + 'k'
  } else if (absPnl >= 1) {
    formattedPnl = pnl.toFixed(1)
  } else if (absPnl >= 0.01) {
    formattedPnl = pnl.toFixed(3)
  } else {
    formattedPnl = pnl.toFixed(6)
  }
  
  return pnl >= 0 ? `+${formattedPnl}` : formattedPnl
}

onMounted(() => {
  loadPositions()
  intervalId = window.setInterval(() => {
    loadPositions()
  }, 5000)
})

onUnmounted(() => {
  if (intervalId) {
    clearInterval(intervalId)
  }
})
</script>

<style scoped>

/* Глобальное скрытие скроллбаров */
:global(::-webkit-scrollbar) {
  display: none;
}

:global(body) {
  overflow: hidden;
  -ms-overflow-style: none;  /* IE and Edge */
  scrollbar-width: none;  /* Firefox */
}

.compact-container {
  width: 130px;
  height: 50px;
  padding: 4px;
  font-family: Arial, sans-serif;
  font-size: 10px;
  background-color: #f8f9fa;
  border: 1px solid #e0e0e0;
  border-radius: 4px;
  display: flex;
  flex-direction: column;
}

/* Сводная строка */
.summary-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 4px;
  padding: 2px 4px;
  background-color: #fff;
  border-radius: 2px;
  font-weight: bold;
  border-bottom: 1px solid #e0e0e0;
}

.summary-item {
  display: flex;
  align-items: center;
}

/* Список позиций */
.positions-list {
  flex: 1;
  overflow-y: auto;
  max-height: 60px;
}

.position-item {
  display: flex;
  justify-content: space-between;
  padding: 2px 4px;
  margin-bottom: 1px;
  background-color: #fff;
  border-radius: 2px;
  font-size: 9px;
}

.position-item:hover {
  background-color: #f0f0f0;
}

.position-symbol {
  font-weight: bold;
  min-width: 40px;
  text-align: left;
}

.position-size {
  min-width: 40px;
  text-align: center;
}

.position-pnl {
  min-width: 40px;
  text-align: right;
  font-weight: bold;
}

.color-green {
  color: #00a86b;
}

.color-red {
  color: #ff4444;
}

.no-positions {
  text-align: center;
  color: #666;
  font-style: italic;
  padding: 8px;
  font-size: 9px;
}

/* Скрываем scrollbar для компактности */
.positions-list::-webkit-scrollbar {
  width: 3px;
}

.positions-list::-webkit-scrollbar-track {
  background: #f1f1f1;
}

.positions-list::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 2px;
}

.positions-list::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}
</style>