<template>
  <div :id="containerId" class="tv-widget-container"></div>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from "vue";

const props = withDefaults(defineProps<{
  symbol?: string
}>(), {
  symbol: "BYBIT:BTCUSDT"
})

const containerId = computed(() => {
  const clean = props.symbol.replace(/[^a-zA-Z0-9]/g, '').toLowerCase()
  return `tv-widget-${clean}`
})

const loadWidget = (symbol: string) => {
  const id = containerId.value
  const container = document.getElementById(id)
  if (!container) return

  // Очищаем контейнер от предыдущего скрипта
  container.innerHTML = ""

  const script = document.createElement("script");
  script.src = "https://s3.tradingview.com/external-embedding/embed-widget-advanced-chart.js";
  script.async = true;
  script.innerHTML = JSON.stringify({
    symbol: symbol.startsWith("BYBIT:") ? symbol : `BYBIT:${symbol}`,
    interval: "1",
    theme: "dark",
    style: "1",
    locale: "ru",
    timezone: "Europe/Moscow",
    allow_symbol_change: false,
    enable_publishing: false,
    container_id: id
  });

  container.appendChild(script);
}

onMounted(() => {
  loadWidget(props.symbol)
})

watch(() => props.symbol, (newSymbol) => {
  if (newSymbol) {
    // Даём Vue обновить DOM с новым containerId, затем грузим виджет
    setTimeout(() => loadWidget(newSymbol), 0)
  }
})
</script>

<style scoped>
.tv-widget-container {
  width: 100%;
  height: 100%;
}
</style>