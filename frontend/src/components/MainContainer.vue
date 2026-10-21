<template>
  <div class="main-window">
    <div class="content-section">
      <PositionComponent
        :view-mode="viewMode"
        :selected-symbol="selectedSymbol"
        @select-position="onSelectPosition"
        @toggle-chart="toggleChart"
      />
    </div>

    <div class="control-panel-section">
      <ControlPanel
        :view-mode="viewMode"
        @go-back="goBackToList"
      />
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import { WindowSetSize } from '../../wailsjs/runtime/runtime'
import PositionComponent from './Position.vue'
import ControlPanel from './ControlPanel.vue'

// Ширина одинаковая у detail и detail-chart — при раскрытии графика меняется только
// высота окна, без горизонтального "скачка".
const WIDGET_WIDTH = 500
// +20px под шапку с позициями (summary-row) и ещё +20px под строку выбранной позиции
// (detail-row) — обе остаются видны поверх графика в режиме detail-chart
const WIDGET_HEIGHT = 440
const DETAIL_WIDTH = WIDGET_WIDTH
// summary-row + строка позиции + кнопка/прогресс-бар лимитного закрытия
const DETAIL_HEIGHT = 140
const POSITION_WIDTH = 180
const POSITION_HEIGHT = 46

// list          — полный список позиций
// detail        — одна выбранная позиция (та же строка, что и в списке), без графика
// detail-chart  — та же позиция + раскрытый график, окно расширяется дальше по высоте
type ViewMode = 'list' | 'detail' | 'detail-chart'

const viewMode = ref<ViewMode>('list')
const selectedSymbol = ref('')

// Клик по позиции в списке — сразу открываем detail в увеличенном окне
const onSelectPosition = (symbol: string) => {
  selectedSymbol.value = symbol
  viewMode.value = 'detail'
  WindowSetSize(DETAIL_WIDTH, DETAIL_HEIGHT)
}

const goBackToList = () => {
  viewMode.value = 'list'
  selectedSymbol.value = ''
  WindowSetSize(POSITION_WIDTH, POSITION_HEIGHT)
}

const toggleChart = () => {
  if (viewMode.value === 'detail-chart') {
    viewMode.value = 'detail'
    WindowSetSize(DETAIL_WIDTH, DETAIL_HEIGHT)
  } else if (viewMode.value === 'detail') {
    viewMode.value = 'detail-chart'
    WindowSetSize(WIDGET_WIDTH, WIDGET_HEIGHT)
  }
}
</script>

<style scoped>

.main-window {
  width: 100%;
  height: 100%;
  display: flex;
  overflow: hidden;
  font-size: 10px;
  background: var(--color-bg);
}

.content-section {
  flex: 1;
  min-width: 0;
}

.control-panel-section {
  width: 20px;
  border-left: 1px solid var(--color-border);
  padding-top: 4px;
  display: flex;
  flex-direction: column;
  align-items: center;
  overflow-y: auto;
}

</style>
