import { computed, reactive } from "vue";

/**
 * 主题偏好：浅色 / 深色 / 跟随系统。
 *
 * 实际生效的明暗写到 <html data-theme>，global.css 用它切换 --gh-* 变量；
 * index.html 里有一段内联脚本在首屏前做同样的计算，避免先渲染浅色再翻深色。
 */

export type ThemeMode = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

const STORAGE_KEY = "wopan.theme";

function readStoredMode(): ThemeMode {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === "light" || raw === "dark" || raw === "system") return raw;
  } catch {
    // localStorage 不可用时退化为跟随系统
  }
  return "system";
}

const mediaQuery = typeof window !== "undefined" ? window.matchMedia("(prefers-color-scheme: dark)") : null;

interface ThemeState {
  mode: ThemeMode;
  /** 系统当前是否为深色（mode=system 时决定实际外观） */
  systemDark: boolean;
}

export const theme = reactive<ThemeState>({
  mode: readStoredMode(),
  systemDark: !!mediaQuery?.matches,
});

/** 当前实际使用的明暗主题（naive-ui 的 theme 与 data-theme 都取这个值） */
export const resolvedTheme = computed<ResolvedTheme>(() => {
  if (theme.mode === "system") return theme.systemDark ? "dark" : "light";
  return theme.mode;
});

export const isDark = computed(() => resolvedTheme.value === "dark");

function applyToDocument(value: ResolvedTheme): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  root.dataset.theme = value;
  // 让浏览器原生控件（滚动条、输入框、日期选择器）跟着变色
  root.style.colorScheme = value;
}

/** 把当前生效主题写入 DOM。切换 mode 或系统主题变化时都要调用 */
export function syncTheme(): void {
  applyToDocument(resolvedTheme.value);
}

export function setThemeMode(mode: ThemeMode): void {
  theme.mode = mode;
  try {
    localStorage.setItem(STORAGE_KEY, mode);
  } catch {
    // 存不下就只在本次会话生效
  }
  syncTheme();
}

/** 监听系统主题变化；返回取消监听的函数 */
export function watchSystemTheme(): () => void {
  if (!mediaQuery) return () => {};
  const handler = (event: MediaQueryListEvent): void => {
    theme.systemDark = event.matches;
    if (theme.mode === "system") syncTheme();
  };
  mediaQuery.addEventListener("change", handler);
  return () => mediaQuery.removeEventListener("change", handler);
}

/** 应用启动时调用：对齐一次系统状态并写入 DOM */
export function initTheme(): void {
  if (mediaQuery) theme.systemDark = mediaQuery.matches;
  syncTheme();
}
