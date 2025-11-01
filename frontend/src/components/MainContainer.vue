<template>
  <div class="main-window">
    <div class="content-section">
      <PositionComponent />
    </div>
    
    <div class="control-panel-section">
      <ControlPanel />
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import PositionComponent from './Position.vue'
import ControlPanel from './ControlPanel.vue'

const controlPanelRef = ref<InstanceType<typeof ControlPanel>>()

// При необходимости можно управлять статусом извне:
// controlPanelRef.value?.updateStatus('connected')
</script>

<style scoped>
.main-window {
  width: 160px;
  height: 50px;
  border: 1px solid #ddd;
  border-radius: 6px;
  display: flex;
  overflow: hidden;
  font-size: 10px;
  background: #f8f9fa;
}

.content-section {
  flex: 1;
  min-width: 0;
}

.control-panel-section {
  width: 20px;
  border-left: 1px solid #ddd;
  padding-top: 4px;
}

</style>