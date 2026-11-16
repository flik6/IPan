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
} from "../stores/files";
import { uploadFiles } from "../stores/upload";
import { showToast } from "../stores/toast";
import { formatSize, fileTime, iconKindForFile } from "../utils/format";
import { ui, openFolderPicker, openShareDialog } from "../stores/ui";
import FileIcon from "./FileIcon.vue";
import Icon from "./Icon.vue";
import LoadingSpinner from "./LoadingSpinner.vue";
import WorkspaceEmpty from "./WorkspaceEmpty.vue";

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
const toolbarOptions = ctxOptions
  .filter(({ key }) => key === "upload-folder" || key === "refresh")
  .map(({ key, label }) => ({ key, label }));

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

function fileKind(file: WopanFile) {
  return iconKindForFile(file, Number(file.type) === 0);
}

function cardMeta(file: WopanFile): string {
  const size = Number(file.type) === 0 ? "文件夹" : formatSize(Number(file.size || 0));
  const date = fileTime(file).slice(0, 10);
  return date === "-" ? size : size + " · " + date;
}

function filePlateLabel(file: WopanFile): string {
  if (Number(file.type) === 0) return "FOLDER";
  const name = displayName(file);
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot + 1).toUpperCase().slice(0, 5) : "FILE";
}

</script>

<template>
  <section class="file-workspace" aria-label="文件工作台" @contextmenu="onContextmenu">
    <!-- 右键菜单 -->
    <Teleport to="body">
      <div
        v-if="ctxMenu"
        class="ctx-menu glass-panel"
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
        class="ctx-menu glass-panel"
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

    <header class="page-header file-page-head">
      <div class="page-header-copy">
        <h1 class="display">{{ files.stack.length > 1 ? currentDirectoryName() : "全部文件" }}</h1>
        <p class="file-page-meta">
          <span class="mono">{{ shownFiles.length }}{{ files.hasMore ? "+" : "" }}</span>
          <span>个项目</span>
          <span class="meta-dot" aria-hidden="true">·</span>
          <span>{{ ui.fileSearch.trim() ? "当前筛选结果" : "文件夹优先 · 最近修改在前" }}</span>
        </p>
      </div>
      <div class="page-header-actions">
        <button type="button" class="btn" @click="openMkdir">
          <Icon name="plus" :size="15" />
          新建文件夹
        </button>
        <button type="button" class="btn btn-primary" @click="pickFiles">
          <Icon name="upload" :size="15" />
          上传文件
        </button>
        <NDropdown trigger="click" placement="bottom-end" :options="toolbarOptions" @select="onCtxSelect">
          <button type="button" class="icon-control page-more" title="更多目录操作" aria-label="更多目录操作" aria-haspopup="menu">
            <Icon name="dots" :size="18" />
          </button>
        </NDropdown>
      </div>
    </header>

    <div class="workspace-panel surface-panel" :aria-busy="files.loading">
      <div class="panel-bar">
        <nav class="crumbs" aria-label="目录路径">
          <button
            type="button"
            class="crumb home"
            :class="{ current: files.stack.length <= 1 }"
            title="全部文件"
            :disabled="files.stack.length <= 1 || files.loading"
            @click="goCrumb(0)"
          >
            <Icon name="folder" :size="16" /> <span>全部文件</span>
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
          <span v-if="!files.loading" class="crumb-count" role="status">
            {{ ui.fileSearch.trim() ? shownFiles.length + " 个匹配项" : files.files.length + (files.hasMore ? "+ 项" : " 项") }}
          </span>
        </nav>

        <div class="panel-tools">
          <NInput
            v-model:value="ui.fileSearch"
            class="directory-search"
            :input-props="{ 'aria-label': '搜索当前目录' }"
            clearable
            placeholder="搜索目录"
          >
            <template #prefix><Icon name="search" :size="17" /></template>
          </NInput>
          <div class="view-toggle" role="group" aria-label="视图切换">
            <button
              type="button"
              class="view-btn icon-control"
              :class="{ active: ui.fileViewMode === 'list' }"
              title="列表视图"
              aria-label="列表视图"
              :aria-pressed="ui.fileViewMode === 'list'"
              @click="setViewMode('list')"
            >
              <Icon name="list" :size="16" />
            </button>
            <button
              type="button"
              class="view-btn icon-control"
              :class="{ active: ui.fileViewMode === 'grid' }"
              title="网格视图"
              aria-label="网格视图"
              :aria-pressed="ui.fileViewMode === 'grid'"
              @click="setViewMode('grid')"
            >
              <Icon name="grid" :size="16" />
            </button>
          </div>
          <button type="button" class="icon-control panel-tool" title="新建文件夹" aria-label="新建文件夹" @click="openMkdir">
            <Icon name="plus" :size="17" />
          </button>
          <button type="button" class="panel-cta" @click="pickFiles">
            <Icon name="upload" :size="16" />上传
          </button>
          <NDropdown trigger="click" placement="bottom-end" :options="toolbarOptions" @select="onCtxSelect">
            <button type="button" class="toolbar-more icon-control" title="目录操作" aria-label="目录操作" aria-haspopup="menu">
              <Icon name="dots" :size="17" />
            </button>
          </NDropdown>
        </div>
      </div>

      <div v-if="bulkCount > 0" class="workspace-bulk panel-bulk">
        <span class="workspace-bulk-count" role="status"><Icon name="check" :size="16" /> 已选 {{ bulkCount }} 项</span>
        <div class="workspace-bulk-actions">
          <NButton :loading="bulkBusy === 'download'" :disabled="isBusy" @click="onBulkDownload">批量下载</NButton>
          <NButton :disabled="isBusy" @click="shareBulk">批量分享</NButton>
          <NButton :disabled="isBusy" @click="bulkMove">批量移动</NButton>
          <NButton :loading="bulkBusy === 'delete'" :disabled="isBusy" type="error" secondary @click="bulkDelete">
            批量删除
          </NButton>
          <NButton quaternary @click="clearSelection">取消选择</NButton>
        </div>
      </div>

      <!-- 空态 -->
      <WorkspaceEmpty
        v-if="!shownFiles.length && !files.loading"
        :icon="ui.fileSearch.trim() ? 'search' : 'folder'"
        :title="ui.fileSearch.trim() ? '没有匹配的文件' : '这个目录还没有文件'"
        :description="ui.fileSearch.trim() ? '换个关键词，或清除搜索后查看全部文件。' : '可以选择文件上传，也可以把文件拖到这里。'"
      >
        <button v-if="ui.fileSearch.trim()" type="button" class="glass-control" @click="ui.fileSearch = ''">清除搜索</button>
        <button v-else type="button" class="glass-control" @click="pickFiles"><Icon name="upload" :size="16" />选择文件</button>
      </WorkspaceEmpty>

      <!-- 网格视图 -->
      <div v-else-if="ui.fileViewMode === 'grid'" class="panel-body">
        <div v-if="!shownFiles.length" class="grid-loading">
          <LoadingSpinner :size="30" label="正在加载…" />
        </div>
        <div v-else class="grid-wrap">
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
              :aria-label="'选择 ' + displayName(f)"
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
              <button type="button" class="card-more icon-control" title="更多操作" :aria-label="'更多操作，' + displayName(f)" @click.stop>
                <Icon name="dots" :size="16" />
              </button>
            </NDropdown>
            <div class="file-cover file-glyph" :class="'chip-' + fileKind(f)">
              <FileIcon :kind="fileKind(f)" />
              <span class="plate-label">{{ filePlateLabel(f) }}</span>
            </div>
            <div class="file-card-copy">
              <div v-if="renamingId === fileIds(f).id" class="file-card-name editing" @click.stop>
                <input
                  :ref="setRenameRef"
                  v-model="renameDraft"
                  aria-label="文件名称"
                  class="rename-input"
                  :disabled="renameBusy"
                  @keydown="onRenameKey(f, $event)"
                  @blur="onRenameBlur(f)"
                  @click.stop
                />
              </div>
              <button v-else type="button" class="file-card-name file-open" :title="displayName(f)" @click.stop="onRowClick(f, $event)">
                {{ displayName(f) }}
              </button>
              <div class="file-card-meta">{{ cardMeta(f) }}</div>
            </div>
          </div>
        </div>
        <!-- 网格视图下刷新当前目录时同样用遮罩提示 -->
        <div v-if="files.loading && shownFiles.length" class="grid-reload-mask">
          <LoadingSpinner :size="26" label="正在加载…" />
        </div>
      </div>

      <!-- 列表视图 -->
      <div v-else class="panel-body list-body">
        <!-- 刷新当前目录时列表可能仍有旧内容，用一层遮罩给出等待反馈 -->
        <div v-if="files.loading && shownFiles.length" class="reload-mask">
          <LoadingSpinner :size="26" label="正在加载…" />
        </div>
        <table>
          <thead>
            <tr>
              <th class="col-check">
                <NCheckbox aria-label="选择当前列表全部文件" :checked="allShownSelected" @update:checked="toggleAllShown" />
              </th>
              <th class="col-name">名称</th>
              <th class="col-hide-mobile">大小</th>
              <th class="col-hide-mobile col-time">修改时间</th>
              <th class="col-ops">操作</th>
            </tr>
          </thead>
          <tbody>
            <template v-if="!shownFiles.length && files.loading">
              <tr v-for="i in skeletonRows" :key="'sk' + i" class="skeleton-row">
                <td :colspan="emptyColspan"><span class="skeleton-bar" /></td>
              </tr>
            </template>
            <tr
              v-for="f in shownFiles"
              :key="(fileIds(f).id || '') + displayName(f)"
              class="row-clickable"
              :class="{ selected: selection.checkedIds.has(fileIds(f).id) }"
              :data-file-id="fileIds(f).id"
              @click="onRowClick(f, $event)"
            >
              <td class="col-check">
                <NCheckbox
                  :aria-label="'选择 ' + displayName(f)"
                  :checked="selection.checkedIds.has(fileIds(f).id)"
                  @update:checked="(v: boolean) => toggleSelect(fileIds(f).id, v)"
                  @click.stop
                />
              </td>
              <td class="col-name">
                <div class="name">
                  <span class="row-glyph file-glyph" :class="'chip-' + fileKind(f)"><FileIcon :kind="fileKind(f)" /></span>
                  <div class="name-copy">
                    <input
                      v-if="renamingId === fileIds(f).id"
                      :ref="setRenameRef"
                      v-model="renameDraft"
                      aria-label="文件名称"
                      class="rename-input"
                      :disabled="renameBusy"
                      @keydown="onRenameKey(f, $event)"
                      @blur="onRenameBlur(f)"
                      @click.stop
                    />
                    <button
                      v-else
                      type="button"
                      class="file-name file-open"
                      :title="displayName(f)"
                      @click.stop="onRowClick(f, $event)"
                      @touchstart="onTouchStart(f, $event)"
                      @touchend="onTouchEnd"
                      @touchmove="onTouchEnd"
                      @touchcancel="onTouchEnd"
                    >
                      {{ displayName(f) }}
                    </button>
                    <small class="file-meta-mobile">{{ cardMeta(f) }}</small>
                  </div>
                </div>
              </td>
              <td class="col-hide-mobile">{{ Number(f.type) === 0 ? "-" : formatSize(Number(f.size || 0)) }}</td>
              <td class="col-hide-mobile col-time">{{ fileTime(f) }}</td>
              <td class="col-ops">
                <div class="actions">
                  <!-- 行内只保留一个「更多」按钮，其余操作都收进菜单 -->
                  <NDropdown
                    trigger="click"
                    placement="bottom-end"
                    :options="dropdownOptions(f)"
                    @select="(key: string) => onRowAction(f, key)"
                  >
                    <button type="button" class="op-btn icon-control" title="更多操作" :aria-label="'更多操作，' + displayName(f)" @click.stop>
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
    </div>

    <!-- 滚动加载哨兵：进入视口即追加下一页 -->
    <div ref="sentinel" class="sentinel" aria-hidden="true">
      <LoadingSpinner v-if="files.loadingMore" :size="18" label="正在加载更多…" />
      <!-- 只有翻过页才提示到底，普通小目录不显示多余文字 -->
      <span
        v-else-if="files.page > 0 && !files.hasMore && !ui.fileSearch"
        class="sentinel-text"
      >已经到底了</span>
    </div>

    <input ref="fileInput" type="file" multiple hidden @change="onFilesChosen" />
    <input ref="folderInput" type="file" webkitdirectory multiple hidden @change="onFolderChosen" />

    <!-- 新建文件夹 -->
    <NModal
      v-model:show="mkdirVisible"
      preset="dialog"
      title="新建文件夹"
      aria-label="新建文件夹"
      positive-text="创建"
      negative-text="取消"
      :positive-button-props="{ loading: mkdirBusy }"
      :close-on-esc="!mkdirBusy"
      :mask-closable="!mkdirBusy"
      @positive-click="confirmMkdir"
    >
      <label for="new-folder-name" class="mkdir-label">文件夹名称</label>
      <NInput
        v-model:value="mkdirNameInput"
        :input-props="{ id: 'new-folder-name' }"
        placeholder="例如：项目资料"
        @keydown.enter.prevent="confirmMkdir"
      />
    </NModal>
  </section>
</template>

<style scoped>
/* 工作台是单一容器：路径栏、工具与列表同处一张面板，避免多块浮层割裂界面 */
.workspace-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}
.panel-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 9px 8px 12px;
  border-bottom: 1px solid var(--gh-row-border);
}
.panel-tools { display: flex; align-items: center; gap: 6px; min-width: 0; flex: 0 1 auto; }
.panel-tool { width: 40px; height: 40px; border: 1px solid var(--gh-border); background: var(--gh-surface); }
.panel-cta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 40px;
  padding: 0 14px;
  border: 1px solid transparent;
  border-radius: var(--gh-radius-pill);
  color: var(--gh-on-action);
  background: var(--gh-action);
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
  cursor: pointer;
  transition: filter 160ms ease, box-shadow 160ms ease;
}
.panel-cta:hover { filter: brightness(1.08); box-shadow: var(--gh-shadow-md); }
.panel-bulk { margin: 10px 9px 0; }
.panel-body { position: relative; }
.panel-body.list-body { padding: 6px; }
.directory-search {
  width: clamp(172px, 23vw, 270px);
  min-width: 0;
  --n-height: 40px !important;
  --n-border-radius: 13px !important;
  --n-border: 1px solid var(--gh-border) !important;
}
.directory-search :deep(.n-input__prefix) { margin-right: 7px; color: var(--gh-primary); }
.crumbs {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  flex: 1 1 auto;
  overflow-x: auto;
  scrollbar-width: thin;
}
.crumb-count {
  flex: 0 0 auto;
  margin-left: 4px;
  padding: 3px 9px;
  border-radius: 999px;
  color: var(--gh-primary);
  background: var(--gh-primary-soft);
  font-size: 12px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
.crumb {
  flex: 0 0 auto;
  max-width: 220px;
  min-height: 36px;
  padding: 6px 9px;
  overflow: hidden;
  border: 0;
  border-radius: 10px;
  color: var(--gh-ink-soft);
  background: transparent;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}
.crumb:focus-visible { outline-offset: -2px; }
.crumb:hover:not(:disabled) { color: var(--gh-primary); background: var(--gh-primary-soft); }
.crumb.current { color: var(--gh-ink); background: var(--gh-surface-2); font-weight: 700; cursor: default; }
.crumb.home { display: inline-flex; align-items: center; gap: 6px; }
.crumb.home svg { flex: none; color: var(--gh-primary); }
.sep { flex: none; color: var(--gh-ink-faint); }
.view-toggle {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--gh-border);
  border-radius: 14px;
  background: var(--gh-surface-2);
}
.view-btn { width: 40px; height: 36px; border-radius: 11px; }
.view-btn.active { color: var(--gh-primary); background: var(--gh-lens); box-shadow: var(--gh-shadow-sm); }
.toolbar-more { border-color: var(--gh-border); background: var(--gh-surface); }

.ctx-menu {
  position: fixed;
  z-index: 400;
  min-width: 168px;
  padding: 5px;
  border-radius: 15px;
  box-shadow: var(--gh-shadow-lg);
}
.ctx-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 9px;
  color: var(--gh-ink);
  background: transparent;
  font: inherit;
  font-size: 13px;
  text-align: start;
  cursor: pointer;
}
.ctx-item:hover:not(:disabled) { color: var(--gh-primary); background: var(--gh-primary-soft); }
.ctx-item:disabled { opacity: 0.5; cursor: not-allowed; }

.grid-wrap { position: relative; display: grid; grid-template-columns: repeat(auto-fill, minmax(173px, 1fr)); gap: 10px; padding: 10px; }
.file-card {
  position: relative;
  min-width: 0;
  padding: 8px;
  border: 1px solid var(--gh-row-border);
  border-radius: 15px;
  background: var(--gh-surface-2);
  cursor: pointer;
  transition: border-color 160ms ease, background 160ms ease;
}
.file-card:hover { border-color: var(--gh-border-hover); background: var(--gh-surface); }
.file-card.selected { border-color: var(--gh-primary); background: var(--gh-selected-bg); }
.file-cover {
  display: flex;
  align-items: end;
  min-height: 81px;
  padding: 12px;
  border-radius: 13px;
  background: linear-gradient(140deg, var(--gh-surface-2), var(--file-tint));
}
.file-glyph {
  --file-tint: var(--gh-primary-soft);
  --file-edge: var(--gh-glint);
}
.chip-folder { --file-tint: var(--ac-blue-soft); }
.chip-image { --file-tint: var(--ac-violet-soft); }
.chip-video { --file-tint: var(--ac-red-soft); }
.chip-music { --file-tint: var(--ac-pink-soft); }
.chip-archive { --file-tint: var(--ac-green-soft); }
.file-cover .file-icon { width: 36px; height: 36px; flex-basis: 36px; }
.card-check { position: absolute; top: 20px; right: 56px; opacity: 0; }
.file-card:hover .card-check, .file-card:focus-within .card-check, .file-card.selected .card-check { opacity: 1; }
.card-more { position: absolute; top: 9px; right: 9px; }
.file-card-copy { padding: 9px 6px 6px; }
.file-card-name.file-open {
  display: block;
  width: 100%;
  min-height: 32px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-card-name.editing { display: flex; align-items: center; min-height: 32px; }
.file-card-meta, .file-meta-mobile {
  display: block;
  overflow: hidden;
  color: var(--gh-ink-soft);
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-meta-mobile { display: none; }

.reload-mask, .grid-reload-mask {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: grid;
  place-items: center;
  border-radius: 0 0 var(--gh-radius-card) var(--gh-radius-card);
  background: var(--gh-overlay-mask);
}
.grid-loading { display: grid; justify-items: center; padding: 44px 12px; }
table { width: 100%; border-collapse: separate; border-spacing: 0; table-layout: fixed; }
thead th {
  height: 41px;
  padding: 0 12px;
  color: var(--gh-ink-soft);
  background: var(--gh-surface-2);
  font-size: 12px;
  font-weight: 650;
  text-align: start;
}
thead th:first-child { border-radius: 11px 0 0 11px; }
thead th:last-child { border-radius: 0 11px 11px 0; }
.col-check { width: 42px; text-align: center; }
.col-hide-mobile { width: 106px; text-align: end; }
.col-time { width: 175px; }
.col-ops { width: 64px; text-align: end; }
tbody td {
  height: 57px;
  padding: 5px 12px;
  border-bottom: 1px solid var(--gh-row-border);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  vertical-align: middle;
}
tbody tr:last-child td { border-bottom: 0; }
tbody .col-hide-mobile { color: var(--gh-ink-soft); white-space: nowrap; }
.row-clickable { cursor: pointer; }
.row-clickable:hover td, .row-clickable:focus-within td { background: var(--gh-surface-2); }
.row-clickable.selected td { background: var(--gh-selected-bg); }
.row-clickable.selected td:first-child { box-shadow: inset 3px 0 var(--gh-primary); }
.name { display: flex; align-items: center; gap: 11px; min-width: 0; }
.name-copy { min-width: 0; flex: 1; }
.row-glyph {
  display: grid;
  place-items: center;
  flex: 0 0 36px;
  height: 36px;
  border: 1px solid var(--file-edge);
  border-radius: 11px;
  background: var(--file-tint);
}
.row-glyph .file-icon { width: 24px; height: 24px; flex-basis: 24px; }
.file-name { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-open { padding: 3px 0; border: 0; color: var(--gh-ink); background: transparent; font: inherit; font-weight: 650; text-align: start; cursor: pointer; }
.file-open:hover { color: var(--gh-primary); }
.actions { display: flex; justify-content: flex-end; }
.op-btn { width: 40px; height: 40px; }
.op-loading { display: grid; place-items: center; }

.skeleton-row td { height: 53px; }
.skeleton-bar {
  display: block;
  width: min(48%, 310px);
  height: 10px;
  margin-left: 44px;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--gh-skeleton-a) 25%, var(--gh-skeleton-b) 37%, var(--gh-skeleton-a) 63%);
  background-size: 400% 100%;
  animation: skeleton-shimmer 1.4s ease infinite;
}
@keyframes skeleton-shimmer {
  from { background-position: 100% 50%; }
  to { background-position: 0 50%; }
}
.mkdir-label { display: block; margin-bottom: 6px; font-weight: 650; }
.rename-input {
  width: 100%;
  min-width: 0;
  height: 32px;
  padding: 0 7px;
  border: 1px solid var(--gh-primary);
  border-radius: 8px;
  color: var(--gh-ink);
  background: var(--gh-surface);
  font: inherit;
}
.sentinel { display: flex; align-items: center; justify-content: center; min-height: 36px; padding: 14px 0 6px; }
.sentinel-text { color: var(--gh-ink-faint); font-size: 12px; }

@media (pointer: coarse) {
  .card-check { opacity: 1; }
  .op-btn, .view-btn, .panel-tool { width: 44px; height: 44px; }
  .crumb { min-height: 44px; }
}
@media (max-width: 760px) {
  .panel-bar { flex-wrap: wrap; gap: 8px; padding: 8px 8px 6px; }
  .panel-tools { width: 100%; flex-wrap: wrap; }
  .directory-search { width: 100%; flex: 1 0 100%; order: -1; }
  .crumbs { flex-basis: 100%; }
  .col-hide-mobile { display: none; }
  .col-check { width: 34px; }
  .col-ops { width: 48px; }
  .file-meta-mobile { display: block; }
  .panel-body.list-body { padding: 4px; }
  thead th, tbody td { padding-inline: 6px; }
  th.col-ops, td.col-ops { padding-inline: 2px; }
  .grid-wrap { grid-template-columns: repeat(auto-fill, minmax(128px, 1fr)); gap: 8px; padding: 8px; }
  .file-card { padding: 6px; }
}
@media (max-width: 560px) {
  .crumb { max-width: 52vw; }
}
</style>

<style scoped>
.file-workspace {
  display: block;
  min-width: 0;
}

.file-page-head {
  margin-bottom: 24px;
  animation: op-rise 480ms cubic-bezier(0.22, 0.72, 0.22, 1) both;
}

.file-page-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin: 16px 0 0;
  color: var(--op-ink-soft);
  font-size: 12px;
}

.file-page-meta .mono {
  color: var(--op-ink);
  font-size: 11px;
  font-weight: 600;
}

.meta-dot { color: var(--op-accent); }
.page-more { border-color: var(--op-rule-strong); }

.workspace-panel {
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-radius: 0;
  background: var(--op-surface);
  animation: op-rise 540ms 60ms cubic-bezier(0.22, 0.72, 0.22, 1) both;
}

.panel-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 62px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--op-rule);
  background: transparent;
}

.crumbs {
  display: flex;
  align-items: center;
  gap: 3px;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: thin;
}

.crumb {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
  gap: 6px;
  min-height: 34px;
  max-width: 240px;
  padding: 5px 8px;
  overflow: hidden;
  border: 0;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.crumb:hover:not(:disabled) { color: var(--op-accent); background: var(--op-accent-soft); }
.crumb.current { color: var(--op-ink); background: transparent; cursor: default; }
.crumb.home svg { color: var(--op-accent); }
.sep { flex: none; color: var(--op-ink-faint); }

.crumb-count {
  flex: 0 0 auto;
  margin-left: 8px;
  color: var(--op-ink-soft);
  font-family: var(--op-font-mono);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.panel-tools { display: flex; align-items: center; gap: 6px; min-width: 0; flex: 0 1 auto; }

.directory-search {
  width: clamp(180px, 22vw, 260px);
  --n-height: 38px !important;
  --n-border: 1px solid var(--op-rule) !important;
  --n-border-radius: 0 !important;
  --n-color: transparent !important;
}

.directory-search :deep(.n-input-wrapper) { padding-inline: 10px; }
.directory-search :deep(.n-input__prefix) { margin-right: 7px; color: var(--op-ink-faint); }

.view-toggle {
  display: inline-flex;
  gap: 0;
  padding: 0;
  border: 1px solid var(--op-rule);
  background: transparent;
}

.view-btn {
  width: 38px;
  height: 36px;
  border: 0;
  border-radius: 0;
  color: var(--op-ink-faint);
  background: transparent;
}

.view-btn + .view-btn { border-left: 1px solid var(--op-rule); }
.view-btn:hover { color: var(--op-accent); background: var(--op-accent-soft); }
.view-btn.active { color: var(--op-feature-ink); background: var(--op-feature-bg); }

.panel-bar .panel-tool,
.panel-bar .panel-cta,
.panel-bar .toolbar-more { display: none; }

.workspace-bulk {
  margin: 12px 14px 0;
  border-radius: 0;
}

.workspace-bulk-actions .n-button {
  --n-color: transparent !important;
  --n-text-color: var(--op-feature-ink) !important;
  --n-border: 1px solid color-mix(in srgb, var(--op-feature-ink) 34%, transparent) !important;
}

.workspace-bulk-actions .n-button:hover {
  --n-color: color-mix(in srgb, var(--op-feature-ink) 12%, transparent) !important;
}

.panel-body { position: relative; }
.panel-body.list-body { padding: 0; }

.list-body table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.list-body thead th {
  height: 42px;
  padding: 0 14px;
  border-bottom: 1px solid var(--op-rule);
  color: var(--op-ink-faint);
  background: var(--op-surface-muted);
  font-family: var(--op-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-align: left;
  text-transform: uppercase;
  white-space: nowrap;
}

.list-body thead .col-name { padding-left: 66px; }

.list-body tbody { counter-reset: file-row; }

.list-body tbody tr.row-clickable {
  counter-increment: file-row;
  height: 60px;
  border-bottom: 1px solid var(--op-rule);
  cursor: pointer;
  transition: background-color 160ms ease, box-shadow 160ms ease;
}

.list-body tbody tr.row-clickable:last-child { border-bottom: 0; }

.list-body tbody tr.row-clickable:hover,
.list-body tbody tr.row-clickable.selected {
  background: var(--op-accent-soft);
  box-shadow: inset 2px 0 0 var(--op-accent);
}

.list-body tbody td {
  height: 60px;
  padding: 7px 14px;
  border-bottom: 1px solid var(--op-rule);
  color: var(--op-ink-soft);
  font-size: 12px;
  vertical-align: middle;
}

.list-body tbody tr:last-child td { border-bottom: 0; }
.list-body tbody td.col-name { padding-left: 66px; }
.col-check { width: 42px; }
.col-hide-mobile { width: 116px; }
.col-time { width: 170px; }
.col-ops { width: 52px; }

.row-clickable .col-name::before {
  content: counter(file-row, decimal-leading-zero);
  position: absolute;
  left: 16px;
  top: 50%;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  letter-spacing: 0.04em;
  transform: translateY(-50%);
}

.row-clickable .col-name { position: relative; }
.name { display: flex; align-items: center; gap: 12px; min-width: 0; }
.name-copy { display: grid; min-width: 0; gap: 2px; }

.row-glyph {
  display: grid;
  place-items: center;
  flex: 0 0 42px;
  width: 42px;
  height: 42px;
  border: 1px solid var(--op-rule);
  background: var(--op-surface-raised);
}

.row-glyph .file-icon { width: 22px; height: 22px; flex-basis: 22px; }

.file-name {
  max-width: 100%;
  overflow: hidden;
  color: var(--op-ink);
  font-size: 13px;
  font-weight: 650;
  line-height: 1.35;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-open { border: 0; background: transparent; cursor: pointer; }
.file-open:hover { color: var(--op-accent); }
.file-meta-mobile { display: none; color: var(--op-ink-faint); font-size: 11px; }

.list-body .col-hide-mobile { font-family: var(--op-font-mono); font-size: 11px; font-variant-numeric: tabular-nums; }

.actions { display: flex; justify-content: flex-end; }
.op-btn {
  width: 36px;
  height: 36px;
  color: var(--op-ink-faint);
}

.op-btn:hover { color: var(--op-accent); background: var(--op-accent-soft); }

.reload-mask,
.grid-reload-mask {
  position: absolute;
  z-index: 3;
  inset: 0;
  display: grid;
  place-items: center;
  background: color-mix(in srgb, var(--op-surface) 78%, transparent);
  backdrop-filter: blur(2px);
}

.skeleton-row td { height: 60px; padding: 0 18px; }
.skeleton-bar {
  display: block;
  width: min(46%, 420px);
  height: 9px;
  background: linear-gradient(90deg, var(--gh-skeleton-a), var(--gh-skeleton-b), var(--gh-skeleton-a));
  background-size: 200% 100%;
  animation: file-shimmer 1.3s linear infinite;
}

@keyframes file-shimmer {
  to { background-position: -200% 0; }
}

.grid-wrap {
  counter-reset: file-card;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 1px;
  padding: 1px;
  background: var(--op-rule);
}

.file-card {
  counter-increment: file-card;
  position: relative;
  min-width: 0;
  padding: 14px;
  border: 0;
  border-radius: 0;
  background: var(--op-surface);
  cursor: pointer;
  transition: background-color 160ms ease;
}

.file-card:hover { background: var(--op-surface-raised); transform: none; }
.file-card.selected { background: var(--op-accent-soft); box-shadow: inset 0 2px 0 var(--op-accent); }

.file-card::before {
  content: counter(file-card, decimal-leading-zero);
  position: absolute;
  top: 14px;
  left: 14px;
  z-index: 1;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  letter-spacing: 0.05em;
}

.card-check { position: absolute; top: 11px; right: 48px; z-index: 2; opacity: 0; }
.card-more { position: absolute; top: 7px; right: 7px; z-index: 2; width: 34px; height: 34px; }
.file-card:hover .card-check,
.file-card:focus-within .card-check,
.file-card.selected .card-check { opacity: 1; }

.file-cover {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  min-height: 108px;
  margin: 22px 0 0;
  padding: 18px;
  border: 0;
  border-radius: 0;
  background: var(--op-surface-muted);
}

.file-cover.chip-folder { background: color-mix(in srgb, var(--op-accent) 14%, var(--op-surface-muted)); }
.file-cover.chip-image { background: color-mix(in srgb, #5f7f7b 16%, var(--op-surface-muted)); }
.file-cover.chip-video { background: color-mix(in srgb, #7b5d57 16%, var(--op-surface-muted)); }
.file-cover.chip-music { background: color-mix(in srgb, #7b637b 16%, var(--op-surface-muted)); }
.file-cover.chip-archive { background: color-mix(in srgb, #6f7b5b 16%, var(--op-surface-muted)); }

.plate-label {
  color: var(--op-ink-soft);
  font-family: var(--op-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
}

.file-cover .file-icon { width: 42px; height: 42px; flex-basis: 42px; }
.file-card-copy { padding: 13px 0 2px; }

.file-card-name.file-open {
  display: block;
  width: 100%;
  min-height: 0;
  padding: 0;
  overflow: hidden;
  color: var(--op-ink);
  font-size: 13px;
  font-weight: 650;
  line-height: 1.4;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-card-name.editing .rename-input { width: 100%; }
.file-card-meta {
  margin-top: 5px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  letter-spacing: 0.03em;
  text-transform: uppercase;
}

.rename-input {
  min-height: 30px;
  padding: 3px 6px;
  border: 1px solid var(--op-accent);
  color: var(--op-ink);
  background: var(--op-surface-raised);
  font-size: 13px;
  outline: none;
}

.ctx-menu {
  position: fixed;
  z-index: 400;
  min-width: 184px;
  padding: 4px;
  border: 1px solid var(--op-rule-strong);
  border-radius: 3px;
  background: var(--op-surface-raised);
  box-shadow: var(--op-shadow-md);
}

.ctx-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  min-height: 34px;
  padding: 6px 9px;
  border: 0;
  border-radius: 0;
  color: var(--op-ink);
  background: transparent;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.ctx-item:hover:not(:disabled) { color: var(--op-accent); background: var(--op-accent-soft); }
.ctx-item:disabled { opacity: 0.4; cursor: not-allowed; }

.sentinel-text {
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.grid-loading { min-height: 280px; display: grid; place-items: center; }

@media (max-width: 760px) {
  .file-page-head { margin-bottom: 18px; }
  .file-page-meta { margin-top: 12px; }
  .page-header-actions .btn { flex: 1; }
  .page-more { display: none; }
  .workspace-panel { border-inline: 0; border-right: 0; border-left: 0; }
  .panel-bar { min-height: 0; padding: 9px 10px; flex-wrap: wrap; }
  .crumbs { flex-basis: 100%; order: 2; }
  .panel-tools { width: 100%; order: 1; }
  .directory-search { flex: 1; width: auto; }
  .list-body thead .col-name,
  .list-body tbody td.col-name { padding-left: 48px; }
  .row-clickable .col-name::before { left: 10px; }
  .list-body thead th,
  .list-body tbody td { padding-inline: 8px; }
  .col-check { width: 34px; }
  .col-ops { width: 44px; }
  .row-glyph { flex-basis: 36px; width: 36px; height: 36px; }
  .row-glyph .file-icon { width: 20px; height: 20px; flex-basis: 20px; }
  .grid-wrap { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .file-card { padding: 11px; }
  .file-cover { min-height: 86px; padding: 14px; }
  .workspace-bulk { margin: 9px 10px 0; }
}

@media (max-width: 420px) {
  .grid-wrap { grid-template-columns: 1fr; }
}
</style>

<style scoped>
.file-workspace { display: block; min-width: 0; }

.file-page-head {
  align-items: flex-end;
  margin-bottom: 18px;
  padding-bottom: 0;
  animation: op-rise 240ms ease-out both;
}

.file-page-head .display {
  font-size: clamp(28px, 3.1vw, 36px);
  font-weight: 700;
  letter-spacing: -0.035em;
}

.file-page-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0 0;
  color: var(--op-ink-soft);
  font-size: 13px;
}

.file-page-meta .mono { color: var(--op-ink); font-size: 13px; font-weight: 600; }
.meta-dot { color: var(--op-ink-faint); }

.page-more { border: 1px solid var(--op-rule-strong); background: #fff; }

.workspace-panel {
  overflow: hidden;
  border: 1px solid var(--op-rule);
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--op-shadow-sm);
  animation: op-rise 300ms 40ms ease-out both;
}

.panel-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 58px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--op-rule);
  background: #fff;
}

.crumbs { display: flex; align-items: center; gap: 2px; min-width: 0; overflow-x: auto; }

.crumb {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
  gap: 7px;
  min-height: 34px;
  max-width: 240px;
  padding: 5px 8px;
  overflow: hidden;
  border: 0;
  border-radius: 7px;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.crumb:hover:not(:disabled) { color: var(--db-blue); background: var(--db-blue-soft); }
.crumb.current { color: var(--op-ink); background: transparent; cursor: default; font-weight: 600; }
.crumb.home svg { color: var(--db-blue); }
.sep { flex: none; color: var(--op-ink-faint); }

.crumb-count {
  flex: 0 0 auto;
  margin-left: 6px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-sans);
  font-size: 12px;
  letter-spacing: 0;
  text-transform: none;
}

.panel-tools { display: flex; align-items: center; gap: 7px; min-width: 0; flex: 0 1 auto; }

.directory-search {
  width: clamp(180px, 22vw, 250px);
  --n-height: 36px !important;
  --n-border: 1px solid var(--op-rule) !important;
  --n-border-radius: 8px !important;
  --n-color: var(--op-surface-muted) !important;
}

.directory-search :deep(.n-input__prefix) { color: var(--op-ink-faint); margin-right: 6px; }

.view-toggle {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--op-rule);
  border-radius: 8px;
  background: var(--op-surface-muted);
}

.view-btn {
  width: 32px;
  height: 30px;
  border: 0;
  border-radius: 6px;
  color: var(--op-ink-soft);
  background: transparent;
}

.view-btn + .view-btn { border-left: 0; }
.view-btn:hover { color: var(--db-blue); background: #fff; }
.view-btn.active { color: var(--db-blue); background: #fff; box-shadow: var(--op-shadow-sm); }

.panel-bar .panel-tool,
.panel-bar .panel-cta,
.panel-bar .toolbar-more { display: none; }

.workspace-bulk { margin: 10px 12px 0; border-radius: 8px; }
.workspace-bulk-actions .n-button {
  --n-color: #fff !important;
  --n-text-color: var(--db-blue) !important;
  --n-border: 1px solid #c7dbff !important;
}

.panel-body { position: relative; }
.panel-body.list-body { padding: 0; }
.list-body table { width: 100%; border-collapse: collapse; table-layout: fixed; }

.list-body thead th {
  height: 38px;
  padding: 0 12px;
  border-bottom: 1px solid var(--op-rule);
  color: var(--op-ink-faint);
  background: #f9fafb;
  font-family: var(--op-font-sans);
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0;
  text-align: left;
  text-transform: none;
  white-space: nowrap;
}

.list-body thead .col-name { padding-left: 12px; }
.list-body tbody tr.row-clickable {
  height: 54px;
  border-bottom: 1px solid #f0f1f4;
  cursor: pointer;
  transition: background-color 120ms ease;
}

.list-body tbody tr.row-clickable:last-child { border-bottom: 0; }
.list-body tbody tr.row-clickable:hover { background: #f9fafb; box-shadow: none; }
.list-body tbody tr.row-clickable.selected { background: var(--db-blue-soft); box-shadow: none; }

.list-body tbody td {
  height: 54px;
  padding: 6px 12px;
  border-bottom: 1px solid #f0f1f4;
  color: var(--op-ink-soft);
  font-size: 13px;
  vertical-align: middle;
}

.list-body tbody tr:last-child td { border-bottom: 0; }
.list-body tbody td.col-name { padding-left: 12px; }
.col-check { width: 40px; }
.col-hide-mobile { width: 110px; }
.col-time { width: 160px; }
.col-ops { width: 48px; }

.row-clickable .col-name::before { display: none; }
.name { display: flex; align-items: center; gap: 10px; min-width: 0; }
.name-copy { display: grid; min-width: 0; gap: 1px; }

.row-glyph {
  display: grid;
  place-items: center;
  flex: 0 0 36px;
  width: 36px;
  height: 36px;
  border: 1px solid var(--op-rule);
  border-radius: 8px;
  background: #fff;
}

.row-glyph .file-icon { width: 22px; height: 22px; flex-basis: 22px; }
.file-name { max-width: 100%; overflow: hidden; color: var(--op-ink); font-size: 14px; font-weight: 500; line-height: 1.35; text-align: left; text-overflow: ellipsis; white-space: nowrap; }
.file-open { border: 0; background: transparent; cursor: pointer; }
.file-open:hover { color: var(--db-blue); }
.file-meta-mobile { display: none; color: var(--op-ink-faint); font-size: 12px; }
.list-body .col-hide-mobile { color: var(--op-ink-soft); font-family: var(--op-font-sans); font-size: 13px; font-variant-numeric: tabular-nums; }

.actions { display: flex; justify-content: flex-end; }
.op-btn { width: 32px; height: 32px; color: var(--op-ink-faint); }
.op-btn:hover { color: var(--db-blue); background: var(--db-blue-soft); }

.reload-mask,
.grid-reload-mask {
  position: absolute;
  z-index: 3;
  inset: 0;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.76);
  backdrop-filter: none;
}

.skeleton-row td { height: 54px; padding: 0 14px; }
.skeleton-bar { display: block; width: min(42%, 380px); height: 8px; border-radius: 999px; background: linear-gradient(90deg, #f1f2f4, #e8eaee, #f1f2f4); background-size: 200% 100%; animation: file-shimmer 1.2s linear infinite; }
@keyframes file-shimmer { to { background-position: -200% 0; } }

.grid-wrap {
  counter-reset: none;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(196px, 1fr));
  gap: 12px;
  padding: 12px;
  background: #fff;
}

.file-card {
  counter-increment: none;
  position: relative;
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: #fff;
  cursor: pointer;
  transition: border-color 120ms ease, background-color 120ms ease, box-shadow 120ms ease;
}

.file-card:hover { border-color: #b8ccf5; background: #fff; box-shadow: var(--op-shadow-sm); transform: none; }
.file-card.selected { border-color: var(--db-blue); background: var(--db-blue-soft); box-shadow: 0 0 0 1px var(--db-blue) inset; }
.file-card::before { display: none; }
.card-check { position: absolute; top: 12px; right: 44px; z-index: 2; opacity: 0; }
.card-more { position: absolute; top: 7px; right: 7px; z-index: 2; width: 30px; height: 30px; }
.file-card:hover .card-check,
.file-card:focus-within .card-check,
.file-card.selected .card-check { opacity: 1; }

.file-cover {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  min-height: 104px;
  margin: 0;
  padding: 14px;
  border-radius: 8px;
  background: #f7f8fa;
}

.file-cover.chip-folder { background: var(--db-blue-soft); }
.file-cover.chip-image,
.file-cover.chip-video,
.file-cover.chip-music,
.file-cover.chip-archive { background: #f7f8fa; }
.file-cover .file-icon { width: 40px; height: 40px; flex-basis: 40px; }

.plate-label { color: var(--op-ink-faint); font-family: var(--op-font-sans); font-size: 11px; font-weight: 500; letter-spacing: 0; text-transform: uppercase; }
.file-card-copy { padding: 10px 2px 2px; }

.file-card-name.file-open {
  display: block;
  width: 100%;
  min-height: 0;
  padding: 0;
  overflow: hidden;
  color: var(--op-ink);
  font-size: 14px;
  font-weight: 500;
  line-height: 1.35;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-card-name.editing .rename-input { width: 100%; }
.file-card-meta { margin-top: 4px; color: var(--op-ink-faint); font-family: var(--op-font-sans); font-size: 11px; letter-spacing: 0; text-transform: none; }

.rename-input { min-height: 30px; padding: 3px 7px; border: 1px solid var(--db-blue); border-radius: 6px; color: var(--op-ink); background: #fff; font-size: 14px; outline: none; }

.ctx-menu {
  position: fixed;
  z-index: 400;
  min-width: 180px;
  padding: 4px;
  border: 1px solid var(--op-rule);
  border-radius: 8px;
  background: #fff;
  box-shadow: var(--op-shadow-md);
}

.ctx-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  min-height: 34px;
  padding: 6px 9px;
  border: 0;
  border-radius: 6px;
  color: var(--op-ink);
  background: transparent;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.ctx-item:hover:not(:disabled) { color: var(--db-blue); background: var(--db-blue-soft); }
.ctx-item:disabled { opacity: 0.4; cursor: not-allowed; }
.sentinel-text { color: var(--op-ink-faint); font-family: var(--op-font-sans); font-size: 12px; letter-spacing: 0; text-transform: none; }
.grid-loading { min-height: 260px; display: grid; place-items: center; }

@media (max-width: 760px) {
  .file-page-head { margin-bottom: 14px; }
  .file-page-head { align-items: flex-start; }
  .file-page-head .page-header-copy { width: 100%; margin: 0; text-align: left; }
  .file-page-head .display { text-align: left; }
  .file-page-meta { margin-top: 6px; }
  .page-header-actions .btn { flex: 1; }
  .page-more { display: none; }
  .workspace-panel { border-right: 0; border-left: 0; border-radius: 0; }
  .panel-bar { min-height: 0; padding: 9px 10px; flex-wrap: wrap; }
  .crumbs { flex-basis: 100%; order: 2; }
  .panel-tools { width: 100%; order: 1; }
  .directory-search { flex: 1; width: auto; }
  .list-body thead .col-name,
  .list-body tbody td.col-name { padding-left: 8px; }
  .list-body thead th,
  .list-body tbody td { padding-inline: 8px; }
  .col-check { width: 34px; }
  .col-ops { width: 40px; }
  .row-glyph { flex-basis: 34px; width: 34px; height: 34px; }
  .row-glyph .file-icon { width: 20px; height: 20px; flex-basis: 20px; }
  .file-meta-mobile { display: block; }
  .grid-wrap { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; padding: 10px; }
  .file-card { padding: 8px; }
  .file-cover { min-height: 86px; padding: 12px; }
  .workspace-bulk { margin: 8px 10px 0; }
}

@media (max-width: 420px) {
  .grid-wrap { grid-template-columns: 1fr; }
}
</style>
