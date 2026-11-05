import { reactive } from "vue";
import type { SelectedItem, ShareCreateResult, ShareMethod, WopanFile, ResolveResult } from "../api/client";

type PickerAction = "move" | "copy";

export type AppView = "files" | "settings" | "shares";

interface UiState {
  // 布局
  view: AppView;
  fileSearch: string;
  mobileNav: boolean;
  fileViewMode: "list" | "grid";
  // 弹窗
  showResolve: boolean;
  showPreview: boolean;
  showDetail: boolean;
  showShareCreate: boolean;
  showFolderPicker: boolean;
  showProfileEdit: boolean;
  // 数据
  previewFile: { fid: string; name: string; kind: string } | null;
  detailFile: WopanFile | null;
  /** 分享创建弹窗：待分享项目 */
  shareTargets: SelectedItem[];
  /** 分享创建弹窗：用户勾选的分享方式 */
  shareMethods: ShareMethod[];
  /** 分享创建结果（三种方式各自可选） */
  shareResult: ShareCreateResult | null;
  shareError: string;
  shareLoading: boolean;
  picker: { action: PickerAction; items: SelectedItem[] } | null;
  resolveResult: ResolveResult | null;
  dropActive: boolean;
}

export const ui = reactive<UiState>({
  view: "files",
  fileSearch: "",
  mobileNav: false,
  // 文件列表视图模式，list=表格 / grid=卡片网格，持久化到 localStorage
  fileViewMode: ((): "list" | "grid" => {
    try {
      return localStorage.getItem("wopan.fileView") === "grid" ? "grid" : "list";
    } catch {
      return "list";
    }
  })(),
  showResolve: false,
  showPreview: false,
  showDetail: false,
  showShareCreate: false,
  showFolderPicker: false,
  showProfileEdit: false,
  previewFile: null,
  detailFile: null,
  shareTargets: [],
  shareMethods: ["official"],
  shareResult: null,
  shareError: "",
  shareLoading: false,
  picker: null,
  resolveResult: null,
  dropActive: false,
});

export function openFolderPicker(action: PickerAction, items: SelectedItem[]): void {
  ui.picker = { action, items };
  ui.showFolderPicker = true;
}

/** 打开分享弹窗：默认勾选「官方链接」 */
export function openShareDialog(items: SelectedItem[], methods: ShareMethod[] = ["official"]): void {
  ui.shareTargets = items;
  ui.shareMethods = [...methods];
  ui.shareResult = null;
  ui.shareError = "";
  ui.shareLoading = false;
  ui.showShareCreate = true;
}
