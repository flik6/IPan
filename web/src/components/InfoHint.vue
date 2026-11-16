<script setup lang="ts">
// 说明浮层：以 ? 图标代替整段解释文字，鼠标悬停/键盘聚焦时展开。
import { computed, onBeforeUnmount, ref } from "vue";
import { NPopover } from "naive-ui";
import Icon from "./Icon.vue";

const props = withDefaults(defineProps<{ text: string; label?: string; size?: number }>(), {
  label: "",
  size: 14,
});

const ariaLabel = computed(() => props.label || props.text);
const open = ref(false);
let hideTimer: ReturnType<typeof setTimeout> | null = null;

function show(): void {
  if (hideTimer) {
    clearTimeout(hideTimer);
    hideTimer = null;
  }
  open.value = true;
}

/** 悬停移出时留一点缓冲，避免鼠标划过图标时浮层闪烁 */
function hide(): void {
  if (hideTimer) clearTimeout(hideTimer);
  hideTimer = setTimeout(() => {
    open.value = false;
    hideTimer = null;
  }, 90);
}

/** 点击切换：触屏设备没有悬停，靠点击展开 */
function toggle(): void {
  if (hideTimer) {
    clearTimeout(hideTimer);
    hideTimer = null;
  }
  open.value = !open.value;
}

onBeforeUnmount(() => {
  if (hideTimer) clearTimeout(hideTimer);
});
</script>

<template>
  <NPopover
    :show="open"
    trigger="manual"
    placement="top"
    :delay="0"
    :show-arrow="true"
    class="info-hint-pop"
    @clickoutside="open = false"
  >
    <template #trigger>
      <button
        type="button"
        class="info-hint"
        :aria-label="ariaLabel"
        :title="ariaLabel"
        :aria-expanded="open"
        @mouseenter="show"
        @mouseleave="hide"
        @focus="show"
        @blur="hide"
        @click.stop="toggle"
      >
        <Icon name="help" :size="size" />
      </button>
    </template>
    <span class="info-hint-text">{{ text }}</span>
  </NPopover>
</template>

<style scoped>
.info-hint {
  display: inline-grid;
  place-items: center;
  flex: none;
  width: 20px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: var(--gh-ink-faint);
  background: transparent;
  cursor: help;
  transition: color 160ms ease, background 160ms ease;
}
.info-hint:hover,
.info-hint:focus-visible { color: var(--gh-primary); background: var(--gh-primary-soft); }
</style>

<style>
.info-hint-pop .info-hint-text {
  display: block;
  max-width: 240px;
  font-size: 12px;
  line-height: 1.6;
}
</style>
