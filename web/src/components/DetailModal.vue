<script setup lang="ts">
import { computed } from "vue";
import { NButton, NModal } from "naive-ui";
import { ui } from "../stores/ui";
import { formatSize, formatTime } from "../utils/format";

const rows = computed(() => {
  const f = ui.detailFile;
  if (!f) return [];
  const isDir = Number(f.type) === 0;
  return [
    ["名称", f.name || "-"],
    ["ID", f.id || "-"],
    ["FID", f.fid || "-"],
    ["类型", isDir ? "文件夹" : "文件"],
    ["大小", isDir ? "-" : formatSize(Number(f.size || 0))],
    ["创建时间", formatTime(f.createTime || f.updateTime || f.uploadTime || f.shootingTime || "")],
    ["父目录 ID", f.parentDirectoryId || "-"],
  ];
});

function close(): void {
  ui.showDetail = false;
}
</script>

<template>
  <NModal :show="ui.showDetail" @update:show="(v: boolean) => (!v && close())">
    <div class="detail-box glass-panel">
      <div class="preview-head">
        <span class="file-name" :title="ui.detailFile?.name">{{ ui.detailFile?.name || "详情" }}</span>
        <NButton size="small" @click="close">关闭</NButton>
      </div>
      <div class="preview-body">
        <table class="detail-table">
          <tbody>
            <tr v-for="[label, value] in rows" :key="label">
              <th>{{ label }}</th>
              <td>{{ value }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.detail-box {
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
  width: min(560px, calc(100vw - 48px));
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
.preview-body {
  padding: 18px;
  overflow: auto;
}
.detail-table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
}
.detail-table th {
  width: 120px;
  background: var(--gh-surface-2);
  color: var(--gh-ink-soft);
  font-weight: 700;
  text-align: left;
  padding: 10px 14px;
  border-bottom: 1px solid var(--gh-row-border);
}
.detail-table td {
  padding: 10px 14px;
  border-bottom: 1px solid var(--gh-row-border);
  overflow-wrap: anywhere;
}
.detail-table tr:last-child th,
.detail-table tr:last-child td {
  border-bottom: 0;
}
</style>

<style scoped>
.detail-box { width: min(620px, calc(100vw - 28px)); max-height: min(82vh, 700px); overflow: auto; border: 1px solid var(--op-rule); border-radius: 12px; background: #fff; box-shadow: var(--op-shadow-lg); }
.preview-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 18px; border-bottom: 1px solid var(--op-rule); }
.preview-head .file-name { overflow: hidden; color: var(--op-ink); font-size: 18px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.preview-body { padding: 0 18px 14px; }
.detail-table { width: 100%; border-collapse: collapse; }
.detail-table tr { border-bottom: 1px solid var(--op-rule); }
.detail-table th { width: 140px; padding: 11px 0; color: var(--op-ink-soft); font-size: 12px; font-weight: 500; text-align: left; }
.detail-table td { padding: 11px 0; overflow-wrap: anywhere; color: var(--op-ink); font-size: 13px; }
.detail-table tr:last-child { border-bottom: 0; }
</style>

<style scoped>
.detail-box {
  width: min(640px, calc(100vw - 28px));
  max-height: min(82vh, 720px);
  overflow: auto;
  border: 1px solid var(--op-rule-strong);
  border-top: 3px solid var(--op-accent);
  border-radius: 0;
  background: var(--op-surface);
  box-shadow: var(--op-shadow-lg);
}

.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 15px 19px;
  border-bottom: 1px solid var(--op-rule-strong);
}

.preview-head .file-name {
  overflow: hidden;
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 28px;
  font-weight: 500;
  letter-spacing: -0.04em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-body { padding: 0 19px 16px; }
.detail-table { width: 100%; border-collapse: collapse; }
.detail-table tr { border-bottom: 1px solid var(--op-rule); }
.detail-table th {
  width: 150px;
  padding: 12px 0;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.07em;
  text-align: left;
  text-transform: uppercase;
}

.detail-table td { padding: 12px 0; overflow-wrap: anywhere; color: var(--op-ink); font-size: 12px; }
</style>
