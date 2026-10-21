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
    <div class="positions-list" v-if="viewMode === 'list'">
      <div
        v-for="position in positions"
        :key="position.Symbol + position.Side"
        class="position-item"
        :class="getSideClass(position.Side)"
        :style="getMomentumStyle(position)"
        @click="$emit('select-position', position.Symbol)"
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

    <!-- Строка выбранной позиции — те же параметры, что и в списке, в режимах detail и detail-chart -->
    <div v-else-if="detailPosition" class="detail-row-wrap">
      <div
        class="position-item"
        :class="getSideClass(detailPosition.Side)"
        :style="getMomentumStyle(detailPosition)"
      >
        <div class="col-symbol">{{ getShortSymbol(detailPosition.Symbol) }}</div>
        <div class="col-size">{{ formatCompactNumber(detailPosition.CurrentValue) }}</div>
        <div class="col-pnl" :class="getPnlClass(detailPosition.UnrealisedPnl)">
          {{ formatPnl(detailPosition.UnrealisedPnl) }}
        </div>
        <div class="col-pnlpc" :class="getPnlClass(getPnLPercent(detailPosition))">
          {{ formatPnLPercent(getPnLPercent(detailPosition)) }}
        </div>
      </div>

      <!-- Раскрывает/сворачивает график этой позиции -->
      <button
        class="chart-toggle-btn"
        :title="viewMode === 'detail-chart' ? 'Свернуть график' : 'Показать график'"
        @click="$emit('toggle-chart')"
      >
        📊
      </button>

      <!-- Автоматическое закрытие позиции лимитками по лучшей цене -->
      <div class="limit-close-panel">
        <button
          class="limit-close-btn"
          :class="{ 'limit-close-btn--active': detailPosition.LimitCloseActive }"
          :disabled="limitCloseBusy"
          @click="onToggleLimitClose(detailPosition)"
        >
          {{ detailPosition.LimitCloseActive
            ? 'Стоп'
            : (detailPosition.Side === 'Sell' ? 'Купить лимитками' : 'Продать лимитками') }}
        </button>

        <div v-if="detailPosition.LimitCloseActive" class="limit-close-progress">
          <div class="limit-close-progress-bar" :style="{ width: detailPosition.LimitCloseProgress + '%' }"></div>
          <span class="limit-close-progress-label">{{ detailPosition.LimitCloseProgress.toFixed(0) }}%</span>
        </div>

        <!-- limitCloseError — ответ на клик по кнопке; LimitCloseError — джоб мог остановиться
             сам уже после запуска (см. registerLimitCloseFailure в position_service.go) -->
        <div v-if="limitCloseError" class="limit-close-error">{{ limitCloseError }}</div>
        <div v-else-if="detailPosition.LimitCloseError" class="limit-close-error">{{ detailPosition.LimitCloseError }}</div>
      </div>
    </div>

    <!-- Позиция закрылась целиком (в т.ч. лимитками) — самой позиции уже нет, но
         сводку по завершённому закрытию оставляем на виду вместо пустого экрана -->
    <div v-else-if="limitCloseSummary" class="limit-close-summary">
      <div class="limit-close-summary-title">Позиция закрыта лимитками</div>

      <div class="limit-close-progress">
        <div class="limit-close-progress-bar" :style="{ width: limitCloseSummary.Progress + '%' }"></div>
        <span class="limit-close-progress-label">{{ limitCloseSummary.Progress.toFixed(0) }}%</span>
      </div>

      <div class="limit-close-summary-stats">
        <div>Время торговли: {{ formatDuration(limitCloseSummary.DurationSeconds) }}</div>
        <div>Сделок: {{ limitCloseSummary.FillCount }}</div>
        <div>Объём: {{ formatCompactNumber(limitCloseSummary.FilledQty) }} / {{ formatCompactNumber(limitCloseSummary.StartSize) }}</div>
        <div v-if="limitCloseSummary.FillCount > 0">
          Цена мин/сред/макс: {{ formatNumber(limitCloseSummary.MinPrice) }} / {{ formatNumber(limitCloseSummary.AvgPrice) }} / {{ formatNumber(limitCloseSummary.MaxPrice) }}
        </div>
      </div>
    </div>

    <div v-else class="no-positions">
      Позиция закрыта
    </div>

    <!-- График — раскрывается вместе с окном поверх строки выбранной позиции -->
    <div class="widget-area" v-if="viewMode === 'detail-chart'">
      <WidgetTradingView :symbol="selectedSymbol" />
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { GetPositions, StartLimitClose, StopLimitClose, GetLimitCloseSummary } from '../../wailsjs/go/main/App'
import type { exchange } from '../../wailsjs/go/models'
import WidgetTradingView from './WidgetTradingView.vue'

type Position = exchange.Position
type LimitCloseSummary = exchange.LimitCloseSummary

const props = withDefaults(defineProps<{
  viewMode?: 'list' | 'detail' | 'detail-chart'
  selectedSymbol?: string
}>(), {
  viewMode: 'list',
  selectedSymbol: ''
})

defineEmits<{
  (e: 'select-position', symbol: string): void
  (e: 'toggle-chart'): void
}>()

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

// Позиция, открытая в detail/detail-chart. Если позицию закрыли на бирже, пока окно
// было открыто, она пропадёт из positions (см. position_service.go) и здесь станет
// undefined — строка просто перестанет отображаться.
const detailPosition = computed(() =>
  positions.value.find(p => p.Symbol === props.selectedSymbol)
)

// Состояние LimitCloseActive/LimitCloseProgress приходит вместе с обычным поллингом
// GetPositions (раз в секунду) — отдельного опроса не нужно.
const limitCloseBusy = ref(false)
const limitCloseError = ref('')

// Сводка по последнему полностью завершённому лимитками закрытию выбранного символа —
// показывается вместо строки позиции, когда самой позиции уже нет (см. шаблон).
const limitCloseSummary = ref<LimitCloseSummary | null>(null)

const loadLimitCloseSummary = async () => {
  if (props.viewMode === 'list' || !props.selectedSymbol) {
    limitCloseSummary.value = null
    return
  }
  try {
    limitCloseSummary.value = (await GetLimitCloseSummary(props.selectedSymbol)) || null
  } catch (error) {
    console.error('Ошибка при загрузке сводки лимитного закрытия:', error)
  }
}

// При смене выбранной позиции сразу сбрасываем сводку от предыдущего символа, не дожидаясь
// очередного тика поллинга — иначе на миг мелькнёт чужая сводка.
watch(() => props.selectedSymbol, () => {
  limitCloseSummary.value = null
  loadLimitCloseSummary()
})

const onToggleLimitClose = async (position: Position) => {
  limitCloseError.value = ''
  limitCloseBusy.value = true
  try {
    if (position.LimitCloseActive) {
      await StopLimitClose(position.Symbol)
    } else {
      await StartLimitClose(position.Symbol)
    }
  } catch (error) {
    console.error('Ошибка лимитного закрытия позиции:', error)
    limitCloseError.value = String(error)
  } finally {
    limitCloseBusy.value = false
  }
}

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

// Подсветка фона по скорости изменения цены (position.PriceChangePercent, % в минуту).
// Порог насыщения и мёртвая зона согласованы с бэкендом (окно 60с в position_service.go).
const MOMENTUM_DEADZONE = 0.05 // %/мин — ниже этого фон не подсвечивается
const MOMENTUM_SATURATION = 0.6 // %/мин — при этой скорости фон уже максимально насыщен
const MOMENTUM_MAX_OPACITY = 0.5

const PROFIT_COLOR = '0, 168, 107'
const LOSS_COLOR = '255, 68, 68'

// Ориентируем сырое изменение цены на сторону позиции: для шорта падение цены — это рост позиции
const getMomentum = (position: Position): number => {
  const change = position.PriceChangePercent || 0
  return position.Side === 'Sell' ? -change : change
}

const getMomentumStyle = (position: Position) => {
  const momentum = getMomentum(position)
  const magnitude = Math.abs(momentum)

  if (magnitude < MOMENTUM_DEADZONE) {
    return { '--momentum-bg': 'transparent' }
  }

  const intensity = Math.min(
    (magnitude - MOMENTUM_DEADZONE) / (MOMENTUM_SATURATION - MOMENTUM_DEADZONE),
    1
  )
  const opacity = (intensity * MOMENTUM_MAX_OPACITY).toFixed(3)
  const color = momentum > 0 ? PROFIT_COLOR : LOSS_COLOR

  return { '--momentum-bg': `rgba(${color}, ${opacity})` }
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

const formatDuration = (seconds: number): string => {
  if (!seconds || seconds < 0) return '0с'
  const total = Math.round(seconds)
  const m = Math.floor(total / 60)
  const s = total % 60
  return m > 0 ? `${m}м ${s}с` : `${s}с`
}

onMounted(() => {
  loadPositions()
  loadLimitCloseSummary()
  intervalId = window.setInterval(() => {
    loadPositions()
    loadLimitCloseSummary()
  }, 1000)
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

.widget-area {
  flex: 1;
  min-height: 0;
  border: 10px solid var(--color-widget-border);
  box-sizing: border-box;
}

.detail-row-wrap {
  position: relative;
}

/* Кнопка графика — сетка строки (4x40px) занимает только часть ширины detail-окна,
   поэтому кнопка не встроена в сетку, а закреплена в свободном углу строки */
.chart-toggle-btn {
  position: absolute;
  top: 1px;
  right: 1px;
  width: 12px;
  height: 12px;
  border: none;
  border-radius: 2px;
  background: var(--color-bg-alt);
  cursor: pointer;
  padding: 0;
  font-size: 9px;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.15s ease;
}

.chart-toggle-btn:hover {
  transform: scale(1.2);
}

.limit-close-panel {
  padding: 6px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}

.limit-close-btn {
  padding: 3px 8px;
  font-size: 9px;
  font-weight: bold;
  border: 1px solid var(--color-border);
  border-radius: 3px;
  background: var(--color-bg-alt);
  color: var(--color-text);
  cursor: pointer;
}

.limit-close-btn:hover:not(:disabled) {
  background: var(--color-hover-bg);
}

.limit-close-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

.limit-close-btn--active {
  border-color: #ff4444;
  color: #ff4444;
}

.limit-close-progress {
  position: relative;
  width: 160px;
  height: 12px;
  border: 1px solid var(--color-border);
  border-radius: 3px;
  overflow: hidden;
  background: var(--color-bg-alt);
}

.limit-close-progress-bar {
  position: absolute;
  inset: 0 auto 0 0;
  width: 0;
  background: rgba(0, 168, 107, 0.5);
  transition: width 0.3s ease;
}

.limit-close-progress-label {
  position: relative;
  display: block;
  text-align: center;
  font-size: 8px;
  line-height: 12px;
}

.limit-close-error {
  max-width: 300px;
  font-size: 8px;
  color: #ff4444;
}

.limit-close-summary {
  padding: 8px 6px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.limit-close-summary-title {
  font-weight: bold;
  font-size: 10px;
}

.limit-close-summary-stats {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 9px;
  color: var(--color-text);
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
  --momentum-bg: transparent;
  background-color: var(--momentum-bg);
  transition: background-color 1.2s ease-out;
}

.position-item:hover {
  background-color: var(--color-hover-bg);
}

.summary-row {
  border-bottom: 1px solid var(--color-border);
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
  color: var(--color-empty-text);
}
</style>