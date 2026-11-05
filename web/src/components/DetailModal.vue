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
    <div class="detail-box">
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
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 18px;
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
