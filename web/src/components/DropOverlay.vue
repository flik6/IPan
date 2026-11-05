<script setup lang="ts">
import { ui } from "../stores/ui";
</script>

<template>
  <div class="drop-overlay" :class="{ active: ui.dropActive }" aria-hidden="true">
    <div class="drop-overlay-card">
      松开鼠标上传文件
    </div>
  </div>
</template>

<style scoped>
.drop-overlay {
  position: fixed;
  inset: 0;
  z-index: 300;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(38, 43, 74, 0.35);
  backdrop-filter: blur(3px);
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition:
    opacity 0.18s ease-out,
    visibility 0.18s ease-out;
}
.drop-overlay.active {
  opacity: 1;
  visibility: visible;
}
.drop-overlay-card {
  width: min(520px, 100%);
  padding: 36px 24px;
  border: 2px dashed var(--gh-primary);
  border-radius: 24px;
  background: var(--gh-surface-pop);
  box-shadow: var(--gh-shadow-lg);
  color: var(--gh-ink);
  text-align: center;
  font-size: 18px;
  font-weight: 700;
}
</style>
