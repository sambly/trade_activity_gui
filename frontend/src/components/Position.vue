<template>
  <main>
    <div class="position-container">
      <h2>Текущие позиции</h2>
      
      <div v-if="positions.length === 0" class="no-positions">
        Нет открытых позиций
      </div>

      <div v-else class="positions-grid">
        <div class="position-header">
          <div class="header-cell">Символ</div>
          <div class="header-cell">Сторона</div>
          <div class="header-cell">Размер</div>
          <div class="header-cell">Цена входа</div>
          <div class="header-cell">Нереализованный PnL</div>
          <div class="header-cell">Реализованный PnL</div>
        </div>

        <div 
          v-for="position in positions" 
          :key="position.Symbol + position.Side"
          class="position-row"
          :class="getRowClass(position)"
        >
          <div class="cell symbol">{{ position.Symbol }}</div>
          <div class="cell side" :class="getSideClass(position.Side)">
            {{ getSideText(position.Side) }}
          </div>
          <div class="cell size">{{ formatNumber(position.Size) }}</div>
          <div class="cell entry-price">{{ formatNumber(position.EntryPrice) }}</div>
          <div class="cell pnl" :class="getPnlClass(position.UnrealisedPnl)">
            {{ formatPnl(position.UnrealisedPnl) }}
          </div>
          <div class="cell pnl" :class="getPnlClass(position.CumRealisedPnl)">
            {{ formatPnl(position.CumRealisedPnl) }}
          </div>
        </div>
      </div>

      <!-- Сводная информация -->
      <div v-if="positions.length > 0" class="summary">
        <div class="summary-item">
          <span>Всего позиций:</span>
          <span>{{ positions.length }}</span>
        </div>
        <div class="summary-item">
          <span>Общий нереализованный PnL:</span>
          <span :class="getPnlClass(totalUnrealisedPnl)">{{ formatPnl(totalUnrealisedPnl) }}</span>
        </div>
        <div class="summary-item">
          <span>Общий реализованный PnL:</span>
          <span :class="getPnlClass(totalRealisedPnl)">{{ formatPnl(totalRealisedPnl) }}</span>
        </div>
      </div>

      <button class="btn" @click="loadPositions">Обновить</button>
    </div>
  </main>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { GetPositions } from '../../wailsjs/go/main/App'
import type { exchange } from '../../wailsjs/go/models'

type Position = exchange.Position

const positions = ref<Position[]>([])
let intervalId: number | null = null

const totalUnrealisedPnl = computed(() => {
  return positions.value.reduce((sum, position) => sum + position.UnrealisedPnl, 0)
})

const totalRealisedPnl = computed(() => {
  return positions.value.reduce((sum, position) => sum + position.CumRealisedPnl, 0)
})

const loadPositions = async () => {
  try {
    const result = await GetPositions()
    positions.value = result || []
  } catch (error) {
    console.error('Ошибка при загрузке позиций:', error)
    positions.value = []
  }
}

const getSideClass = (side: string) => {
  return side.toLowerCase() === 'buy' ? 'side-buy' : 'side-sell'
}

const getSideText = (side: string) => {
  return side.toLowerCase() === 'buy' ? 'LONG' : 'SHORT'
}

const getPnlClass = (pnl: number) => {
  if (pnl > 0) return 'pnl-positive'
  if (pnl < 0) return 'pnl-negative'
  return 'pnl-neutral'
}

const getRowClass = (position: Position) => {
  return getPnlClass(position.UnrealisedPnl)
}

const formatNumber = (value: number) => {
  if (value === undefined || value === null) return '0'
  return new Intl.NumberFormat('ru-RU', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 8
  }).format(value)
}

const formatPnl = (pnl: number) => {
  return `${pnl >= 0 ? '+' : ''}${formatNumber(pnl)}`
}

onMounted(() => {
  loadPositions()
  // Опционально: обновлять позиции периодически
  intervalId = window.setInterval(() => {
    loadPositions()
  }, 5000) // Обновление каждые 5 секунд
})

onUnmounted(() => {
  if (intervalId) {
    clearInterval(intervalId)
  }
})
</script>

<style scoped>
.position-container {
  padding: 20px;
  font-family: Arial, sans-serif;
}

h2 {
  color: #333;
  margin-bottom: 20px;
}

.no-positions {
  text-align: center;
  color: #666;
  font-style: italic;
  padding: 40px;
}

.positions-grid {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 20px;
}

.position-header {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr 1fr 1fr 1fr;
  background-color: #f5f5f5;
  font-weight: bold;
  border-bottom: 1px solid #e0e0e0;
}

.header-cell, .cell {
  padding: 12px 8px;
  text-align: center;
  border-right: 1px solid #e0e0e0;
}

.header-cell:last-child, .cell:last-child {
  border-right: none;
}

.position-row {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr 1fr 1fr 1fr;
  border-bottom: 1px solid #e0e0e0;
  transition: background-color 0.2s;
}

.position-row:hover {
  background-color: #f9f9f9;
}

.position-row:last-child {
  border-bottom: none;
}

/* Стили для сторон позиции */
.side-buy {
  color: #00a86b;
  font-weight: bold;
}

.side-sell {
  color: #ff4444;
  font-weight: bold;
}

/* Стили для PnL */
.pnl-positive {
  color: #00a86b;
  font-weight: bold;
}

.pnl-negative {
  color: #ff4444;
  font-weight: bold;
}

.pnl-neutral {
  color: #666;
}

/* Сводная информация */
.summary {
  margin-bottom: 20px;
  padding: 15px;
  background-color: #f8f9fa;
  border-radius: 8px;
  border-left: 4px solid #007bff;
}

.summary-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.summary-item:last-child {
  margin-bottom: 0;
  font-weight: bold;
  font-size: 1.1em;
}

.btn {
  background-color: #007bff;
  color: white;
  border: none;
  padding: 10px 20px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 16px;
}

.btn:hover {
  background-color: #0056b3;
}

/* Адаптивность */
@media (max-width: 768px) {
  .position-header,
  .position-row {
    grid-template-columns: 1fr;
    gap: 5px;
  }
  
  .header-cell, .cell {
    text-align: left;
    padding: 8px;
    border-right: none;
    border-bottom: 1px solid #e0e0e0;
  }
  
  .header-cell:last-child, .cell:last-child {
    border-bottom: none;
  }
}
</style>