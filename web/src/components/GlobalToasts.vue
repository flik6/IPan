<script setup lang="ts">
import { toast, hideToast } from "../stores/toast";
</script>

<template>
  <div class="message-toasts" aria-live="polite">
    <TransitionGroup name="toast">
      <div
        v-for="t in toast.toasts"
        :key="t.id"
        class="message-toast"
        :class="t.type"
        :role="t.type === 'error' ? 'alert' : 'status'"
        @click="hideToast(t.id)"
      >
        {{ t.text }}
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.message-toasts {
  position: fixed;
  top: 24px;
  left: 50%;
  /* naive-ui 的弹窗容器是 2000，toast 必须在其之上，否则在弹窗里操作失败时的提示会被遮住 */
  z-index: 3000;
  display: grid;
  gap: 10px;
  transform: translateX(-50%);
  width: min(390px, calc(100vw - 36px));
}
.message-toast {
  padding: 13px 16px 13px 18px;
  border: 1px solid var(--op-rule-strong);
  border-left: 3px solid var(--op-accent);
  border-radius: 0;
  background: var(--op-surface-raised);
  color: var(--op-ink);
  box-shadow: var(--op-shadow-md);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  cursor: pointer;
  overflow-wrap: anywhere;
}
.message-toast.success {
  border-left-color: var(--op-success);
}
.message-toast.error {
  border-left-color: var(--op-danger);
  background: var(--op-danger-soft);
}
.toast-enter-active,
.toast-leave-active {
  transition: all 0.18s ease-out;
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(-8px);
}
.toast-leave-to {
  opacity: 0;
}
@media (max-width: 760px) {
  .message-toasts {
    top: 70px;
    width: calc(100vw - 24px);
  }
}
</style>

<style scoped>
.message-toasts { position: fixed; top: 20px; left: 50%; z-index: 3000; display: grid; gap: 8px; width: min(380px, calc(100vw - 32px)); transform: translateX(-50%); }
.message-toast {
  padding: 11px 14px;
  border: 1px solid var(--op-rule);
  border-left: 3px solid var(--db-blue);
  border-radius: 8px;
  background: #fff;
  color: var(--op-ink);
  box-shadow: var(--op-shadow-md);
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  cursor: pointer;
  overflow-wrap: anywhere;
}
.message-toast.success { border-left-color: var(--op-success); }
.message-toast.error { border-color: #fecdca; border-left-color: var(--op-danger); background: var(--op-danger-soft); }
.message-toast.warning { border-left-color: var(--op-warning); }
</style>
