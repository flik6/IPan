<script setup lang="ts">
import { computed } from "vue";
import { NButton, NProgress } from "naive-ui";
import { upload, togglePanel, closePanel, cancelUpload } from "../stores/upload";
import { formatSize, formatSpeed } from "../utils/format";

const STATUS_TEXT: Record<string, string> = {
  pending: "等待上传",
  preparing: "正在准备",
  uploading: "正在上传",
  done: "上传完成",
  error: "上传失败",
  cancelled: "已取消",
};

const hasError = computed(() => upload.tasks.some((t) => t.status === "error"));
const activeCount = computed(() => upload.tasks.filter((t) => !["done", "error", "cancelled"].includes(t.status)).length);

function taskStatus(t: { status: string; error: string; partIndex: number; totalPart: number }): string {
  const base = t.status === "error" ? t.error || "上传失败" : STATUS_TEXT[t.status] || t.status;
  const part = t.totalPart ? ` · 分片 ${t.partIndex}/${t.totalPart}` : "";
  return base + part;
}
</script>

<template>
  <!-- 右下角 FAB：有任务时常驻 -->
  <button
    v-if="upload.fabVisible && upload.tasks.length"
    class="upload-fab"
    :class="{ uploading: upload.uploading, open: upload.panelOpen }"
    type="button"
    :title="upload.panelOpen ? '收起传输列表' : '查看传输任务'"
    aria-label="传输任务"
    @click="togglePanel"
  >
    <img src="/upload.svg" alt="" />
    <span v-if="!upload.panelOpen" class="fab-badge" :class="{ error: hasError && !upload.uploading }">{{ activeCount || "✓" }}</span>
  </button>

  <!-- 底部抽屉：固定高度，点头部开合 -->
  <Transition name="drawer">
    <section v-if="upload.panelOpen" class="upload-drawer" role="dialog" aria-label="传输任务">
      <header class="drawer-head" type="button" title="点击收起" @click="closePanel">
        <span class="drawer-title">
          传输任务
          <em v-if="activeCount" class="head-badge">{{ activeCount }}</em>
        </span>
        <span class="head-right">
          <span class="speed">{{ upload.uploading ? formatSpeed(upload.speed) : "" }}</span>
          <NButton
            v-if="upload.uploading"
            size="tiny"
            type="error"
            secondary
            :disabled="upload.cancelling"
            @click.stop="cancelUpload"
          >
            {{ upload.cancelling ? "正在放弃…" : "放弃" }}
          </NButton>
          <span class="head-chevron" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="m6 15 6-6 6 6" /></svg>
          </span>
        </span>
      </header>
      <div class="drawer-body">
        <div v-if="!upload.tasks.length" class="empty muted">暂无上传任务</div>
        <div v-for="task in upload.tasks" :key="task.id" class="task-item" :class="task.status">
          <div class="task-row">
            <span class="task-name" :title="task.name">{{ task.name }}</span>
            <span class="task-percent">{{ task.percent }}%</span>
          </div>
          <div class="task-meta">
            <span class="task-status">{{ taskStatus(task) }}</span>
            <span>{{ formatSize(task.uploaded) }} / {{ formatSize(task.size) }}</span>
          </div>
          <NProgress
            type="line"
            :percentage="task.percent"
            :show-indicator="false"
            :height="6"
            border-radius="999px"
            color="var(--gh-primary)"
            rail-color="var(--gh-primary-soft)"
            :status="task.status === 'error' ? 'error' : task.status === 'cancelled' ? 'warning' : 'default'"
          />
        </div>
      </div>
    </section>
  </Transition>
</template>

<style scoped>
.upload-fab {
  position: fixed;
  right: 22px;
  bottom: 22px;
  z-index: 255;
  width: 52px;
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: var(--gh-primary);
  box-shadow: 0 8px 24px rgba(109, 123, 245, 0.35);
  cursor: pointer;
  overflow: visible;
  transition:
    transform 0.3s ease-out,
    background 0.3s;
}
.upload-fab:hover {
  background: var(--gh-primary-deep);
  transform: translateY(-2px);
}
.upload-fab img {
  width: 26px;
  height: 26px;
  object-fit: contain;
}
.upload-fab.uploading img {
  animation: fab-pulse 1.6s ease-in-out infinite;
}
@keyframes fab-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}
.fab-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  border-radius: 10px;
  background: var(--gh-primary-deep);
  color: #fff;
  font-size: 11px;
  line-height: 20px;
  text-align: center;
}
.fab-badge.error {
  background: var(--gh-danger);
}

.upload-drawer {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 260;
  display: flex;
  flex-direction: column;
  height: 320px;
  overflow: hidden;
  border-top: 1px solid var(--gh-border);
  border-radius: 18px 18px 0 0;
  background: var(--gh-surface);
  box-shadow: 0 -10px 34px rgba(38, 43, 74, 0.16);
}
.drawer-enter-active,
.drawer-leave-active {
  transition:
    opacity 0.24s ease-out,
    transform 0.24s ease-out;
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
  transform: translateY(24px);
}
.drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 20px;
  border-bottom: 1px solid var(--gh-border);
  cursor: pointer;
  user-select: none;
  flex: 0 0 auto;
}
.drawer-head:hover {
  background: var(--gh-surface-2);
}
.drawer-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 700;
  color: var(--gh-ink);
}
.head-badge {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--gh-gradient);
  color: #fff;
  font-size: 11px;
  font-style: normal;
  line-height: 18px;
  text-align: center;
}
.head-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
.speed {
  color: var(--gh-ink-soft);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.head-chevron {
  display: grid;
  place-items: center;
  color: var(--gh-ink-soft);
}
.drawer-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 6px 20px 14px;
}
.empty {
  padding: 28px 0;
  text-align: center;
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 13px;
}
.task-item {
  padding: 11px 0;
  border-bottom: 1px solid var(--gh-border);
}
.task-item:last-child {
  border-bottom: 0;
}
.task-row,
.task-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.task-name {
  min-width: 0;
  overflow: hidden;
  color: var(--gh-ink);
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.task-percent {
  flex: 0 0 auto;
  color: var(--gh-primary);
  font-weight: 700;
}
.task-meta {
  margin: 5px 0 7px;
  color: var(--gh-ink-soft);
  font-size: 12px;
}
.task-status {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.task-item.done .task-status {
  color: var(--gh-success);
}
.task-item.error .task-status,
.task-item.cancelled .task-status {
  color: var(--gh-danger);
}
@media (max-width: 760px) {
  .upload-fab {
    right: 14px;
    bottom: 14px;
  }
  .upload-drawer {
    height: 260px;
  }
  .drawer-head,
  .drawer-body {
    padding-left: 14px;
    padding-right: 14px;
  }
}
</style>
