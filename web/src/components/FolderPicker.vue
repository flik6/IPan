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
    <div class="picker-box">
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
  background: var(--gh-cream-soft);
  border: 1px solid var(--gh-border);
  border-radius: 18px;
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
