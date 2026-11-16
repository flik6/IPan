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
  background: color-mix(in srgb, var(--op-bg) 84%, transparent);
  backdrop-filter: blur(4px);
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
  padding: 58px 24px;
  border: 1px dashed var(--op-accent);
  border-radius: 0;
  background: var(--op-surface);
  box-shadow: var(--op-shadow-lg);
  color: var(--op-ink);
  text-align: center;
  font-family: var(--op-font-display);
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.04em;
}
</style>

<style scoped>
.drop-overlay { position: fixed; inset: 0; z-index: 300; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgba(17, 24, 39, 0.28); opacity: 0; visibility: hidden; pointer-events: none; transition: opacity 160ms ease, visibility 160ms ease; }
.drop-overlay.active { opacity: 1; visibility: visible; }
.drop-overlay-card {
  width: min(480px, 100%);
  padding: 36px 24px;
  border: 2px dashed var(--db-blue);
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--op-shadow-lg);
  color: var(--op-ink);
  text-align: center;
  font-size: 18px;
  font-weight: 600;
}
</style>
