<template>
  <div class="main-window">
    <div class="content-section">
      <PositionComponent
        :show-widget="showWidget"
        :selected-symbol="selectedSymbol"
        @select-position="onSelectPosition"
      />
    </div>

    <div class="control-panel-section">
      <ControlPanel @toggle-widget="toggleWidget" />
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import { WindowSetSize } from '../../wailsjs/runtime/runtime'
import PositionComponent from './Position.vue'
import ControlPanel from './ControlPanel.vue'

const WIDGET_WIDTH = 500
// +20px под шапку с позициями (summary-row), которая теперь остаётся видна поверх графика
const WIDGET_HEIGHT = 420
const POSITION_WIDTH = 180
const POSITION_HEIGHT = 46

const showWidget = ref(false)
const selectedSymbol = ref('')

const onSelectPosition = (symbol: string) => {
  selectedSymbol.value = symbol
  showWidget.value = true
  WindowSetSize(WIDGET_WIDTH, WIDGET_HEIGHT)
}

const toggleWidget = () => {
  showWidget.value = !showWidget.value

  if (showWidget.value) {
    WindowSetSize(WIDGET_WIDTH, WIDGET_HEIGHT)
  } else {
    WindowSetSize(POSITION_WIDTH, POSITION_HEIGHT)
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
