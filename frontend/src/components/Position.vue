<template>
  <div class="positions-container">
    <!-- Сводная строка -->
    <div class="summary-row">
      <div class="col-symbol">
        <span>{{ positions.length }} [</span>
        <span class="profit-count">{{ profitableCount }}</span>
        <span>/</span>
        <span class="loss-count">{{ lossCount }}</span>
        <span>]</span>
      </div>
      <div class="col-size">
        <span>{{ formatNumber(totalValue) }}$</span>
      </div>
      <div class="col-pnl" :class="getPnlClass(totalUnrealisedPnl)">
        <span>{{ formatPnl(totalUnrealisedPnl) }}$</span>
      </div>
      <div class="col-pnlpc" :class="getPnlClass(totalPnLPercent)">
        <span>{{ formatPnLPercent(totalPnLPercent)+'%' }}</span>
      </div>
    </div>

    <!-- Список позиций -->
    <div class="positions-list">
      <div 
        v-for="position in positions" 
        :key="position.Symbol + position.Side"
        class="position-item"
        :class="getSideClass(position.Side)"
      >
        <div class="col-symbol">{{ getShortSymbol(position.Symbol) }}</div>
        <div class="col-size">{{ formatCompactNumber(position.CurrentValue) }}</div>
        <div class="col-pnl" :class="getPnlClass(position.UnrealisedPnl)">
          {{ formatPnl(position.UnrealisedPnl) }}
        </div>
        <div class="col-pnlpc" :class="getPnlClass(getPnLPercent(position))">
          {{ formatPnLPercent(getPnLPercent(position)) }}
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

const totalInvestment = computed(() => {
  return positions.value.reduce((sum, position) => sum + Math.abs(position.EntryPrice * position.Size), 0)
})

const totalPnLPercent = computed(() => {
  const inv = totalInvestment.value
  if (inv === 0) return 0
  return (totalUnrealisedPnl.value / inv) * 100
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

const getSideClass = (side: string) => {
  if (side === 'Buy') return 'position-long'
  if (side === 'Sell') return 'position-short'
  return ''
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

const getPnLPercent = (position: Position): number => {
  const investment = Math.abs(position.EntryPrice * position.Size)
  if (investment === 0) return 0
  return (position.UnrealisedPnl / investment) * 100
}

const formatPnLPercent = (percent: number): string => {
  return percent.toFixed(2)
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
  display: flex;
  flex-direction: column;

  width: 100%;
  height: 100%;
}

.positions-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.summary-row,
.position-item {
  display: grid;
  grid-template-columns: 40px 40px 40px 40px;
}

.col-symbol,
.col-size,
.col-pnl,
.col-pnlpc {
  font-weight: bold;
  padding: 2px 6px;
  min-width: 0;

  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;

  display: flex;
  align-items: center;

  justify-content: center;
}

.col-symbol {
  justify-content: flex-start;
  text-align: left;
}

.position-item {
  transition: background 0.15s;
}

.position-item:hover {
  background:rgba(195, 228, 186, 0.5);
}

.summary-row {
  border-bottom: 1px solid #e0e0e0;
}

.profit-count,
.col-pnl.profit,
.col-pnlpc.profit {
  color: #00a86b;
}

.loss-count,
.col-pnl.loss,
.col-pnlpc.loss {
  color: #ff4444;
}

/* граница слева */
.position-long > .col-symbol {
  border-left: 3px solid #00a86b;
}

.position-short > .col-symbol {
  border-left: 3px solid #ff4444;
}

/* пустое состояние */
.no-positions {
  grid-column: 1 / -1;
  text-align: center;
  padding: 10px;
  color: #777;
}
</style>