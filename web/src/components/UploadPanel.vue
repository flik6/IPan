<script setup lang="ts">
import { computed } from "vue";
import { NButton, NProgress } from "naive-ui";
import { upload, togglePanel, closePanel, cancelUpload } from "../stores/upload";
import { formatSize, formatSpeed } from "../utils/format";
import Icon from "./Icon.vue";

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
    :aria-label="upload.panelOpen ? '收起传输任务' : '查看传输任务'"
    :aria-expanded="upload.panelOpen"
    @click="togglePanel"
  >
    <Icon name="upload" :size="23" />
    <span v-if="!upload.panelOpen" class="fab-badge" :class="{ error: hasError && !upload.uploading }" aria-hidden="true">
      <template v-if="activeCount">{{ activeCount }}</template>
      <Icon v-else :name="hasError ? 'warning' : 'check'" :size="12" />
    </span>
  </button>

  <!-- 浮动传输面板 -->
  <Transition name="drawer">
    <section v-if="upload.panelOpen" class="upload-drawer glass-panel" role="dialog" aria-label="传输任务">
      <header class="drawer-head">
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
          <button type="button" class="head-chevron" aria-label="关闭传输任务面板" @click="closePanel">
            <Icon name="chevron-down" :size="16" />
          </button>
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
  color: var(--gh-on-action);
  background: var(--gh-action);
  box-shadow: var(--gh-shadow-md);
  cursor: pointer;
  overflow: visible;
  transition: filter 160ms ease;
}
.upload-fab:hover {
  filter: brightness(1.12);
}
.upload-fab.uploading > svg {
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
  display: grid;
  place-items: center;
  background: var(--gh-accent);
  color: var(--gh-on-accent);
  font-size: 11px;
  line-height: 20px;
  text-align: center;
}
.fab-badge.error {
  background: var(--gh-danger-bg);
  color: var(--gh-danger);
}

.upload-drawer {
  position: fixed;
  right: 22px;
  bottom: 22px;
  width: min(450px, calc(100% - 44px));
  z-index: 260;
  display: flex;
  flex-direction: column;
  height: min(360px, calc(100dvh - 44px));
  overflow: hidden;
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
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
  user-select: none;
  flex: 0 0 auto;
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
  background: var(--gh-action);
  color: var(--gh-on-action);
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
  width: 44px;
  height: 44px;
  padding: 0;
  border: 1px solid var(--gh-border);
  border-radius: 8px;
  background: var(--gh-surface);
  color: var(--gh-ink-soft);
  cursor: pointer;
}
.head-chevron:hover { color: var(--gh-primary); background: var(--gh-primary-soft); }
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
    bottom: var(--gh-mobile-dock-clearance, 14px);
  }
  .upload-drawer {
    right: 14px;
    bottom: var(--gh-mobile-dock-bottom);
    width: calc(100% - 28px);
    height: min(300px, calc(100dvh - 36px));
  }
  .drawer-head,
  .drawer-body {
    padding-left: 14px;
    padding-right: 14px;
  }
}
</style>

<style scoped>
.upload-fab { right: 22px; bottom: 22px; width: 50px; height: 50px; border-radius: 12px; color: #fff; background: var(--db-blue); box-shadow: var(--op-shadow-md); }
.upload-fab:hover { background: var(--db-blue-hover); }
.fab-badge { top: -5px; right: -5px; min-width: 20px; height: 20px; border-radius: 999px; color: #fff; background: var(--db-blue); font-size: 10px; }
.fab-badge.error { color: var(--op-danger); background: var(--op-danger-soft); }
.upload-drawer { right: 22px; bottom: 82px; width: min(420px, calc(100% - 44px)); height: min(360px, calc(100dvh - 110px)); overflow: hidden; border: 1px solid var(--op-rule); border-radius: 12px; background: #fff; box-shadow: var(--op-shadow-lg); }
.drawer-head { padding: 13px 16px; border-bottom: 1px solid var(--op-rule); background: #fff; }
.drawer-title { color: var(--op-ink); font-size: 16px; font-weight: 600; }
.head-badge { min-width: 18px; height: 18px; border-radius: 999px; color: #fff; background: var(--db-blue); font-size: 10px; line-height: 18px; }
.speed { color: var(--op-ink-faint); font-size: 11px; }
.head-chevron { width: 32px; height: 32px; border: 0; border-radius: 8px; color: var(--op-ink-soft); background: transparent; }
.head-chevron:hover { color: var(--db-blue); background: var(--db-blue-soft); }
.drawer-body { padding: 0 16px 12px; }
.empty { padding: 30px 0; color: var(--op-ink-faint); font-size: 12px; text-align: center; }
.task-item { padding: 12px 0; border-bottom: 1px solid var(--op-rule); }
.task-name { color: var(--op-ink); font-size: 13px; font-weight: 500; }
.task-percent { color: var(--db-blue); font-size: 12px; font-weight: 600; }
.task-meta { margin: 4px 0 7px; color: var(--op-ink-faint); font-size: 11px; }
.task-item :deep(.n-progress-graph-line-rail) { background: var(--op-rule) !important; }
.task-item :deep(.n-progress-graph-line-fill) { background: var(--db-blue) !important; }
.task-item.done .task-status { color: var(--op-success); }
.task-item.error .task-status, .task-item.cancelled .task-status { color: var(--op-danger); }
@media (max-width: 760px) { .upload-fab { right: 14px; bottom: calc(var(--op-mobile-nav) + 14px + env(safe-area-inset-bottom, 0px)); } .upload-drawer { right: 10px; bottom: calc(var(--op-mobile-nav) + 18px + env(safe-area-inset-bottom, 0px)); width: calc(100% - 20px); } }
</style>

<style scoped>
.upload-fab {
  right: 24px;
  bottom: 24px;
  width: 54px;
  height: 54px;
  border-radius: 0;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
  box-shadow: var(--op-shadow-md);
}

.upload-fab:hover { background: var(--op-accent); }
.upload-fab.uploading > svg { animation: fab-pulse 1.6s ease-in-out infinite; }

.fab-badge {
  top: -5px;
  right: -5px;
  min-width: 20px;
  height: 20px;
  border-radius: 0;
  color: var(--op-on-accent);
  background: var(--op-accent);
  font-family: var(--op-font-mono);
  font-size: 9px;
}

.fab-badge.error { color: var(--op-danger); background: var(--op-danger-soft); }

.upload-drawer {
  right: 24px;
  bottom: 90px;
  width: min(430px, calc(100% - 48px));
  height: min(380px, calc(100dvh - 120px));
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-radius: 0;
  background: var(--op-surface);
  box-shadow: var(--op-shadow-lg);
}

.drawer-head {
  padding: 15px 18px;
  border-bottom: 1px solid var(--op-rule-strong);
  background: var(--op-surface-raised);
}

.drawer-title { color: var(--op-ink); font-family: var(--op-font-display); font-size: 24px; font-weight: 500; letter-spacing: -0.03em; }
.head-badge { min-width: 18px; height: 18px; border-radius: 0; color: var(--op-on-accent); background: var(--op-accent); font-family: var(--op-font-mono); font-size: 9px; line-height: 18px; }
.speed { color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 9px; }

.head-chevron {
  width: 36px;
  height: 36px;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-ink-soft);
  background: transparent;
}

.head-chevron:hover { color: var(--op-accent); border-color: var(--op-accent); background: transparent; }
.drawer-body { padding: 0 18px 14px; }
.empty { padding: 32px 0; color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 10px; letter-spacing: 0.06em; text-transform: uppercase; }

.task-item { padding: 14px 0; border-bottom: 1px solid var(--op-rule); }
.task-row, .task-meta { gap: 12px; }
.task-name { color: var(--op-ink); font-size: 12px; font-weight: 650; }
.task-percent { color: var(--op-accent); font-family: var(--op-font-mono); font-size: 10px; }
.task-meta { margin: 5px 0 8px; color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 9px; }
.task-status { text-transform: uppercase; }

.task-item :deep(.n-progress-graph-line-rail) { background: var(--op-rule) !important; }
.task-item :deep(.n-progress-graph-line-fill) { background: var(--op-accent) !important; }
.task-item.done .task-status { color: var(--op-success); }
.task-item.error .task-status, .task-item.cancelled .task-status { color: var(--op-danger); }

@media (max-width: 760px) {
  .upload-fab { right: 14px; bottom: calc(var(--op-mobile-nav) + 14px + env(safe-area-inset-bottom, 0px)); }
  .upload-drawer { right: 10px; bottom: calc(var(--op-mobile-nav) + 18px + env(safe-area-inset-bottom, 0px)); width: calc(100% - 20px); height: min(360px, calc(100dvh - 150px)); }
}
</style>
