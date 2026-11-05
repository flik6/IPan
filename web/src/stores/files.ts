import { reactive } from "vue";
import { api, type ListResult, type WopanFile, type SelectedItem } from "../api/client";
import { ui, type AppView } from "./ui";

export interface Crumb {
  id: string;
  name: string;
}

interface FilesState {
  stack: Crumb[];
  files: WopanFile[];
  page: number;
  pageSize: number;
  hasMore: boolean;
  loading: boolean;
  /** 正在加载下一页（滚动触底时用，避免和首屏 loading 抢加载态） */
  loadingMore: boolean;
}

export const files = reactive<FilesState>({
  stack: [{ id: "0", name: "根目录" }],
  files: [],
  page: 0,
  pageSize: 50,
  hasMore: false,
  loading: false,
  loadingMore: false,
});

/** 各组件可注册的「加载后回调」（如刷新云盘状态） */
const afterLoadHooks: Array<() => void> = [];
export function onAfterLoad(cb: () => void): void {
  afterLoadHooks.push(cb);
}

// ---- 浏览历史：与浏览器历史栈同步，使界面返回按钮和系统返回键行为一致 ----

/**
 * 历史条目只记录「在哪个视图、哪个目录」，不记录翻页进度：
 * 滚动加载是连续动作，回到某个目录时从头加载即可。
 */
interface NavSnapshot {
  view: AppView;
  stack: Crumb[];
}

const navEntries: NavSnapshot[] = [];
let navIndex = -1;
let historyBound = false;

function snapshot(): NavSnapshot {
  return { view: ui.view, stack: files.stack.map((c) => ({ ...c })) };
}

function sameSnapshot(a: NavSnapshot, b: NavSnapshot): boolean {
  if (a.view !== b.view || a.stack.length !== b.stack.length) return false;
  return a.stack.every((c, i) => c.id === b.stack[i].id);
}

/** 记录当前界面状态；replace 用于「原地切换」（如打开设置页），不新增历史条目 */
function pushSnapshot(replace = false): void {
  const snap = snapshot();
  if (!replace && navIndex >= 0 && sameSnapshot(navEntries[navIndex], snap)) return;
  if (replace && navIndex >= 0) navEntries[navIndex] = snap;
  else {
    navEntries.splice(navIndex + 1);
    navEntries.push(snap);
    navIndex = navEntries.length - 1;
  }
  if (!historyBound) return;
  const state = { ...snap, __ipan: true };
  if (replace) history.replaceState(state, "");
  else history.pushState(state, "");
}

/** 把状态写进当前历史条目的 state（进入主界面时调用，可重复调用） */
export function initNavHistory(): void {
  navEntries.length = 0;
  navIndex = -1;
  navEntries.push(snapshot());
  navIndex = 0;
  history.replaceState({ ...snapshot(), __ipan: true }, "");
  if (!historyBound) {
    historyBound = true;
    window.addEventListener("popstate", onPopState);
  }
}

function onPopState(event: PopStateEvent): void {
  const state = event.state as (NavSnapshot & { __ipan?: boolean }) | null;
  // 退出了本应用管理的条目：交给浏览器继续后退
  if (!state?.__ipan) {
    navIndex = Math.max(0, navIndex - 1);
    return;
  }
  const target: NavSnapshot = { view: state.view, stack: state.stack };
  const idx = navEntries.findIndex((e) => sameSnapshot(e, target));
  if (idx >= 0) navIndex = idx;
  else {
    navEntries.splice(navIndex + 1);
    navEntries.push(target);
    navIndex = navEntries.length - 1;
  }
  const changed = !sameSnapshot(snapshot(), target);
  ui.view = target.view;
  files.stack = target.stack.map((c) => ({ ...c }));
  if (changed) {
    clearSelection();
    ui.fileSearch = "";
    void load(0);
  }
}

export function currentId(): string {
  return files.stack[files.stack.length - 1].id;
}

export function currentDirectoryName(): string {
  return files.stack[files.stack.length - 1].name;
}

/** 请求序号：目录切换/翻页时后发请求覆盖先发请求，避免旧响应写回新目录 */
let loadSeq = 0;

/** 加载指定页并替换列表（首屏、刷新、返回上级时使用） */
export async function load(
  page?: number,
  opts: {
    resetSelection?: () => void;
    onError?: (msg: string) => void;
    onFatal?: (msg: string) => void;
    /** 切换目录时清空旧列表，避免请求期间仍显示上一个目录的内容 */
    clear?: boolean;
  } = {},
): Promise<void> {
  if (typeof page === "number") files.page = page;
  const seq = ++loadSeq;
  const parentId = currentId();
  const targetPage = files.page;
  files.loading = true;
  if (opts.clear) {
    files.files = [];
    files.hasMore = false;
  }
  try {
    const data = await api<ListResult>("/api/list?parentId=" + parentId + "&page=" + targetPage + "&pageSize=" + files.pageSize);
    if (seq !== loadSeq) return;
    files.files = data.files || [];
    files.hasMore = !!data.hasMore;
    opts.resetSelection?.();
    afterLoadHooks.forEach((cb) => cb());
  } catch (e) {
    if (seq !== loadSeq) return;
    const msg = e instanceof Error ? e.message : String(e);
    if (/unauthorized|401|未授权|访问密码|认证/i.test(msg)) {
      // 全局 401 拦截已处理跳转，这里仅重置本地状态
      files.stack = [{ id: "0", name: "根目录" }];
      files.files = [];
      opts.onFatal?.(msg);
    } else {
      opts.onError?.(msg);
    }
  } finally {
    if (seq === loadSeq) files.loading = false;
  }
}

/**
 * 滚动触底时追加下一页。不经过历史栈：加载更多是一个连续动作，
 * 中途回退没有意义，目录层级才是真正需要返回的粒度。
 */
export async function loadMore(opts: { onError?: (msg: string) => void } = {}): Promise<void> {
  if (files.loading || files.loadingMore || !files.hasMore) return;
  const seq = loadSeq;
  const parentId = currentId();
  const nextPage = files.page + 1;
  files.loadingMore = true;
  try {
    const data = await api<ListResult>("/api/list?parentId=" + parentId + "&page=" + nextPage + "&pageSize=" + files.pageSize);
    // 期间切换了目录或刷新：丢弃这批结果
    if (seq !== loadSeq) return;
    const incoming = data.files || [];
    files.files = incoming.length ? [...files.files, ...incoming] : files.files;
    // 空页视为到底，避免 hasMore 与内容不一致时反复请求同一页
    files.hasMore = incoming.length > 0 && !!data.hasMore;
    files.page = nextPage;
  } catch (e) {
    if (seq !== loadSeq) return;
    opts.onError?.((e as Error).message);
  } finally {
    if (seq === loadSeq) files.loadingMore = false;
  }
}

export function openDirectory(id: string, name: string): void {
  if (currentId() === id) return;
  files.stack.push({ id, name });
  files.page = 0;
  clearSelection();
  pushSnapshot();
  void load(undefined, { clear: true });
}

export function jumpTo(index: number): void {
  if (index < 0 || index >= files.stack.length - 1) return;
  files.stack = files.stack.slice(0, index + 1);
  files.page = 0;
  clearSelection();
  pushSnapshot();
  void load(undefined, { clear: true });
}

export function goBack(): void {
  goUp();
}

/**
 * 返回上一级：优先复用浏览器历史（保证系统返回键与界面按钮一致），
 * 历史里没有对应父目录时再原地跳转。
 */
export function goUp(): void {
  if (ui.view === "settings" || ui.view === "shares") {
    const prev = navEntries[navIndex - 1];
    if (historyBound && prev && prev.view === "files") {
      history.back();
      return;
    }
    setView("files");
    return;
  }
  if (files.stack.length <= 1) return;
  const parentStack = files.stack.slice(0, -1).map((c) => ({ ...c }));
  const prev = navEntries[navIndex - 1];
  if (historyBound && prev && sameSnapshot(prev, { view: "files", stack: parentStack })) {
    history.back();
    return;
  }
  files.stack = parentStack;
  files.page = 0;
  clearSelection();
  pushSnapshot();
  void load();
}

export function canGoUp(): boolean {
  if (ui.view === "settings" || ui.view === "shares") return true;
  return files.stack.length > 1;
}

/** 切换主视图（文件 / 设置），并作为一条可返回的历史记录 */
export function setView(view: AppView): void {
  if (ui.view === view) return;
  ui.view = view;
  ui.fileSearch = "";
  pushSnapshot();
}

/** 回到文件根目录（可跨视图），只写一条历史记录 */
export function goRoot(): void {
  if (files.loading) return;
  const viewChanged = ui.view !== "files";
  const atRoot = files.stack.length <= 1;
  if (!viewChanged && atRoot) return;
  ui.view = "files";
  ui.fileSearch = "";
  if (atRoot) {
    pushSnapshot();
    return;
  }
  files.stack = [{ id: "0", name: "根目录" }];
  files.page = 0;
  clearSelection();
  pushSnapshot();
  void load();
}

export function findFile(id: string): WopanFile | undefined {
  return files.files.find((f) => (f.id || f.fid) === id);
}

export function fileIds(f: WopanFile): { id: string; fid: string } {
  const id = f.id || f.fid || "";
  const fid = f.fid || id;
  return { id, fid };
}

// ---- 批量选择 ----

interface SelectionState {
  checkedIds: Set<string>;
  selectAll: boolean;
}

export const selection = reactive<SelectionState>({
  checkedIds: new Set<string>(),
  selectAll: false,
});

export function selectedItems(): SelectedItem[] {
  return Array.from(selection.checkedIds)
    .map((id) => {
      const f = findFile(id);
      if (!f) return null;
      const { id: realId, fid } = fileIds(f);
      return { id: realId, fid, type: Number(f.type) === 0 ? 0 : 1, name: f.name || realId };
    })
    .filter((x): x is SelectedItem => x !== null);
}

export function toggleSelect(id: string, checked: boolean): void {
  if (checked) selection.checkedIds.add(id);
  else selection.checkedIds.delete(id);
  selection.selectAll = selection.checkedIds.size > 0 && selection.checkedIds.size === files.files.length;
}

export function toggleSelectAll(checked: boolean): void {
  selection.selectAll = checked;
  selection.checkedIds.clear();
  if (checked) {
    for (const f of files.files) {
      const { id } = fileIds(f);
      selection.checkedIds.add(id);
    }
  }
}

export function clearSelection(): void {
  selection.checkedIds.clear();
  selection.selectAll = false;
}

export function resetToRoot(): void {
  files.stack = [{ id: "0", name: "根目录" }];
  files.files = [];
  files.page = 0;
  clearSelection();
  navEntries.length = 0;
  navIndex = -1;
  if (typeof history !== "undefined") history.replaceState(null, "");
}
