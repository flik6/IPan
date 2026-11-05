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
    <div class="resolve-box">
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
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 18px;
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
