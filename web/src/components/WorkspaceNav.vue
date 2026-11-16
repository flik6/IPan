<script setup lang="ts">
import { nextTick } from "vue";
import Icon from "./Icon.vue";
import { setView } from "../stores/files";
import { ui, type AppView } from "../stores/ui";

const links: { view: AppView; label: string; icon: string }[] = [
  { view: "files", label: "文件", icon: "folder" },
  { view: "shares", label: "分享", icon: "share" },
  { view: "settings", label: "设置", icon: "settings" },
];
const scrollPositions = new Map<AppView, number>();

async function selectView(view: AppView): Promise<void> {
  if (ui.view === view) return;
  scrollPositions.set(ui.view, window.scrollY);
  setView(view);
  await nextTick();
  if (ui.view === view) window.scrollTo(0, scrollPositions.get(view) ?? 0);
}
</script>

<template>
  <nav class="workspace-nav" aria-label="主导航">
    <button
      v-for="item in links"
      :key="item.view"
      type="button"
      class="nav-item"
      :aria-current="ui.view === item.view ? 'page' : undefined"
      @click="selectView(item.view)"
    >
      <Icon :name="item.icon" :size="19" />
      <span>{{ item.label }}</span>
    </button>
  </nav>
</template>

<style scoped>
.workspace-nav {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 11px;
  width: 100%;
  min-height: 40px;
  padding: 0 11px;
  border: 0;
  border-radius: 8px;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 14px;
  font-weight: 500;
  text-align: left;
  cursor: pointer;
  transition: color 140ms ease, background-color 140ms ease;
}

.nav-item svg { flex: none; }
.nav-item:hover { color: var(--db-blue); background: var(--db-blue-soft); }
.nav-item[aria-current="page"] { color: var(--db-blue); background: var(--db-blue-soft); font-weight: 600; }

@media (max-width: 1040px) {
  .workspace-nav {
    position: fixed;
    z-index: 40;
    right: 0;
    bottom: 0;
    left: 0;
    grid-template-columns: repeat(3, 1fr);
    gap: 0;
    min-height: var(--op-mobile-nav);
    border-top: 1px solid var(--op-rule);
    background: #ffffff;
  }

  .nav-item {
    flex-direction: column;
    justify-content: center;
    gap: 3px;
    min-height: var(--op-mobile-nav);
    padding: 6px 4px calc(6px + env(safe-area-inset-bottom, 0px));
    border-radius: 0;
    font-size: 11px;
  }

  .nav-item[aria-current="page"] { background: transparent; }
  .nav-item[aria-current="page"] svg { color: var(--db-blue); }
}
</style>
