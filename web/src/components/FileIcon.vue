<script setup lang="ts">
import type { FileIconKind } from "../utils/format";

defineProps<{ kind: FileIconKind }>();
</script>

<template>
  <svg class="file-icon" :class="kind" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <template v-if="kind === 'folder'">
      <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" class="icon-fill" />
    </template>
    <template v-else-if="kind === 'image'">
      <rect x="3" y="5" width="18" height="14" rx="2" class="icon-fill" />
      <circle cx="9" cy="10" r="1.6" fill="currentColor" stroke="none" />
      <path d="M4.5 18l5-5 3.5 3.5L16 13l4 5" />
    </template>
    <template v-else-if="kind === 'video'">
      <rect x="3" y="5" width="18" height="14" rx="2" class="icon-fill" />
      <path d="M10 9.5v5l4.5-2.5L10 9.5z" fill="currentColor" stroke="none" />
    </template>
    <template v-else-if="kind === 'music'">
      <circle cx="8" cy="17" r="2.5" class="icon-fill" />
      <circle cx="17" cy="15.5" r="2.5" class="icon-fill" />
      <path d="M10.5 17V7l9-1.5v10" />
    </template>
    <template v-else-if="kind === 'archive'">
      <rect x="4" y="3" width="16" height="18" rx="2" class="icon-fill" />
      <path d="M12 3v4m0 2v2m0 2v1.5" />
      <rect x="10" y="14.5" width="4" height="4" rx="1" fill="currentColor" stroke="none" />
    </template>
    <template v-else>
      <path d="M6 3h8l4 4v13a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" class="icon-fill" />
      <path d="M14 3v4h4M8.5 12h7m-7 3h7" />
    </template>
  </svg>
</template>

<style scoped>
.file-icon {
  width: 26px;
  height: 26px;
  flex: 0 0 26px;
  color: var(--db-blue);
  --file-fill: #e8f0ff;
}
.icon-fill { fill: var(--file-fill); }
.folder { color: var(--db-blue); --file-fill: var(--db-blue); }
.image,
.video,
.music,
.archive { color: var(--db-blue); --file-fill: #e8f0ff; }
</style>
