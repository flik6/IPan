<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NModal } from "naive-ui";
import { getAuthToken } from "../api/client";
import { ui } from "../stores/ui";
import { previewKind } from "../utils/format";
import AudioPlayer from "./AudioPlayer.vue";
import LoadingSpinner from "./LoadingSpinner.vue";

const kind = computed(() => (ui.previewFile ? previewKind(ui.previewFile.name) : "unknown"));
const extLabel = computed(() => (ui.previewFile?.name.split(".").pop() || "").toLowerCase());

function withAuth(path: string): string {
  const f = ui.previewFile;
  if (!f) return path;
  return (
    path +
    "?fid=" +
    encodeURIComponent(f.fid) +
    "&name=" +
    encodeURIComponent(f.name) +
    "&auth=" +
    encodeURIComponent(getAuthToken())
  );
}

const viewUrl = computed(() => (kind.value === "image" || kind.value === "text" ? withAuth("/api/view") : ""));
const downloadUrl = computed(() => withAuth("/api/download"));

/** 图片/音视频在浏览器真正拿到数据前都会白屏，用这个状态统一驱动加载动画 */
const mediaLoading = ref(false);
const mediaError = ref("");

/** 文本预览：先整体拉到本地再展示，期间显示加载动画 */
const textContent = ref("");
const textError = ref("");

/** 只有能预览的类型才需要在打开弹窗后等待内容 */
const needsLoading = computed(() => kind.value !== "unknown");

const loadingLabel = computed(() => {
  if (kind.value === "image") return "正在加载图片…";
  if (kind.value === "video") return "正在加载视频…";
  if (kind.value === "audio") return "正在加载音频…";
  return "正在加载文件…";
});

function reset(): void {
  mediaLoading.value = needsLoading.value;
  mediaError.value = "";
  textContent.value = "";
  textError.value = "";
}

watch(
  () => [ui.showPreview, ui.previewFile?.fid] as const,
  ([show]) => {
    if (!show) return;
    reset();
    if (kind.value !== "text") return;
    fetch(viewUrl.value)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.text();
      })
      .then((t) => {
        textContent.value = t;
      })
      .catch((e: Error) => {
        textError.value = e?.message || String(e);
      })
      .finally(() => {
        mediaLoading.value = false;
      });
  },
);

function onMediaLoaded(): void {
  mediaLoading.value = false;
}

function onMediaError(): void {
  mediaLoading.value = false;
  mediaError.value = "文件加载失败，可能链接已过期，请重试或改用「下载」保存到本地查看。";
}

function close(): void {
  ui.showPreview = false;
}
</script>

<template>
  <NModal :show="ui.showPreview" @update:show="(v: boolean) => (!v && close())">
    <div class="preview-box glass-panel">
      <div class="preview-head">
        <span class="file-name" :title="ui.previewFile?.name">{{ ui.previewFile?.name }}</span>
        <div class="head-actions">
          <a rel="noreferrer" target="_blank" :href="downloadUrl">
            <NButton size="small">下载</NButton>
          </a>
          <NButton size="small" @click="close">关闭</NButton>
        </div>
      </div>
      <div class="preview-body" :class="{ 'is-media': kind === 'image' || kind === 'video' }">
        <div v-if="mediaLoading" class="preview-loading">
          <LoadingSpinner :size="30" :label="loadingLabel" />
        </div>
        <div v-else-if="mediaError" class="preview-error">{{ mediaError }}</div>

        <template v-if="kind === 'image'">
          <img
            v-show="!mediaLoading && !mediaError"
            draggable="false"
            :src="viewUrl"
            :alt="ui.previewFile?.name"
            @load="onMediaLoaded"
            @error="onMediaError"
          />
        </template>
        <template v-else-if="kind === 'video'">
          <video
            v-show="!mediaLoading && !mediaError"
            :src="downloadUrl"
            controls
            preload="auto"
            playsinline
            @loadeddata="onMediaLoaded"
            @error="onMediaError"
          />
        </template>
        <AudioPlayer
          v-else-if="kind === 'audio'"
          :key="ui.previewFile?.fid || ''"
          :src="downloadUrl"
          @ready="onMediaLoaded"
          @error="onMediaError"
        />
        <pre v-else-if="kind === 'text'">{{
          mediaLoading ? "" : textError ? `预览失败：${textError}` : textContent
        }}</pre>
        <div v-else class="muted">
          暂不支持预览该文件类型（{{ extLabel || "未知" }}），请点击右上角“下载”保存到本地查看。
        </div>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.preview-box {
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
  width: min(960px, calc(100vw - 48px));
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--gh-border);
}
.file-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.head-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex: 0 0 auto;
}
.head-actions a {
  text-decoration: none;
}
.preview-body {
  position: relative;
  padding: 18px;
  overflow: auto;
  flex: 1;
  /* 加载动画需要居中的最小高度，否则短内容时提示会贴边 */
  min-height: 180px;
}
/* 图片/视频区域固定一块画布，避免加载完成时弹窗高度突变 */
.preview-body.is-media {
  display: grid;
  place-items: center;
  min-height: 300px;
  background: var(--gh-surface-2);
}
.preview-loading {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  z-index: 2;
}
.preview-body.is-media .preview-loading {
  position: static;
}
.preview-error {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 24px;
  text-align: center;
  font-size: 13px;
  color: var(--gh-danger);
}
.preview-body img,
.preview-body video {
  max-width: 100%;
  max-height: 70vh;
  display: block;
  margin: 0 auto;
  border-radius: 16px;
  box-shadow: var(--gh-shadow-md);
  user-select: none;
}
.preview-body.is-media img,
.preview-body.is-media video {
  box-shadow: none;
}
.preview-body pre {
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
  line-height: 1.7;
  margin: 0;
  background: var(--gh-surface-2);
  padding: 14px;
  border-radius: 14px;
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 13px;
}
</style>

<style scoped>
.preview-box { display: flex; flex-direction: column; width: min(1080px, calc(100vw - 32px)); height: min(86vh, 820px); overflow: hidden; border: 1px solid var(--op-rule); border-radius: 12px; background: #111827; box-shadow: var(--op-shadow-lg); }
.preview-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; flex: 0 0 auto; padding: 11px 14px; border-bottom: 1px solid var(--op-rule); color: var(--op-ink); background: #fff; }
.preview-head .file-name { min-width: 0; overflow: hidden; color: var(--op-ink); font-size: 15px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.head-actions { display: flex; align-items: center; gap: 7px; }
.preview-body { position: relative; display: grid; place-items: center; flex: 1; min-height: 0; overflow: auto; padding: 20px; background: #111827; }
.preview-body.is-media { padding: 0; }
.preview-body img { max-width: 100%; max-height: 100%; object-fit: contain; }
.preview-body video { width: 100%; max-height: 100%; background: #000; }
.preview-body pre { width: min(900px, 100%); min-height: 100%; margin: 0; padding: 20px; overflow: auto; color: #e5e7eb; background: #0b1017; font-family: var(--op-font-mono); font-size: 12px; line-height: 1.65; white-space: pre-wrap; overflow-wrap: anywhere; }
.preview-loading, .preview-error, .preview-body > .muted { color: #cbd5e1; font-size: 13px; }
.preview-error { max-width: 52ch; color: #fda29b; text-align: center; line-height: 1.6; }
@media (max-width: 640px) { .preview-box { width: 100vw; height: 100dvh; border: 0; border-radius: 0; } .preview-head { padding: 9px 10px; } .preview-body { padding: 10px; } }
</style>

<style scoped>
.preview-box {
  display: flex;
  flex-direction: column;
  width: min(1120px, calc(100vw - 32px));
  height: min(88vh, 860px);
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-radius: 0;
  background: #0d0c0a;
  box-shadow: var(--op-shadow-lg);
}

.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex: 0 0 auto;
  padding: 12px 16px;
  border-bottom: 1px solid #302c25;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
}

.preview-head .file-name {
  min-width: 0;
  overflow: hidden;
  font-family: var(--op-font-display);
  font-size: 22px;
  font-weight: 500;
  letter-spacing: -0.03em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.head-actions { display: flex; align-items: center; gap: 7px; }
.head-actions .n-button { --n-color: transparent !important; --n-text-color: var(--op-feature-ink) !important; --n-border: 1px solid #4b453a !important; }

.preview-body {
  position: relative;
  display: grid;
  place-items: center;
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 20px;
  background: #0d0c0a;
}

.preview-body.is-media { padding: 0; }
.preview-body img { display: block; max-width: 100%; max-height: 100%; object-fit: contain; }
.preview-body video { display: block; width: 100%; max-height: 100%; background: #000; }
.preview-body pre {
  width: min(900px, 100%);
  min-height: 100%;
  margin: 0;
  padding: 22px;
  overflow: auto;
  color: #e7e0d2;
  background: #12100d;
  font-family: var(--op-font-mono);
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.preview-loading,
.preview-error { color: #bcb3a4; font-size: 12px; }
.preview-error { max-width: 52ch; color: #f09686; text-align: center; line-height: 1.7; }
.preview-body > .muted { color: #bcb3a4; font-size: 12px; }

@media (max-width: 640px) {
  .preview-box { width: 100vw; height: 100dvh; border: 0; }
  .preview-head { padding: 10px 12px; }
  .preview-head .file-name { font-size: 18px; }
  .preview-body { padding: 10px; }
}
</style>
