<template>
  <div class="positions-container">
    <!-- Сводная строка -->
    <div class="summary-row">
      <div class="summary-item">
        <span>{{ positions.length }} [</span>
        <span class="profit-count">{{ profitableCount }}</span>
        <span>/</span>
        <span class="loss-count">{{ lossCount }}</span>
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
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { GetPositions } from '../../wailsjs/go/main/App'
import type { exchange } from '../../wailsjs/go/models'

type Position = exchange.Position

const positions = ref<Position[]>([])
let intervalId: number | null = null

// Компьютеды
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
    
    positions.value = rawPositions.sort((a, b) => {
      const timeA = parseInt(a.CreatedTime) || 0
      const timeB = parseInt(b.CreatedTime) || 0
      return timeB - timeA
    })
  } catch (error) {
    console.error('Ошибка при загрузке позиций:', error)
    positions.value = []
  }
}

const getPnlClass = (pnl: number) => {
  if (pnl > 0) return 'profit'
  if (pnl < 0) return 'loss'
  return ''
}

const getShortSymbol = (symbol: string) => {
  return symbol ? symbol.replace(/USDT$/, '') : ''
}

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

onMounted(() => {
  loadPositions()
  intervalId = window.setInterval(loadPositions, 5000)
})

onUnmounted(() => {
  if (intervalId) clearInterval(intervalId)
})
</script>

<style scoped>
.positions-container {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.summary-row {
  display: flex;
  justify-content: space-between;
  margin-bottom: 2px;
  padding: 1px 4px;
  border-radius: 2px;
  font-weight: bold;
  border-bottom: 1px solid #e0e0e0;
  min-height: 14px;
}

.summary-item {
  display: flex;
  align-items: center;
}

.positions-list {
  flex: 1;
  overflow-y: auto;
}

.position-item {
  display: flex;
  justify-content: space-between;
  padding: 2px 4px;
  margin-bottom: 1px;
  border-radius: 2px;
}

.position-item:hover {
  background: #f0f0f0;
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

.profit-count,
.position-pnl.profit,
.summary-item.profit {
  color: #00a86b;
}

.loss-count,
.position-pnl.loss,
.summary-item.loss {
  color: #ff4444;
}

.no-positions {
  text-align: center;
  color: #666;
  font-style: italic;
  padding: 8px;
}
</style>