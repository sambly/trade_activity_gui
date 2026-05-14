<template>
  <div class="main-window">
    <div class="content-section">
      <PositionComponent v-if="!showWidget" @select-position="onSelectPosition" />
      <div v-else class="widget-area">
        <WidgetTradingView :symbol="selectedSymbol" />
      </div>
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
import WidgetTradingView from './WidgetTradingView.vue'
import ControlPanel from './ControlPanel.vue'

const WIDGET_WIDTH = 500
const WIDGET_HEIGHT = 400
const POSITION_WIDTH = 180
const POSITION_HEIGHT = 52

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
  background: #f8f9fa;
}

.content-section {
  flex: 1;
  min-width: 0;
}

.widget-area {
  width: 100%;
  height: 100%;
  border: 10px solid #444;
  box-sizing: border-box;
}

.control-panel-section {
  width: 20px;
  border-left: 1px solid #ddd;
  padding-top: 4px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

</style>
