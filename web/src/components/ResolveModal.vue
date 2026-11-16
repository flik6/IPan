<script setup lang="ts">
import { ref } from "vue";
import { NButton, NInput, NModal } from "naive-ui";
import { api, type ResolveResult } from "../api/client";
import { ui } from "../stores/ui";

const input = ref("");
const result = ref("");
const resultName = ref("");
const packed = ref(false);
const statusText = ref("");
const statusType = ref<"ok" | "error" | "">("");
const resolving = ref(false);

function setStatus(text: string, type: "ok" | "error" | "" = "ok", autoClear = true): void {
  statusText.value = text;
  statusType.value = type;
  if (text && autoClear) {
    setTimeout(() => {
      if (statusText.value === text) {
        statusText.value = "";
        statusType.value = "";
      }
    }, 4000);
  }
}

async function extract(): Promise<void> {
  const url = input.value.trim();
  if (!url) {
    setStatus("请输入分享链接", "error");
    return;
  }
  resolving.value = true;
  try {
    setStatus("正在解析分享链接…", "ok", false);
    const data = await api<ResolveResult>("/api/resolve", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    });
    if (!data?.url) throw new Error("未能解析出下载直链");
    result.value = data.url;
    resultName.value = data.name || "";
    packed.value = !!data.packed;
    setStatus(packed.value ? "多文件分享，已生成打包直链" : "解析成功", "ok");
  } catch (e) {
    result.value = "";
    setStatus((e as Error).message, "error");
  } finally {
    resolving.value = false;
  }
}

async function copyResult(): Promise<void> {
  if (!result.value) {
    setStatus("暂无可复制的直链", "error");
    return;
  }
  if (navigator.clipboard) await navigator.clipboard.writeText(result.value).catch(() => {});
  setStatus("已复制下载直链", "ok");
}

function onShowChange(show: boolean): void {
  ui.showResolve = show;
  if (!show) {
    input.value = "";
    result.value = "";
    resultName.value = "";
    setStatus("", "");
  }
}
</script>

<template>
  <NModal :show="ui.showResolve" @update:show="onShowChange">
    <div class="resolve-box glass-panel">
      <div class="box-head">
        <span>直链解析</span>
        <NButton size="small" @click="ui.showResolve = false">关闭</NButton>
      </div>
      <div class="box-body">
        <div class="row">
          <NInput v-model:value="input" placeholder="请输入分享链接" :disabled="resolving" @keydown.enter.prevent="extract" />
          <NButton type="primary" :loading="resolving" @click="extract">
            {{ resolving ? "解析中…" : "提取" }}
          </NButton>
        </div>
        <div class="row" style="margin-top: 12px">
          <NInput v-model:value="result" readonly placeholder="解析后的下载直链" />
          <NButton type="primary" @click="copyResult">复制</NButton>
        </div>
        <div class="muted hint">
          粘贴分享短链，点击提取即可获取下载直链，无需手动输入提取码。
        </div>
        <div v-if="resultName" class="muted name-line">{{ packed ? "打包下载" : "文件" }}：{{ resultName }}</div>
      </div>
      <div class="status-bar" :class="statusType">{{ statusText }}</div>
    </div>
  </NModal>
</template>

<style scoped>
.resolve-box {
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
  width: min(560px, calc(100vw - 48px));
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
}
.box-body {
  padding: 18px;
}
.row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 12px;
}
.hint {
  margin-top: 12px;
}
.name-line {
  margin-top: 8px;
}
.status-bar {
  min-height: 18px;
  font-size: 13px;
  padding: 6px 16px 12px;
  color: var(--gh-primary);
}
.status-bar.error {
  color: var(--gh-danger);
}
</style>

<style scoped>
.resolve-box { width: min(620px, calc(100vw - 28px)); overflow: hidden; border: 1px solid var(--op-rule); border-radius: 12px; background: #fff; box-shadow: var(--op-shadow-lg); }
.box-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 18px; border-bottom: 1px solid var(--op-rule); color: var(--op-ink); font-size: 18px; font-weight: 600; }
.box-body { display: grid; gap: 10px; padding: 18px; }
.row { display: flex; align-items: center; gap: 8px; }
.row .n-input { flex: 1; }
.hint { color: var(--op-ink-soft); font-size: 12px; line-height: 1.55; }
.name-line { color: var(--op-ink); font-size: 12px; }
.status-bar { min-height: 36px; padding: 9px 18px; border-top: 1px solid var(--op-rule); color: var(--op-ink-faint); background: #f9fafb; font-size: 12px; }
.status-bar.ok { color: var(--op-success); }
.status-bar.error { color: var(--op-danger); }
@media (max-width: 520px) { .row { flex-direction: column; align-items: stretch; } .row .n-button { min-height: 42px; } }
</style>

<style scoped>
.resolve-box {
  width: min(660px, calc(100vw - 28px));
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
  padding: 15px 19px;
  border-bottom: 1px solid var(--op-rule-strong);
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 27px;
  font-weight: 500;
  letter-spacing: -0.04em;
}

.box-body { display: grid; gap: 10px; padding: 20px 19px; }
.row { display: flex; align-items: center; gap: 8px; }
.row .n-input { flex: 1; }
.hint { padding-left: 10px; border-left: 2px solid var(--op-accent); color: var(--op-ink-soft); font-size: 11px; line-height: 1.65; }
.name-line { color: var(--op-ink); font-family: var(--op-font-mono); font-size: 10px; }

.status-bar {
  min-height: 38px;
  padding: 10px 19px;
  border-top: 1px solid var(--op-rule);
  color: var(--op-ink-faint);
  background: var(--op-surface-muted);
  font-family: var(--op-font-mono);
  font-size: 9px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.status-bar.ok { color: var(--op-success); }
.status-bar.error { color: var(--op-danger); }

@media (max-width: 520px) {
  .row { flex-direction: column; align-items: stretch; }
  .row .n-button { min-height: 44px; }
}
</style>
