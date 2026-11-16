import { reactive } from "vue";
import { api, toBirthday, type DetailResult, type UserProfile } from "../api/client";

interface WopanUserState {
  nickName: string;
  avatarUrl: string;
  phone: string;
  vipLevel: string;
  totalSize: number;
  usedSize: number;
  /** 性别，取值与官方一致：男 / 女，空串表示未设置 */
  sex: string;
  /** 生日时间戳（毫秒），null 表示未设置 */
  birthday: number | null;
  loaded: boolean;
  failed: boolean;
  /** 账号信息缓存时间（毫秒），0 表示从未获取 */
  syncedAt: number;
}

const STORAGE_KEY = "wopan.accountInfo";

export const wopanUser = reactive<WopanUserState>({
  nickName: "",
  avatarUrl: "",
  phone: "",
  vipLevel: "",
  totalSize: 0,
  usedSize: 0,
  sex: "",
  birthday: null,
  loaded: false,
  failed: false,
  syncedAt: 0,
});

type CachedInfo = Omit<WopanUserState, "loaded" | "failed">;

function readLocalCache(): CachedInfo | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<CachedInfo>;
    if (!parsed || typeof parsed.syncedAt !== "number") return null;
    return {
      nickName: parsed.nickName || "",
      avatarUrl: parsed.avatarUrl || "",
      phone: parsed.phone || "",
      vipLevel: parsed.vipLevel || "",
      totalSize: Number(parsed.totalSize || 0),
      usedSize: Number(parsed.usedSize || 0),
      sex: parsed.sex || "",
      birthday: typeof parsed.birthday === "number" ? parsed.birthday : null,
      syncedAt: parsed.syncedAt,
    };
  } catch {
    return null;
  }
}

function writeLocalCache(): void {
  try {
    const snapshot: CachedInfo = {
      nickName: wopanUser.nickName,
      avatarUrl: wopanUser.avatarUrl,
      phone: wopanUser.phone,
      vipLevel: wopanUser.vipLevel,
      totalSize: wopanUser.totalSize,
      usedSize: wopanUser.usedSize,
      sex: wopanUser.sex,
      birthday: wopanUser.birthday,
      syncedAt: wopanUser.syncedAt,
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshot));
  } catch {
    // 无 localStorage 时只保留内存态
  }
}

/** 启动时先用本地缓存填充，避免首屏为空再闪烁一次 */
export function hydrateWopanUser(): void {
  if (wopanUser.loaded) return;
  const cached = readLocalCache();
  if (!cached) return;
  Object.assign(wopanUser, cached, { loaded: true, failed: false });
}

/** 账号信息缓存有效期：过期后静默回源，用户无需手动获取 */
const SYNC_TTL_MS = 10 * 60 * 1000;
/** 后台同步轮询间隔 */
const SYNC_INTERVAL_MS = 10 * 60 * 1000;

let syncTimer: ReturnType<typeof setInterval> | null = null;
let syncInFlight = false;

/** 后台同步的 visibilitychange 监听（需要能在停止时精确移除） */
function onVisibilityChange(): void {
  if (document.visibilityState === "visible") void refreshWopanUser();
}

/**
 * 获取官方账号信息（昵称/头像/VIP/容量）。
 *
 * 本地缓存优先：命中且未过期就直接用，避免首屏为空再闪烁。
 * 过期或 force=true 时回源；失败保留旧数据，不打断界面。
 */
export async function refreshWopanUser(options: { force?: boolean } = {}): Promise<void> {
  const force = options.force === true;
  if (!wopanUser.loaded) hydrateWopanUser();
  if (!force && wopanUser.loaded) {
    const age = Date.now() - wopanUser.syncedAt;
    // 缓存仍在有效期内直接复用，避免每次进页面都打上游
    if (wopanUser.syncedAt > 0 && age >= 0 && age < SYNC_TTL_MS) return;
  }
  if (syncInFlight) return;
  syncInFlight = true;
  try {
    const d = await api<DetailResult>(force ? "/api/detail?refresh=1" : "/api/detail");
    wopanUser.nickName = d.nickName || "";
    wopanUser.avatarUrl = d.avatarUrl || "";
    wopanUser.phone = d.phone || "";
    wopanUser.vipLevel = d.vipLevel || "";
    wopanUser.totalSize = Number(d.totalSize || 0);
    wopanUser.usedSize = Number(d.usedSize || 0);
    wopanUser.syncedAt = Number(d.cachedAt || Date.now());
    wopanUser.loaded = true;
    wopanUser.failed = false;
    writeLocalCache();
  } catch {
    // 未登录云盘或接口失败时保持已有数据，不做清空
    wopanUser.failed = true;
  } finally {
    syncInFlight = false;
  }
}

/**
 * 启动后台账号同步：进应用时对齐一次，之后定时静默刷新。
 *
 * 与联通云盘客户端一致——用户不需要点按钮，资料和容量自己跟上；
 * 页面隐藏时跳过请求，切回前台再补一次。
 */
export function startWopanUserSync(): void {
  if (syncTimer) return;
  void refreshWopanUser();
  syncTimer = setInterval(() => {
    if (document.visibilityState === "visible") void refreshWopanUser();
  }, SYNC_INTERVAL_MS);
  document.addEventListener("visibilitychange", onVisibilityChange);
}

export function stopWopanUserSync(): void {
  if (syncTimer) {
    clearInterval(syncTimer);
    syncTimer = null;
  }
  document.removeEventListener("visibilitychange", onVisibilityChange);
}

/** 拉取可编辑的账号资料（昵称/头像/性别/生日），供编辑弹窗使用 */
export async function loadWopanProfile(): Promise<UserProfile> {
  const p = await api<UserProfile>("/api/user/profile");
  applyProfile(p);
  return p;
}

/** 把最新的资料写回全局状态，保证顶栏等处即时更新 */
export function applyProfile(p: Partial<UserProfile>): void {
  if (p.userName !== undefined) wopanUser.nickName = p.userName || "";
  if (p.headUrl !== undefined) wopanUser.avatarUrl = p.headUrl || "";
  if (p.phone !== undefined && p.phone) wopanUser.phone = p.phone;
  if (p.sex !== undefined) wopanUser.sex = p.sex || "";
  if (p.birthday !== undefined) wopanUser.birthday = toBirthday(p.birthday);
  wopanUser.loaded = true;
  wopanUser.syncedAt = Date.now();
  writeLocalCache();
}

/** 退出登录或切换云盘账号时清空，避免展示上一个账号的资料 */
export function resetWopanUser(): void {
  wopanUser.nickName = "";
  wopanUser.avatarUrl = "";
  wopanUser.phone = "";
  wopanUser.vipLevel = "";
  wopanUser.totalSize = 0;
  wopanUser.usedSize = 0;
  wopanUser.sex = "";
  wopanUser.birthday = null;
  wopanUser.loaded = false;
  wopanUser.failed = false;
  wopanUser.syncedAt = 0;
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}
