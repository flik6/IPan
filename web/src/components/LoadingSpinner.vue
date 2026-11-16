<script setup lang="ts">
// 行内加载指示：与主题主色一致的圆环，用于目录切换、预览载入等局部等待场景。
// 全屏启动动画用 LoadingLogo（logo 光带），这里只负责小尺寸、可嵌在任意容器里的等待态。
withDefaults(defineProps<{ size?: number; label?: string }>(), { size: 24, label: "" });
</script>

<template>
  <div class="loading-spinner" role="status" :aria-label="label || '加载中'">
    <svg :width="size" :height="size" viewBox="0 0 24 24" aria-hidden="true">
      <circle class="ring-track" cx="12" cy="12" r="9" />
      <circle class="ring-arc" cx="12" cy="12" r="9" />
    </svg>
    <span v-if="label" class="spinner-label">{{ label }}</span>
  </div>
</template>

<style scoped>
.loading-spinner {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  color: var(--op-ink-soft);
}
svg {
  flex: 0 0 auto;
  display: block;
  animation: spin 0.9s linear infinite;
}
.ring-track,
.ring-arc {
  fill: none;
  stroke-width: 2.6;
}
.ring-track {
  stroke: var(--op-rule);
}
/* 只画一段弧，旋转时形成流动感 */
.ring-arc {
  stroke: var(--op-accent);
  stroke-linecap: round;
  stroke-dasharray: 17 40;
  stroke-dashoffset: 0;
}
.spinner-label {
  font-size: 12px;
  white-space: nowrap;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  svg {
    animation-duration: 2.4s;
  }
}
</style>
