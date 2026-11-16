<script setup lang="ts">
import { NCheckbox, NDropdown, type DropdownOption } from "naive-ui";
import Icon from "./Icon.vue";

defineProps<{
  title: string;
  description: string;
  icon: string;
  checked: boolean;
  options: DropdownOption[];
}>();
const emit = defineEmits<{
  "update:checked": [checked: boolean];
  copy: [];
  action: [key: string];
}>();

function onActionSelect(key: string | number): void {
  // NDropdown 的 key 类型较宽，分享操作只用字符串键
  emit("action", String(key));
}
</script>

<template>
  <li class="share-record" :class="{ selected: checked }">
    <div class="identity">
      <NCheckbox :checked="checked" :aria-label="'选择 ' + title" @update:checked="$emit('update:checked', $event)" />
      <span class="record-icon"><Icon :name="icon" :size="20" /></span>
      <div class="record-title">
        <strong :title="title">{{ title }}</strong>
        <span>{{ description }}</span>
      </div>
    </div>

    <dl class="record-details"><slot /></dl>

    <div class="record-actions">
      <button type="button" class="copy-btn" :aria-label="'复制链接，' + title" title="复制链接" @click="$emit('copy')">
        <Icon name="copy" :size="17" /><span>复制链接</span>
      </button>
      <NDropdown trigger="click" placement="bottom-end" :options="options" @select="onActionSelect">
        <button type="button" class="icon-control" :aria-label="'更多操作，' + title" aria-haspopup="menu" title="更多操作">
          <Icon name="dots" :size="19" />
        </button>
      </NDropdown>
    </div>
  </li>
</template>

<style scoped>
.share-record {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1.45fr) auto;
  align-items: center;
  gap: 16px;
  min-width: 0;
  padding: 13px 16px;
  border: 1px solid transparent;
  border-radius: 16px;
}
.share-record + .share-record { border-top-color: var(--gh-row-border); }
.share-record:hover { background: var(--gh-surface-2); }
.share-record.selected { border-color: var(--gh-border-hover); background: var(--gh-selected-bg); }
.identity, .record-actions { display: flex; align-items: center; gap: 8px; min-width: 0; }
.identity { gap: 12px; }
.record-icon {
  display: grid;
  place-items: center;
  flex: 0 0 40px;
  height: 40px;
  border-radius: 13px;
  color: var(--gh-primary);
  background: var(--gh-primary-soft);
}
.record-title { display: grid; min-width: 0; gap: 2px; }
.record-title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.record-title > span { color: var(--gh-ink-soft); font-size: 12px; overflow-wrap: anywhere; }
.record-details {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-items: center;
  gap: 8px;
  margin: 0;
  min-width: 0;
  font-variant-numeric: tabular-nums;
}
.record-details :slotted(div) { min-width: 0; }
.record-details :deep(dt) { color: var(--gh-ink-faint); font-size: 11px; font-weight: 650; }
.record-details :deep(dd) { margin: 3px 0 0; color: var(--gh-ink); font-size: 12px; overflow-wrap: anywhere; }
.copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 44px;
  min-height: 44px;
  padding: 0 12px;
  border: 1px solid var(--gh-border);
  border-radius: var(--gh-radius-control);
  color: var(--gh-primary);
  background: var(--gh-surface);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
  cursor: pointer;
}
.copy-btn:hover { background: var(--gh-primary-soft); }

@media (max-width: 950px) {
  .share-record { grid-template-columns: minmax(0, 1fr) auto; row-gap: 10px; }
  .record-details { grid-row: 2; grid-column: 1 / -1; padding: 10px 0 0 28px; border-top: 1px solid var(--gh-row-border); }
  .record-title strong { overflow-wrap: anywhere; white-space: normal; }
}
@media (max-width: 560px) {
  .share-record { padding: 10px 8px; column-gap: 6px; }
  .identity { gap: 7px; }
  .record-icon { flex-basis: 32px; height: 32px; border-radius: 10px; }
  .record-icon svg { width: 16px; height: 16px; }
  .record-actions { gap: 2px; }
  .copy-btn { padding: 0; }
  .copy-btn span { display: none; }
  .record-details { padding-left: 0; }
}
</style>

<style scoped>
.share-record {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 0.9fr) auto;
  align-items: center;
  gap: 20px;
  min-width: 0;
  padding: 17px 20px;
  border: 0;
  border-bottom: 1px solid var(--op-rule);
  border-radius: 0;
  background: transparent;
  transition: background-color 160ms ease, box-shadow 160ms ease;
}

.share-record:last-child { border-bottom: 0; }
.share-record:hover { background: var(--op-surface-raised); }
.share-record.selected { background: var(--op-accent-soft); box-shadow: inset 2px 0 0 var(--op-accent); }

.identity,
.record-actions { display: flex; align-items: center; gap: 9px; min-width: 0; }
.identity { gap: 11px; }

.record-icon {
  display: grid;
  place-items: center;
  flex: 0 0 38px;
  height: 38px;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-accent);
  background: var(--op-surface-raised);
}

.record-title { display: grid; min-width: 0; gap: 3px; }
.record-title strong {
  overflow: hidden;
  color: var(--op-ink);
  font-size: 13px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.record-title > span {
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.record-details {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-items: center;
  gap: 10px;
  margin: 0;
  min-width: 0;
  font-variant-numeric: tabular-nums;
}

.record-details > :slotted(div) { min-width: 0; padding-left: 12px; border-left: 1px solid var(--op-rule); }
.record-details :deep(dt) {
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.record-details :deep(dd) {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  color: var(--op-ink);
  font-size: 12px;
}

.copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 44px;
  min-height: 38px;
  padding: 0 11px;
  border: 1px solid var(--op-rule-strong);
  border-radius: 0;
  color: var(--op-ink);
  background: transparent;
  font-size: 11px;
  font-weight: 650;
  white-space: nowrap;
  cursor: pointer;
}

.copy-btn:hover { color: var(--op-accent); border-color: var(--op-accent); background: transparent; }

@media (max-width: 950px) {
  .share-record { grid-template-columns: minmax(0, 1fr) auto; row-gap: 12px; }
  .record-details { grid-row: 2; grid-column: 1 / -1; padding: 0; border-top: 1px solid var(--op-rule); padding-top: 10px; }
  .record-title strong { overflow-wrap: anywhere; white-space: normal; }
}

@media (max-width: 560px) {
  .share-record { padding: 13px 12px; }
  .record-icon { flex-basis: 32px; height: 32px; }
  .record-details { gap: 6px; }
  .record-details > :slotted(div) { padding-left: 7px; }
  .copy-btn { width: 38px; min-width: 38px; padding: 0; }
  .copy-btn span { display: none; }
}
</style>

<style scoped>
.share-record {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 0.85fr) auto;
  align-items: center;
  gap: 18px;
  min-width: 0;
  padding: 13px 16px;
  border: 0;
  border-bottom: 1px solid #f0f1f4;
  border-radius: 0;
  background: #fff;
  box-shadow: none;
}

.share-record:last-child { border-bottom: 0; }
.share-record:hover { background: #f9fafb; }
.share-record.selected { background: var(--db-blue-soft); box-shadow: none; }

.identity,
.record-actions { display: flex; align-items: center; gap: 8px; min-width: 0; }
.identity { gap: 10px; }

.record-icon {
  display: grid;
  place-items: center;
  flex: 0 0 36px;
  height: 36px;
  border: 0;
  border-radius: 8px;
  color: var(--db-blue);
  background: var(--db-blue-soft);
}

.record-title { display: grid; min-width: 0; gap: 2px; }
.record-title strong { overflow: hidden; color: var(--op-ink); font-size: 13px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.record-title > span { color: var(--op-ink-faint); font-size: 12px; letter-spacing: 0; text-transform: none; }

.record-details {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-items: center;
  gap: 10px;
  margin: 0;
  min-width: 0;
  font-variant-numeric: tabular-nums;
}

.record-details > :slotted(div) { min-width: 0; padding-left: 0; border-left: 0; }
.record-details :deep(dt) { color: var(--op-ink-faint); font-family: var(--op-font-sans); font-size: 11px; font-weight: 400; letter-spacing: 0; text-transform: none; }
.record-details :deep(dd) { margin: 2px 0 0; color: var(--op-ink); font-size: 12px; overflow-wrap: anywhere; }

.copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  min-width: 38px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid var(--op-rule-strong);
  border-radius: 8px;
  color: var(--db-blue);
  background: #fff;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
}

.copy-btn:hover { border-color: var(--db-blue); background: var(--db-blue-soft); }

@media (max-width: 950px) {
  .share-record { grid-template-columns: minmax(0, 1fr) auto; row-gap: 10px; }
  .record-details { grid-row: 2; grid-column: 1 / -1; padding-top: 9px; border-top: 1px solid var(--op-rule); }
  .record-title strong { overflow-wrap: anywhere; white-space: normal; }
}

@media (max-width: 560px) {
  .share-record { padding: 12px; }
  .record-icon { flex-basis: 32px; height: 32px; }
  .record-details { gap: 8px; }
  .copy-btn { width: 34px; min-width: 34px; padding: 0; }
  .copy-btn span { display: none; }
}
</style>
