<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import {
  NButton,
  NCheckbox,
  NDropdown,
  NInput,
  NModal,
  useDialog,
} from "naive-ui";
import { api, getAuthToken, type SelectedItem, type WopanFile } from "../api/client";
import {
  files,
  selection,
  currentId,
  currentDirectoryName,
  load,
  loadMore,
  openDirectory,
  jumpTo,
  fileIds,
  selectedItems,
  toggleSelect,
  clearSelection,
} from "../stores/files";import { uploadFiles } from "../stores/upload";
import { showToast } from "../stores/toast";
import { formatSize, fileTime, iconKindForFile } from "../utils/format";
import { ui, openFolderPicker, openShareDialog } from "../stores/ui";
import FileIcon from "./FileIcon.vue";
import Icon from "./Icon.vue";
import LoadingSpinner from "./LoadingSpinner.vue";

const dialog = useDialog();

// ---- 响应式断点 ----
const isMobile = ref(window.innerWidth <= 760);
function onResize(): void {
  isMobile.value = window.innerWidth <= 760;
}
onMounted(() => window.addEventListener("resize", onResize));
onUnmounted(() => window.removeEventListener("resize", onResize));

const busy = ref(0);
const isBusy = computed(() => busy.value > 0);

/**
 * 空态单元格的 colspan 必须等于「当前可见」的列数。移动端「大小」「时间」两列被 CSS 隐藏，
 * 仍用 colspan=5 时固定布局会把跨列宽度分摊到已隐藏的列上，导致「名称」列只剩一小段、
 * 右侧出现大片空白（加载中/空目录时表头只画到一半）。
 */
const emptyColspan = computed(() => (isMobile.value ? 3 : 5));

const shownFiles = computed(() => {
  const q = ui.fileSearch.trim().toLowerCase();
  if (!q) return files.files;
  return files.files.filter((f) => (f.name || "").toLowerCase().includes(q));
});

const allShownSelected = computed(
  () => shownFiles.value.length > 0 && shownFiles.value.every((f) => selection.checkedIds.has(fileIds(f).id)),
);

function toggleAllShown(checked: boolean): void {
  for (const f of shownFiles.value) {
    const { id } = fileIds(f);
    if (checked) selection.checkedIds.add(id);
    else selection.checkedIds.delete(id);
  }
  selection.selectAll = allShownSelected.value;
}

// ---- 新建文件夹 ----
const mkdirVisible = ref(false);
const mkdirNameInput = ref("");
const mkdirBusy = ref(false);

function openMkdir(): void {
  mkdirNameInput.value = "";
  mkdirBusy.value = false;
  mkdirVisible.value = true;
}

async function confirmMkdir(): Promise<void> {
  const name = mkdirNameInput.value.trim();
  if (!name || mkdirBusy.value) return;
  mkdirBusy.value = true;
  try {
    await api("/api/mkdir?parentId=" + currentId() + "&name=" + encodeURIComponent(name), { method: "POST" });
    showToast("创建成功", "success");
    mkdirVisible.value = false;
    await load();
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    mkdirBusy.value = false;
  }
}

// ---- 重命名（行内编辑，默认选中不含后缀的主文件名） ----
const renamingId = ref("");
const renameDraft = ref("");
const renameBusy = ref(false);
/** 当前正在编辑的输入框：v-for 里用模板 ref 会拿到数组，这里用函数式 ref 直接存元素 */
let renameEl: HTMLInputElement | null = null;

function setRenameRef(el: unknown): void {
  renameEl = (el as HTMLInputElement | null) || null;
}

/** 拆分主文件名与后缀：Windows 语义下 a.tar.gz 只选中 "a.tar" */
function splitFileName(name: string, isDir: boolean): { base: string; ext: string } {
  if (isDir) return { base: name, ext: "" };
  const dot = name.lastIndexOf(".");
  if (dot <= 0) return { base: name, ext: "" };
  return { base: name.slice(0, dot), ext: name.slice(dot) };
}

function startRename(f: WopanFile): void {
  const { id } = fileIds(f);
  if (!id) return;
  renamingId.value = id;
  renameDraft.value = displayName(f);
  void nextTick(() => {
    const input = renameEl;
    if (!input) return;
    input.focus();
    const isDir = Number(f.type) === 0;
    const { base } = splitFileName(input.value, isDir);
    input.setSelectionRange(0, base.length);
  });
}

function cancelRename(): void {
  if (renameBusy.value) return;
  renamingId.value = "";
  renameDraft.value = "";
}

async function commitRename(f: WopanFile): Promise<void> {
  if (!renamingId.value || renameBusy.value) return;
  const name = renameDraft.value.trim();
  const current = displayName(f);
  if (!name || name === current) {
    cancelRename();
    return;
  }
  const { id } = fileIds(f);
  const type = Number(f.type) === 0 ? 0 : 1;
  renameBusy.value = true;
  try {
    await api(
      "/api/rename?fid=" + encodeURIComponent(id) + "&type=" + type + "&name=" + encodeURIComponent(name),
      { method: "POST" },
    );
    showToast("重命名成功", "success");
    renamingId.value = "";
    await load();
  } catch (e) {
    showToast((e as Error).message, "error");
    // 失败时留在编辑态，方便直接改名后重试
    void nextTick(() => renameEl?.focus());
  } finally {
    renameBusy.value = false;
  }
}

// ---- 删除 ----
interface DeleteRow {
  id: string;
  type: number;
  label: string;
}

function confirmDelete(rows: DeleteRow[], afterDone: () => Promise<void>): void {
  const dialogReactive = dialog.warning({
    title: "删除确认",
    content: rows.length === 1 ? `确定删除「${rows[0].label}」？` : `确定删除选中的 ${rows.length} 项？`,
    positiveText: "删除",
    negativeText: "取消",
    // 请求期间按钮转圈，避免用户以为没点中而重复点击
    onPositiveClick: async () => {
      dialogReactive.loading = true;
      busy.value++;
      try {
        const dirList = rows.filter((it) => it.type === 0).map((it) => it.id);
        const fileList = rows.filter((it) => it.type === 1).map((it) => it.id);
        await api("/api/delete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ dirList, fileList }),
        });
        showToast("删除成功", "success");
      } catch (e) {
        showToast((e as Error).message, "error");
      } finally {
        busy.value--;
        dialogReactive.loading = false;
      }
      await afterDone();
    },
  });
}

function deleteOne(f: WopanFile): void {
  const { id } = fileIds(f);
  confirmDelete([{ id, type: Number(f.type) === 0 ? 0 : 1, label: f.name || id }], () => load());
}

// ---- 分享 ----
const shareBusyId = ref("");

function toShareItem(f: WopanFile): SelectedItem {
  const { id, fid } = fileIds(f);
  return { id, fid, type: Number(f.type) === 0 ? 0 : 1, name: f.name || id };
}

/** 单文件/文件夹分享：打开分享弹窗（可选多种方式，创建期间按钮转圈） */
function shareOne(f: WopanFile): void {
  shareBusyId.value = fileIds(f).id;
  openShareDialog([toShareItem(f)]);
  void nextTick(() => {
    shareBusyId.value = "";
  });
}

function shareBulk(): void {
  const sel = selectedItems();
  if (sel.length) openShareDialog(sel);
}

// ---- 打包下载 ----
async function packDownload(items: SelectedItem[], zipName: string): Promise<void> {
  if (!items.length) return;
  busy.value++;
  try {
    const res = await fetch("/api/download-zip?auth=" + encodeURIComponent(getAuthToken()), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items, name: zipName }),
    });
    if (!res.ok) {
      let msg = await res.text();
      try {
        msg = JSON.parse(msg).error || msg;
      } catch {
        /* raw */
      }
      throw new Error(msg);
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.rel = "noreferrer";
    a.download = zipName + ".zip";
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
    showToast(`已生成压缩包（${zipName}.zip），共 ${items.length} 个项目`, "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    busy.value--;
  }
}

async function downloadFolder(f: WopanFile): Promise<void> {
  const { id } = fileIds(f);
  await packDownload([{ id, fid: id, type: 0, name: f.name || id }], f.name || id);
}

async function bulkDownload(): Promise<void> {
  const sel = selectedItems();
  if (!sel.length) return;
  const zipName = sel.length === 1 ? sel[0].name || "download" : "ipan-" + new Date().toISOString().slice(0, 10);
  await packDownload(sel, zipName);
}

// ---- 移动 / 复制（选择目标文件夹后由 FolderPicker 调 stores/ops.doMoveCopy 执行） ----

// ---- 上传 ----
const fileInput = ref<HTMLInputElement | null>(null);
const folderInput = ref<HTMLInputElement | null>(null);

function pickFiles(): void {
  fileInput.value?.click();
}

function pickFolder(): void {
  folderInput.value?.click();
}

async function onFilesChosen(): Promise<void> {
  const list = Array.from(fileInput.value?.files || []);
  if (fileInput.value) fileInput.value.value = "";
  await startUpload(list, "选择");
}

/** 上传文件夹：把 webkitRelativePath 里的缺失目录逐级创建出来 */
async function onFolderChosen(): Promise<void> {
  const list = Array.from(folderInput.value?.files || []);
  if (folderInput.value) folderInput.value.value = "";
  if (!list.length) return;
  busy.value++;
  try {
    // pathPrefix -> directoryId 映射，根目录即当前目录
    const dirCache = new Map<string, string>([["", currentId()]]);
    const dirIdFor = async (relPath: string): Promise<string> => {
      const existing = dirCache.get(relPath);
      if (existing) return existing;
      const parts = relPath.split("/");
      const name = parts.pop() as string;
      const parentId = await dirIdFor(parts.join("/"));
      const id = await ensureDirectory(parentId, name);
      dirCache.set(relPath, id);
      return id;
    };
    // 按目录分组，同目录的文件一次性上传
    const byDir = new Map<string, File[]>();
    for (const file of list) {
      const rel = file.webkitRelativePath || "";
      const dirPath = rel.includes("/") ? rel.slice(0, rel.lastIndexOf("/")) : "";
      const arr = byDir.get(dirPath) || [];
      arr.push(file);
      byDir.set(dirPath, arr);
    }
    let failed = 0;
    for (const [dirPath, dirFiles] of byDir) {
      let directoryId = currentId();
      try {
        if (dirPath) directoryId = await dirIdFor(dirPath);
      } catch (e) {
        failed += dirFiles.length;
        showToast(`创建目录「${dirPath}」失败：${(e as Error).message}`, "error");
        continue;
      }
      await uploadFiles(dirFiles, directoryId, {
        onError: (msg) => {
          failed += dirFiles.length;
          showToast(msg, "error");
        },
      });
    }
    if (!failed) showToast("文件夹上传完成", "success");
    else if (failed < list.length) showToast(`文件夹上传完成，${failed} 个文件失败`, "warning");
  } finally {
    busy.value--;
  }
  await load();
}

async function ensureDirectory(parentId: string, name: string): Promise<string> {
  // 先看当前目录列表里有没有同名文件夹，避免重复创建
  await refreshDirectoryFiles(parentId);
  const existing = dirFilesCache.value.find((f) => Number(f.type) === 0 && f.name === name);
  if (existing) return fileIds(existing).id;
  const res = await api<{ id?: string }>("/api/mkdir?parentId=" + parentId + "&name=" + encodeURIComponent(name), {
    method: "POST",
  });
  const id = res?.id;
  if (!id) throw new Error("服务端未返回新目录 ID");
  // 新目录加入缓存，供后续层级复用
  dirFilesCache.value = [...dirFilesCache.value, { id, fid: id, name, type: 0 } as WopanFile];
  return id;
}

/** ensureDirectory 用的目录内容缓存（仅当前上传任务生命周期内有效） */
const dirFilesCache = ref<WopanFile[]>([]);
const dirFilesCacheFor = ref("");
async function refreshDirectoryFiles(parentId: string): Promise<void> {
  if (dirFilesCacheFor.value === parentId) return;
  const res = await api<{ files?: WopanFile[] }>("/api/list?parentId=" + parentId + "&page=0&pageSize=1000");
  dirFilesCache.value = res.files || [];
  dirFilesCacheFor.value = parentId;
}

async function startUpload(list: File[], source: string): Promise<void> {
  const directoryId = currentId();
  await uploadFiles(list, directoryId, {
    onSuccess: (count) => {
      showToast(`${source}上传完成：${count} 个文件，目录：${currentDirectoryName()}`, "success");
    },
    onError: (msg) => showToast(msg, "error"),
    onFinished: () => {
      if (currentId() === directoryId && !files.loading) void load();
    },
  });
}

// 拖拽 / 粘贴上传（全局监听）
let dragDepth = 0;
function hasDraggedFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types || []).includes("Files");
}
function onDragEnter(event: DragEvent): void {
  if (!hasDraggedFiles(event)) return;
  event.preventDefault();
  dragDepth++;
  ui.dropActive = true;
}
function onDragOver(event: DragEvent): void {
  if (!hasDraggedFiles(event)) return;
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";
}
function onDragLeave(event: DragEvent): void {
  if (!dragDepth) return;
  dragDepth = Math.max(0, dragDepth - 1);
  if (!event.relatedTarget || !dragDepth) ui.dropActive = false;
}
function onDrop(event: DragEvent): void {
  if (!hasDraggedFiles(event)) return;
  event.preventDefault();
  dragDepth = 0;
  ui.dropActive = false;
  void startUpload(Array.from(event.dataTransfer?.files || []), "拖放");
}
function onPaste(event: ClipboardEvent): void {
  const list = Array.from(event.clipboardData?.files || []);
  if (!list.length) return;
  event.preventDefault();
  void startUpload(list, "粘贴");
}
onMounted(() => {
  document.addEventListener("dragenter", onDragEnter);
  document.addEventListener("dragover", onDragOver);
  document.addEventListener("dragleave", onDragLeave);
  document.addEventListener("drop", onDrop);
  document.addEventListener("paste", onPaste);
});
onUnmounted(() => {
  document.removeEventListener("dragenter", onDragEnter);
  document.removeEventListener("dragover", onDragOver);
  document.removeEventListener("dragleave", onDragLeave);
  document.removeEventListener("drop", onDrop);
  document.removeEventListener("paste", onPaste);
});

// ---- 右键菜单 ----
// 空白处右键 = 目录级操作；在某个文件/文件夹上右键 = 该条目自己的操作菜单
type CtxActionKey = "mkdir" | "upload-files" | "upload-folder" | "refresh";
const ctxMenu = ref<{ x: number; y: number } | null>(null);
const ctxOptions: Array<{ key: CtxActionKey; label: string; icon: string }> = [
  { key: "mkdir", label: "新建文件夹", icon: "plus" },
  { key: "upload-files", label: "上传文件", icon: "upload" },
  { key: "upload-folder", label: "上传文件夹", icon: "folder" },
  { key: "refresh", label: "刷新", icon: "refresh" },
];

/** 条目右键菜单：{ file, x, y } */
const rowCtxMenu = ref<{ file: WopanFile; x: number; y: number } | null>(null);

function onContextmenu(event: MouseEvent): void {
  const target = event.target as HTMLElement;
  // 输入框/文本区域里不拦截
  if (target.closest("input, textarea, [contenteditable]")) return;
  event.preventDefault();

  // 命中文件行/卡片时，弹出该条目的操作菜单（与「更多」按钮同一份菜单）
  const rowEl = target.closest("[data-file-id]") as HTMLElement | null;
  if (rowEl) {
    const id = rowEl.dataset.fileId || "";
    const file = files.files.find((f) => fileIds(f).id === id);
    if (file) {
      const menuW = 176;
      const menuH = rowActions(file).length * 36 + 10;
      rowCtxMenu.value = {
        file,
        x: Math.min(event.clientX, window.innerWidth - menuW - 8),
        y: Math.min(event.clientY, window.innerHeight - menuH - 8),
      };
      ctxMenu.value = null;
      return;
    }
  }

  rowCtxMenu.value = null;
  const menuW = 168;
  const menuH = ctxOptions.length * 38 + 8;
  ctxMenu.value = {
    x: Math.min(event.clientX, window.innerWidth - menuW - 8),
    y: Math.min(event.clientY, window.innerHeight - menuH - 8),
  };
}

function onRowCtxSelect(key: string): void {
  const file = rowCtxMenu.value?.file;
  rowCtxMenu.value = null;
  if (file) onRowAction(file, key);
}

function onCtxSelect(key: CtxActionKey): void {
  ctxMenu.value = null;
  switch (key) {
    case "mkdir":
      openMkdir();
      break;
    case "upload-files":
      pickFiles();
      break;
    case "upload-folder":
      pickFolder();
      break;
    case "refresh":
      void load();
      break;
  }
}

function onCtxClickOutside(): void {
  ctxMenu.value = null;
  rowCtxMenu.value = null;
}

onMounted(() => document.addEventListener("click", onCtxClickOutside));
onUnmounted(() => document.removeEventListener("click", onCtxClickOutside));

// ---- 行操作下拉（行内只保留一个「更多」按钮，其余操作都在菜单里） ----

type RowActionKey =
  | "open"
  | "preview"
  | "download"
  | "download-zip"
  | "share"
  | "rename"
  | "move"
  | "copy"
  | "detail"
  | "delete";

interface RowActionOption {
  key: RowActionKey;
  label: string;
  danger?: boolean;
}

function rowActions(f: WopanFile): RowActionOption[] {
  const isDir = Number(f.type) === 0;
  const actions: RowActionOption[] = [];
  if (isDir) actions.push({ key: "open", label: "打开" });
  else actions.push({ key: "preview", label: "预览" });
  if (isDir) actions.push({ key: "download-zip", label: "打包下载" });
  else actions.push({ key: "download", label: "下载" });
  actions.push({ key: "share", label: "分享…" });
  actions.push({ key: "rename", label: "重命名" });
  actions.push({ key: "move", label: "移动到…" });
  actions.push({ key: "copy", label: "复制到…" });
  actions.push({ key: "detail", label: "详情" });
  actions.push({ key: "delete", label: "删除", danger: true });
  return actions;
}

function dropdownOptions(f: WopanFile) {
  return rowActions(f).map((a) => ({
    key: a.key,
    label: a.label,
    props: a.danger ? { style: { color: "var(--gh-danger)" } } : undefined,
  }));
}

function openPreviewOf(f: WopanFile): void {
  const { fid } = fileIds(f);
  ui.previewFile = { fid, name: f.name || fileIds(f).id, kind: "" };
  ui.showPreview = true;
}

function showDetailOf(f: WopanFile): void {
  ui.detailFile = f;
  ui.showDetail = true;
}

function onRowAction(f: WopanFile, key: string): void {
  const { id, fid } = fileIds(f);
  const item: SelectedItem = { id, fid, type: Number(f.type) === 0 ? 0 : 1, name: f.name || id };
  switch (key) {
    case "open":
      openDirectory(id, f.name || id);
      break;
    case "preview":
      openPreviewOf(f);
      break;
    case "download":
      openDownload(f);
      break;
    case "download-zip":
      void downloadFolder(f);
      break;
    case "share":
      shareOne(f);
      break;
    case "rename":
      startRename(f);
      break;
    case "move":
      openFolderPicker("move", [item]);
      break;
    case "copy":
      openFolderPicker("copy", [item]);
      break;
    case "detail":
      showDetailOf(f);
      break;
    case "delete":
      deleteOne(f);
      break;
  }
}

/** 文件下载：直接新窗口打开 302 直链 */
function openDownload(f: WopanFile): void {
  window.open(downloadLink(f), "_blank", "noopener,noreferrer");
}

function onRowClick(f: WopanFile, event: MouseEvent): void {
  const target = event.target as HTMLElement;
  // 排除勾选框 / 行内操作 / 网格卡片上的勾选与更多按钮
  if (
    target.closest(".col-check") ||
    target.closest(".actions") ||
    target.closest(".card-check") ||
    target.closest(".card-more")
  )
    return;
  if (files.loading) return;
  if (Number(f.type) === 0) {
    const { id } = fileIds(f);
    openDirectory(id, f.name || id);
  } else {
    openPreviewOf(f);
  }
}

// 长按显示详情（移动端）
let longPressTimer: ReturnType<typeof setTimeout> | null = null;

function onTouchStart(f: WopanFile, event: TouchEvent): void {
  longPressTimer = setTimeout(() => {
    event.preventDefault();
    showDetailOf(f);
  }, 500);
}

function onTouchEnd(): void {
  if (longPressTimer) clearTimeout(longPressTimer);
}

// ---- 下载直链（带 auth 打开新窗口） ----
function downloadLink(f: WopanFile): string {
  const { fid } = fileIds(f);
  return (
    "/api/download?fid=" +
    encodeURIComponent(fid) +
    "&name=" +
    encodeURIComponent(f.name || fid) +
    "&auth=" +
    encodeURIComponent(getAuthToken())
  );
}

// ---- 滚动加载 ----
// 用哨兵元素 + IntersectionObserver：滚到列表底部附近就自动取下一页，
// 不再提供「上一页/下一页」按钮。桌面端和手机端行为一致。
const sentinel = ref<HTMLElement | null>(null);
let sentinelObserver: IntersectionObserver | null = null;
const SENTINEL_MARGIN = 240;

/**
 * 切换目录时列表会被清空，如果只渲染一行「正在加载…」或固定行数，
 * 卡片高度会先塌掉再撑开，看起来是一跳一跳的。
 * 这里按上一屏的实际行数渲染等量骨架行（单行高度与真实行一致），
 * 加载期间容器高度基本不变，加载完成后再平滑过渡到真实内容。
 */
const lastRowCount = ref(1);
const skeletonRows = computed(() => {
  if (!files.loading || files.files.length) return 0;
  return Math.min(Math.max(lastRowCount.value, 1), 20);
});
watch(
  () => files.files.length,
  (n) => {
    if (n > 0) lastRowCount.value = n;
  },
  { immediate: true },
);

function canLoadMore(): boolean {
  return (
    ui.view === "files" &&
    !ui.fileSearch &&
    files.hasMore &&
    !files.loading &&
    !files.loadingMore &&
    !!files.files.length
  );
}

function triggerLoadMore(): void {
  if (!canLoadMore()) return;
  void loadMore({ onError: (msg) => showToast(msg, "error") });
}

function onSentinel(entries: IntersectionObserverEntry[]): void {
  if (entries[0]?.isIntersecting) triggerLoadMore();
}

/**
 * 首屏内容不足一屏时，哨兵一直停在视口内，IntersectionObserver 不会再次触发，
 * 需要在每次加载结束后主动检查一次，否则会停在第一页。
 */
function checkSentinel(): void {
  const el = sentinel.value;
  if (!el) return;
  const top = el.getBoundingClientRect().top;
  if (top <= window.innerHeight + SENTINEL_MARGIN) triggerLoadMore();
}

function observeSentinel(): void {
  sentinelObserver?.disconnect();
  if (!sentinel.value || !sentinelObserver) return;
  sentinelObserver.observe(sentinel.value);
  checkSentinel();
}

onMounted(() => {
  sentinelObserver = new IntersectionObserver(onSentinel, { rootMargin: SENTINEL_MARGIN + "px 0px" });
  observeSentinel();
});

// 视图切换、数据变化后哨兵可能被替换成新节点，需要重新绑定
watch(
  [() => ui.fileViewMode, () => files.files.length, () => files.loading, () => files.loadingMore],
  () => {
    void nextTick(observeSentinel);
  },
);

onUnmounted(() => {
  sentinelObserver?.disconnect();
  sentinelObserver = null;
});

// ---- 批量操作 ----
const bulkCount = computed(() => selection.checkedIds.size);
const bulkBusy = ref("");

function bulkMove(): void {
  const sel = selectedItems();
  if (sel.length) openFolderPicker("move", sel);
}

async function onBulkDownload(): Promise<void> {
  bulkBusy.value = "download";
  try {
    await bulkDownload();
  } finally {
    bulkBusy.value = "";
  }
}

/** 右键菜单里各操作对应的图标 */
function ctxIconFor(key: RowActionKey): string {
  switch (key) {
    case "open":
      return "folder";
    case "preview":
      return "eye";
    case "download":
    case "download-zip":
      return "download";
    case "share":
      return "share";
    case "rename":
      return "file";
    case "move":
      return "folder";
    case "copy":
      return "files";
    case "detail":
      return "help";
    case "delete":
      return "close";
    default:
      return "file";
  }
}

function bulkDelete(): void {
  const sel = selectedItems();
  if (!sel.length) return;
  confirmDelete(
    sel.map((it) => ({ id: it.id, type: it.type, label: it.name })),
    () => load(),
  );
}

function displayName(f: WopanFile): string {
  const { id } = fileIds(f);
  return f.name || id;
}

/** 重命名输入框失焦时提交（点击其他位置视为确认） */
function onRenameBlur(f: WopanFile): void {
  void commitRename(f);
}

function onRenameKey(f: WopanFile, event: KeyboardEvent): void {
  if (event.key === "Enter") {
    event.preventDefault();
    void commitRename(f);
  } else if (event.key === "Escape") {
    event.preventDefault();
    cancelRename();
  }
}

// ---- 面包屑（与文件列表同区，避免占用顶栏） ----
function goCrumb(index: number): void {
  ui.fileSearch = "";
  jumpTo(index);
}

// ---- 视图切换（列表 / 网格） ----
function setViewMode(mode: "list" | "grid"): void {
  ui.fileViewMode = mode;
  try {
    localStorage.setItem("wopan.fileView", mode);
  } catch {
    /* 忽略持久化失败 */
  }
}

// ---- 网格卡片 ----
// 图标芯片背景色分类（与 FileIcon 配色对应）
function chipClass(f: WopanFile): string {
  const kind = iconKindForFile(f, Number(f.type) === 0);
  if (kind === "folder") return "chip-folder";
  if (kind === "image") return "chip-image";
  if (kind === "video") return "chip-video";
  if (kind === "music") return "chip-music";
  if (kind === "archive") return "chip-archive";
  return "chip-file";
}

// 卡片元信息：大小 · 时间（文件夹没有大小，只显示时间；卡片空间有限，时间只保留日期）
function cardMeta(f: WopanFile): string {
  const time = fileTime(f).slice(0, 10);
  if (Number(f.type) === 0) return time;
  return formatSize(Number(f.size || 0)) + " · " + time;
}

function reload(): void {
  void load();
}

defineExpose({ pickFiles, pickFolder, openMkdir, reload });
</script>

<template>
  <section @contextmenu="onContextmenu">
    <!-- 右键菜单 -->
    <Teleport to="body">
      <div
        v-if="ctxMenu"
        class="ctx-menu"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @contextmenu.prevent
      >
        <button
          v-for="opt in ctxOptions"
          :key="opt.key"
          type="button"
          class="ctx-item"
          :disabled="files.loading && opt.key === 'refresh'"
          @click="onCtxSelect(opt.key)"
        >
          <Icon :name="opt.icon" :size="15" />
          <span>{{ opt.label }}</span>
        </button>
      </div>

      <!-- 条目右键菜单：与该行的「更多」下拉是同一份选项 -->
      <div
        v-if="rowCtxMenu"
        class="ctx-menu"
        :style="{ left: rowCtxMenu.x + 'px', top: rowCtxMenu.y + 'px' }"
        @contextmenu.prevent
      >
        <button
          v-for="opt in rowActions(rowCtxMenu.file)"
          :key="opt.key"
          type="button"
          class="ctx-item"
          :style="opt.danger ? { color: 'var(--gh-danger)' } : undefined"
          @click="onRowCtxSelect(opt.key)"
        >
          <Icon :name="ctxIconFor(opt.key)" :size="15" />
          <span>{{ opt.label }}</span>
        </button>
      </div>
    </Teleport>
    <div v-if="bulkCount > 0" class="bulk-bar">
      <span class="muted">已选择 {{ bulkCount }} 项</span>
      <div class="bulk-actions">
        <NButton :loading="bulkBusy === 'download'" :disabled="isBusy" @click="onBulkDownload">批量下载</NButton>
        <NButton :disabled="isBusy" @click="shareBulk">批量分享</NButton>
        <NButton :disabled="isBusy" @click="bulkMove">批量移动</NButton>
        <NButton :loading="bulkBusy === 'delete'" :disabled="isBusy" type="error" secondary @click="bulkDelete">
          批量删除
        </NButton>
        <NButton @click="clearSelection">取消选择</NButton>
      </div>
    </div>

    <div class="list-meta">
      <div class="crumbs">
        <button
          type="button"
          class="crumb home"
          :class="{ current: files.stack.length <= 1 }"
          title="根目录"
          :disabled="files.stack.length <= 1 || files.loading"
          @click="goCrumb(0)"
        >
          <Icon name="files" :size="16" />
        </button>
        <template v-for="(crumb, i) in files.stack" :key="crumb.id">
          <template v-if="i > 0">
            <Icon name="chevron" :size="13" class="sep" />
            <button
              type="button"
              class="crumb"
              :class="{ current: i === files.stack.length - 1 }"
              :title="crumb.name"
              :disabled="i === files.stack.length - 1 || files.loading"
              @click="goCrumb(i)"
            >
              {{ crumb.name }}
            </button>
          </template>
        </template>
      </div>

      <div class="list-meta-right">
        <span class="muted">
          <template v-if="ui.fileSearch">{{ shownFiles.length }} 个结果（共 {{ files.files.length }} 项）</template>
          <template v-else-if="files.hasMore">{{ files.files.length }} 个项目（继续滚动加载更多）</template>
          <template v-else>{{ shownFiles.length }} 个项目</template>
        </span>
        <!-- 列表 / 网格视图切换 -->
        <div class="view-toggle" role="group" aria-label="视图切换">
          <button
            class="view-btn"
            :class="{ active: ui.fileViewMode === 'list' }"
            title="列表视图"
            @click="setViewMode('list')"
          >
            <Icon name="list" :size="16" />
          </button>
          <button
            class="view-btn"
            :class="{ active: ui.fileViewMode === 'grid' }"
            title="网格视图"
            @click="setViewMode('grid')"
          >
            <Icon name="grid" :size="16" />
          </button>
        </div>
      </div>
    </div>

    <!-- 网格视图 -->
    <div v-if="ui.fileViewMode === 'grid'" class="grid-wrap">
      <!-- 空态（加载中 / 无匹配 / 空目录） -->
      <div v-if="!shownFiles.length" class="grid-empty">
        <template v-if="files.loading">
          <LoadingSpinner :size="30" label="正在加载…" />
        </template>
        <template v-else>
          <div class="grid-empty-chip">
            <Icon name="cloud" :size="32" />
          </div>
          <p class="muted">{{ ui.fileSearch ? "没有匹配的文件" : "空目录" }}</p>
        </template>
      </div>
      <div
        v-for="f in shownFiles"
        :key="(fileIds(f).id || '') + displayName(f)"
        class="file-card"
        :class="{ selected: selection.checkedIds.has(fileIds(f).id) }"
        :data-file-id="fileIds(f).id"
        @click="onRowClick(f, $event)"
        @touchstart="onTouchStart(f, $event)"
        @touchend="onTouchEnd"
        @touchmove="onTouchEnd"
        @touchcancel="onTouchEnd"
      >
        <NCheckbox
          class="card-check"
          :checked="selection.checkedIds.has(fileIds(f).id)"
          @update:checked="(v: boolean) => toggleSelect(fileIds(f).id, v)"
          @click.stop
        />
        <NDropdown
          trigger="click"
          placement="bottom-end"
          :options="dropdownOptions(f)"
          @select="(key: string) => onRowAction(f, key)"
        >
          <button class="card-more" title="更多操作" aria-label="更多操作" @click.stop>
            <Icon name="dots" :size="16" />
          </button>
        </NDropdown>
        <div class="file-chip" :class="chipClass(f)">
          <FileIcon :kind="iconKindForFile(f, Number(f.type) === 0)" />
        </div>
        <div v-if="renamingId === fileIds(f).id" class="file-card-name editing" @click.stop>
          <input
            :ref="setRenameRef"
            v-model="renameDraft"
            class="rename-input"
            :disabled="renameBusy"
            @keydown="onRenameKey(f, $event)"
            @blur="onRenameBlur(f)"
            @click.stop
          />
        </div>
        <div v-else class="file-card-name" :title="displayName(f)">{{ displayName(f) }}</div>
        <div class="file-card-meta">{{ cardMeta(f) }}</div>
      </div>
      <!-- 网格视图下刷新当前目录时同样用遮罩提示 -->
      <div v-if="files.loading && shownFiles.length" class="grid-reload-mask">
        <LoadingSpinner :size="26" label="正在加载…" />
      </div>
    </div>

    <!-- 列表视图 -->
    <div v-else class="card-surface">
      <!-- 刷新当前目录时列表可能仍有旧内容，用一层遮罩给出等待反馈 -->
      <div v-if="files.loading && shownFiles.length" class="reload-mask">
        <LoadingSpinner :size="26" label="正在加载…" />
      </div>
      <table>
        <thead>
          <tr>
            <th class="col-check">
              <NCheckbox :checked="allShownSelected" @update:checked="toggleAllShown" />
            </th>
            <th class="col-name">名称</th>
            <th class="col-hide-mobile">大小</th>
            <th class="col-hide-mobile">时间</th>
            <th class="col-ops">操作</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="!shownFiles.length && files.loading">
            <tr v-for="i in skeletonRows" :key="'sk' + i" class="skeleton-row">
              <td :colspan="emptyColspan"><span class="skeleton-bar" /></td>
            </tr>
          </template>
          <tr v-else-if="!shownFiles.length">
            <td :colspan="emptyColspan" class="muted empty-cell">
              {{ ui.fileSearch ? "没有匹配的文件" : "空目录" }}
            </td>
          </tr>
          <tr
            v-for="f in shownFiles"
            :key="(fileIds(f).id || '') + displayName(f)"
            class="row-clickable"
            :data-file-id="fileIds(f).id"
            @click="onRowClick(f, $event)"
          >
            <td class="col-check">
              <NCheckbox
                :checked="selection.checkedIds.has(fileIds(f).id)"
                @update:checked="(v: boolean) => toggleSelect(fileIds(f).id, v)"
                @click.stop
              />
            </td>
            <td class="col-name">
              <span class="name">
                <FileIcon :kind="iconKindForFile(f, Number(f.type) === 0)" />
                <input
                  v-if="renamingId === fileIds(f).id"
                  :ref="setRenameRef"
                  v-model="renameDraft"
                  class="rename-input"
                  :disabled="renameBusy"
                  @keydown="onRenameKey(f, $event)"
                  @blur="onRenameBlur(f)"
                  @click.stop
                />
                <span
                  v-else
                  class="file-name"
                  :title="displayName(f)"
                  @touchstart="onTouchStart(f, $event)"
                  @touchend="onTouchEnd"
                  @touchmove="onTouchEnd"
                  @touchcancel="onTouchEnd"
                >
                  {{ displayName(f) }}
                </span>
              </span>
            </td>
            <td class="col-hide-mobile">{{ Number(f.type) === 0 ? "-" : formatSize(Number(f.size || 0)) }}</td>
            <td class="col-hide-mobile">{{ fileTime(f) }}</td>
            <td class="col-ops">
              <div class="actions">
                <!-- 行内只保留一个「更多」按钮，其余操作都收进菜单 -->
                <NDropdown
                  trigger="click"
                  placement="bottom-end"
                  :options="dropdownOptions(f)"
                  @select="(key: string) => onRowAction(f, key)"
                >
                  <button type="button" class="op-btn" title="更多操作" aria-label="更多操作" @click.stop>
                    <span v-if="renamingId === fileIds(f).id" class="op-loading"><LoadingSpinner :size="14" /></span>
                    <Icon v-else name="dots" :size="16" />
                  </button>
                </NDropdown>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 滚动加载哨兵：进入视口即追加下一页 -->
    <div ref="sentinel" class="sentinel" aria-hidden="true">
      <LoadingSpinner v-if="files.loadingMore" :size="18" label="正在加载更多…" />
      <!-- 只有翻过页才提示到底，普通小目录不显示多余文字 -->
      <span
        v-else-if="files.page > 0 && !files.hasMore && !ui.fileSearch"
        class="sentinel-text end"
      >已经到底了</span>
    </div>

    <input ref="fileInput" type="file" multiple hidden @change="onFilesChosen" />
    <input ref="folderInput" type="file" webkitdirectory multiple hidden @change="onFolderChosen" />

    <!-- 新建文件夹 -->
    <NModal
      v-model:show="mkdirVisible"
      preset="dialog"
      title="新建文件夹"
      positive-text="创建"
      negative-text="取消"
      :positive-button-props="{ loading: mkdirBusy }"
      :close-on-esc="!mkdirBusy"
      :mask-closable="!mkdirBusy"
      @positive-click="confirmMkdir"
    >
      <NInput v-model:value="mkdirNameInput" placeholder="请输入文件夹名称" @keydown.enter.prevent="confirmMkdir" />
    </NModal>
  </section>
</template>

<style scoped>
/* ---- 右键菜单 ---- */
.ctx-menu {
  position: fixed;
  z-index: 400;
  min-width: 168px;
  padding: 5px;
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 12px;
  box-shadow: var(--gh-shadow-lg);
}
.ctx-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--gh-ink);
  font-size: 13px;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
}
.ctx-item:hover:not(:disabled) {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
.ctx-item:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.bulk-bar {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
  padding: 10px 12px;
  background: var(--gh-primary-soft);
  border: 1px solid var(--gh-border-hover);
  border-radius: 12px;
  flex-wrap: wrap;
}
.bulk-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-left: auto;
  flex-wrap: wrap;
}
.list-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 0 2px 10px;
  min-height: 34px;
}
.list-meta-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 0 0 auto;
}
/* ---- 面包屑 ---- */
.crumbs {
  display: flex;
  align-items: center;
  gap: 3px;
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
}
.crumb {
  border: 0;
  background: none;
  padding: 4px 6px;
  border-radius: 6px;
  color: var(--gh-ink-soft);
  font-size: 13px;
  font-family: inherit;
  cursor: pointer;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 0 0 auto;
}
.crumb:hover:not(:disabled) {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
.crumb.current {
  color: var(--gh-ink);
  font-weight: 650;
  cursor: default;
  flex: 0 1 auto;
  min-width: 0;
}
.crumb.home {
  display: grid;
  place-items: center;
  padding: 5px;
}
.sep {
  color: var(--gh-ink-faint);
  flex: 0 0 auto;
}
/* ---- 列表 / 网格视图切换 ---- */
.view-toggle {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  background: var(--gh-surface-2);
  border: 1px solid var(--gh-border);
  border-radius: 10px;
}
.view-btn {
  width: 28px;
  height: 26px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--gh-ink-soft);
  cursor: pointer;
  transition: background 0.15s ease-out, color 0.15s ease-out;
}
.view-btn:hover {
  background: var(--gh-surface);
  color: var(--gh-ink);
}
.view-btn.active {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
/* ---- 网格视图 ---- */
.grid-wrap {
  position: relative;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
  gap: 14px;
}
.grid-reload-mask {
  position: absolute;
  inset: -6px;
  z-index: 3;
  display: grid;
  place-items: center;
  background: rgba(243, 244, 253, 0.72);
  border-radius: 18px;
  backdrop-filter: blur(1px);
}
.file-card {
  position: relative;
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 16px;
  padding: 14px;
  cursor: pointer;
  transition: transform 0.15s ease-out, box-shadow 0.15s ease-out, border-color 0.15s ease-out,
    background 0.15s ease-out;
}
.file-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--gh-shadow-md);
  border-color: var(--gh-border-hover);
}
.file-card.selected {
  border-color: var(--gh-primary);
  background: var(--gh-selected-bg);
}
.file-chip {
  width: 46px;
  height: 46px;
  border-radius: 14px;
  display: grid;
  place-items: center;
}
.chip-folder {
  background: var(--ac-orange-soft);
}
.chip-image {
  background: var(--ac-blue-soft);
}
.chip-video {
  background: var(--ac-red-soft);
}
.chip-music {
  background: var(--ac-violet-soft);
}
.chip-archive {
  background: var(--ac-green-soft);
}
.chip-file {
  background: var(--gh-primary-soft);
}
.file-card-name {
  margin-top: 10px;
  font-size: 13px;
  font-weight: 600;
  color: var(--gh-ink);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
  min-height: 36px;
}
.file-card-meta {
  margin-top: 4px;
  font-size: 11.5px;
  color: var(--gh-ink-soft);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 卡片左上角勾选框：默认隐藏，悬停或选中时显示 */
.file-card .card-check {
  position: absolute;
  left: 8px;
  top: 8px;
  z-index: 2;
  opacity: 0;
  transition: opacity 0.15s ease-out;
}
.file-card:hover .card-check,
.file-card.selected .card-check {
  opacity: 1;
}
/* 卡片右上角更多按钮：默认隐藏，悬停时显示 */
.file-card .card-more {
  position: absolute;
  right: 6px;
  top: 6px;
  z-index: 2;
  width: 30px;
  height: 30px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--gh-ink-soft);
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s ease-out, background 0.15s ease-out, color 0.15s ease-out;
}
.file-card:hover .card-more {
  opacity: 1;
}
.file-card .card-more:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
/* 网格空态 */
.grid-empty {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 56px 0;
}
.grid-empty-chip {
  width: 64px;
  height: 64px;
  border-radius: 20px;
  background: var(--gh-primary-soft);
  display: grid;
  place-items: center;
  color: var(--gh-primary);
}
.card-surface {
  position: relative;
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 16px;
  box-shadow: var(--gh-shadow-sm);
  overflow: hidden;
}
/* 刷新遮罩：盖住旧内容并淡出，避免目录内容「跳变」 */
.reload-mask {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: grid;
  place-items: center;
  background: var(--gh-overlay-mask);
  backdrop-filter: blur(1px);
}
.empty-cell .loading-spinner {
  justify-content: center;
}
table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  table-layout: fixed;
}
thead th {
  background: var(--gh-surface-2);
  color: var(--gh-ink-soft);
  font-weight: 600;
  font-size: 12px;
  letter-spacing: 0.02em;
  padding: 11px 16px;
  border-bottom: 1px solid var(--gh-border);
  text-align: left;
}
thead th.col-hide-mobile,
thead th.col-ops {
  text-align: center;
}
tbody td {
  text-align: center;
  padding: 0 16px;
  /* 固定行高：时间列换行会让行高忽高忽低，这里统一为单行 48px */
  height: 48px;
  border-bottom: 1px solid var(--gh-row-border);
  font-size: 13.5px;
  white-space: nowrap;
}
tbody td.col-name {
  text-align: left;
}
tbody tr:hover td {
  background: var(--gh-primary-soft);
}
tbody tr:last-child td {
  border-bottom: 0;
}
.empty-cell {
  padding: 48px 0 !important;
}
/* 加载骨架：与真实行同高，避免列表先塌再撑开 */
.skeleton-row td {
  height: 48px;
}
.skeleton-bar {
  display: block;
  height: 10px;
  width: 42%;
  margin: 0 auto;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--gh-skeleton-a) 25%, var(--gh-skeleton-b) 37%, var(--gh-skeleton-a) 63%);
  background-size: 400% 100%;
  animation: skeleton-shimmer 1.4s ease infinite;
}
@keyframes skeleton-shimmer {
  0% {
    background-position: 100% 50%;
  }
  100% {
    background-position: 0 50%;
  }
}
.row-clickable {
  cursor: pointer;
}
.col-name {
  width: auto;
}
.col-check {
  width: 42px;
}
.col-hide-mobile {
  width: 108px;
}
.col-ops {
  width: 76px;
}
.name {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  max-width: 100%;
}
.file-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
/* 行内重命名输入框：贴合文件名位置，和文本行同高 */
.rename-input {
  width: 100%;
  min-width: 0;
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--gh-primary);
  border-radius: 7px;
  background: var(--gh-surface);
  color: var(--gh-ink);
  font-size: 13.5px;
  font-family: inherit;
  outline: none;
  box-shadow: 0 0 0 3px rgba(109, 123, 245, 0.16);
}
.file-card-name.editing {
  -webkit-line-clamp: unset;
  display: block;
  min-height: 36px;
}
.op-loading {
  display: grid;
  place-items: center;
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 12px;
}
.actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}
/* 行内操作统一为图标按钮：只保留 title 作为提示，避免文字挤压文件名列 */
.op-btn,
.op-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 1px solid var(--gh-border);
  border-radius: 9px;
  background: var(--gh-surface);
  color: var(--gh-ink-soft);
  cursor: pointer;
  text-decoration: none;
  transition: background 0.15s ease-out, color 0.15s ease-out, border-color 0.15s ease-out;
}
.op-btn:hover,
.op-link:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  border-color: var(--gh-border-hover);
}
.sentinel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 18px 0 8px;
  min-height: 44px;
}
.sentinel-text {
  font-size: 12px;
  color: var(--gh-ink-soft);
}
.sentinel-text.end {
  color: var(--gh-ink-faint);
}
@media (max-width: 760px) {
  .col-hide-mobile {
    display: none;
  }
  .col-ops {
    width: 54px;
  }
  .col-check {
    width: 38px;
  }
  thead th,
  tbody td {
    padding: 0 10px;
  }
  tbody td {
    height: 46px;
  }
  .card-surface {
    border-radius: 10px;
  }
  .grid-wrap {
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 10px;
  }
  .file-card {
    padding: 12px;
  }
  /* 面包屑在窄屏可横向滑动，避免挤压右侧统计与视图切换 */
  .crumbs {
    overflow-x: auto;
    scrollbar-width: none;
  }
  .crumbs::-webkit-scrollbar {
    display: none;
  }
  .crumb {
    max-width: 40vw;
  }
  .list-meta {
    gap: 8px;
  }
  .list-meta-right > .muted {
    display: none;
  }
}
</style>
