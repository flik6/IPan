<script setup lang="ts">
import { ref } from "vue";
import { NBreadcrumb, NBreadcrumbItem, NButton, NModal, NSpin } from "naive-ui";
import { api, type WopanFile } from "../api/client";
import { ui } from "../stores/ui";
import { showToast } from "../stores/toast";
import { load as loadFiles } from "../stores/files";
import { doMoveCopy } from "../stores/ops";

interface PickerCrumb {
  id: string;
  name: string;
}

const stack = ref<PickerCrumb[]>([{ id: "0", name: "根目录" }]);
const dirs = ref<WopanFile[]>([]);
const loading = ref(false);
const errorText = ref("");
const confirming = ref(false);

async function loadDir(): Promise<void> {
  const current = stack.value[stack.value.length - 1];
  loading.value = true;
  errorText.value = "";
  try {
    const data = await api<{ files?: WopanFile[] }>("/api/list?parentId=" + current.id);
    dirs.value = (data.files || []).filter((f) => Number(f.type) === 0);
  } catch (e) {
    dirs.value = [];
    errorText.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

function enter(row: WopanFile): void {
  const id = row.id || row.fid || "";
  stack.value.push({ id, name: row.name || id });
  void loadDir();
}

function jumpTo(index: number): void {
  if (index < stack.value.length - 1) {
    stack.value = stack.value.slice(0, index + 1);
    void loadDir();
  }
}

async function confirmHere(): Promise<void> {
  const picker = ui.picker;
  if (!picker || confirming.value) return;
  confirming.value = true;
  const target = stack.value[stack.value.length - 1];
  ui.showFolderPicker = false;
  try {
    await doMoveCopy(picker.action, picker.items, target.id, target.name, () => loadFiles());
  } finally {
    confirming.value = false;
  }
}

function onShowChange(show: boolean): void {
  ui.showFolderPicker = show;
  if (show && ui.picker) {
    stack.value = [{ id: "0", name: "根目录" }];
    dirs.value = [];
    void loadDir();
  }
}
</script>

<template>
  <NModal :show="ui.showFolderPicker" @update:show="onShowChange">
    <div class="picker-box glass-panel">
      <div class="box-head">
        <span>{{ ui.picker?.action === "copy" ? "复制到" : "移动到" }} - 选择目标文件夹</span>
        <NButton size="small" @click="ui.showFolderPicker = false">关闭</NButton>
      </div>
      <div class="picker-path">
        <NBreadcrumb>
          <NBreadcrumbItem
            v-for="(crumb, i) in stack"
            :key="crumb.id"
            :clickable="i < stack.length - 1"
            @click="jumpTo(i)"
          >
            {{ crumb.name }}
          </NBreadcrumbItem>
        </NBreadcrumb>
      </div>
      <div class="picker-actions">
        <NButton type="primary" :loading="confirming" @click="confirmHere">选择当前目录</NButton>
      </div>
      <div class="picker-list">
        <NSpin v-if="loading" size="small" style="display: block; margin: 24px auto" />
        <div v-else-if="errorText" class="picker-empty error">{{ errorText }}</div>
        <template v-else>
          <div v-for="row in dirs" :key="row.id || row.fid" class="picker-row" @click="enter(row)">
            <svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="#F08C00" stroke-width="1.6">
              <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" fill="#FFE3BF" />
            </svg>
            <span class="name">{{ row.name || row.fid }}</span>
            <span class="muted">进入 ›</span>
          </div>
          <div v-if="!dirs.length" class="picker-empty">此目录没有子文件夹</div>
        </template>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.picker-box {
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
  width: min(560px, calc(100vw - 48px));
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--gh-border);
  font-weight: 700;
}
.picker-path {
  padding: 10px 18px;
  font-size: 12px;
  color: var(--gh-ink-soft);
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  align-items: center;
}
.picker-actions {
  padding: 10px 18px;
  border-bottom: 1px solid var(--gh-border);
  display: flex;
  gap: 8px;
}
.picker-list {
  padding: 10px 18px;
  overflow: auto;
  flex: 1;
  min-height: 120px;
}
.picker-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 14px;
  cursor: pointer;
  transition: background 0.2s ease-out;
}
.picker-row:hover {
  background: var(--gh-primary-soft);
}
.picker-row .name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.picker-empty {
  color: var(--gh-ink-soft);
  font-size: 13px;
  padding: 12px 0;
}
.picker-empty.error {
  color: var(--gh-danger);
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 12px;
}
</style>

<style scoped>
.picker-box { width: min(540px, calc(100vw - 28px)); max-height: min(82vh, 680px); overflow: hidden; border: 1px solid var(--op-rule); border-radius: 12px; background: #fff; box-shadow: var(--op-shadow-lg); }
.box-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 18px; border-bottom: 1px solid var(--op-rule); color: var(--op-ink); font-size: 18px; font-weight: 600; }
.picker-path { padding: 10px 18px; border-bottom: 1px solid var(--op-rule); background: #f9fafb; }
.picker-path :deep(.n-breadcrumb) { font-family: var(--op-font-sans); font-size: 12px; }
.picker-actions { display: flex; justify-content: flex-end; padding: 10px 18px; border-bottom: 1px solid var(--op-rule); }
.picker-list { max-height: min(46vh, 360px); overflow: auto; padding: 6px 10px 10px; }
.picker-row { display: flex; align-items: center; gap: 10px; min-height: 50px; padding: 0 8px; border-radius: 8px; cursor: pointer; }
.picker-row:hover { background: var(--db-blue-soft); box-shadow: none; }
.picker-row svg { width: 28px; height: 28px; }
.picker-row .name { flex: 1; min-width: 0; overflow: hidden; color: var(--op-ink); font-size: 13px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.picker-row .muted { color: var(--op-ink-faint); font-size: 12px; }
.picker-empty { padding: 36px 0; color: var(--op-ink-faint); font-size: 12px; text-align: center; }
.picker-empty.error { color: var(--op-danger); }
</style>

<style scoped>
.picker-box {
  width: min(570px, calc(100vw - 28px));
  max-height: min(82vh, 700px);
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-top: 3px solid var(--op-accent);
  border-radius: 0;
  background: var(--op-surface);
  box-shadow: var(--op-shadow-lg);
}

.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 15px 18px;
  border-bottom: 1px solid var(--op-rule-strong);
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 23px;
  font-weight: 500;
  letter-spacing: -0.03em;
}

.picker-path {
  padding: 10px 18px;
  border-bottom: 1px solid var(--op-rule);
  background: var(--op-surface-muted);
}

.picker-path :deep(.n-breadcrumb) { font-family: var(--op-font-mono); font-size: 9px; }
.picker-actions { display: flex; justify-content: flex-end; padding: 10px 18px; border-bottom: 1px solid var(--op-rule); }
.picker-list { max-height: min(48vh, 390px); overflow: auto; padding: 0 18px 12px; }

.picker-row {
  display: flex;
  align-items: center;
  gap: 11px;
  min-height: 56px;
  border-bottom: 1px solid var(--op-rule);
  cursor: pointer;
}

.picker-row:hover { background: var(--op-accent-soft); box-shadow: inset 2px 0 0 var(--op-accent); }
.picker-row .name { flex: 1; min-width: 0; overflow: hidden; color: var(--op-ink); font-size: 12px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.picker-row .muted { color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 9px; }
.picker-empty { padding: 42px 0; color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 10px; letter-spacing: 0.05em; text-align: center; text-transform: uppercase; }
.picker-empty.error { color: var(--op-danger); }
</style>
