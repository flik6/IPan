<script setup lang="ts">
import Icon from "./Icon.vue";

defineProps<{ icon: string; title: string; description: string }>();
</script>

<template>
  <div class="workspace-empty">
    <span class="empty-mark" aria-hidden="true"><Icon :name="icon" :size="26" /></span>
    <strong>{{ title }}</strong>
    <p>{{ description }}</p>
    <slot />
  </div>
</template>

<style scoped>
.workspace-empty {
  display: grid;
  justify-items: center;
  gap: 9px;
  padding: clamp(46px, 8vw, 92px) 20px;
  text-align: center;
  white-space: normal;
}
.empty-mark {
  display: grid;
  place-items: center;
  width: 64px;
  height: 64px;
  margin-bottom: 5px;
  border: 1px solid var(--op-rule-strong);
  color: var(--op-accent);
  background: var(--op-accent-soft);
}
strong {
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 26px;
  font-weight: 500;
  letter-spacing: -0.03em;
}
p {
  max-width: 42ch;
  margin: 0;
  color: var(--op-ink-soft);
  font-size: 13px;
  line-height: 1.7;
}
</style>

<style scoped>
.workspace-empty { display: grid; justify-items: center; gap: 8px; padding: clamp(42px, 7vw, 72px) 20px; text-align: center; }
.empty-mark {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 4px;
  border: 0;
  border-radius: 12px;
  color: var(--db-blue);
  background: var(--db-blue-soft);
}
.workspace-empty strong { color: var(--op-ink); font-family: var(--op-font-sans); font-size: 16px; font-weight: 600; letter-spacing: -0.01em; }
.workspace-empty p { max-width: 42ch; margin: 0; color: var(--op-ink-soft); font-size: 13px; line-height: 1.6; }
</style>
