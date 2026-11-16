/** API 访问封装：统一 Bearer 鉴权、错误解析、401 全局回调 */

let authToken = "";
try {
  authToken = localStorage.getItem("appToken") || "";
} catch {
  authToken = "";
}

let onUnauthorized: (() => void) | null = null;

export function setAuthToken(token: string): void {
  authToken = token;
  try {
    if (token) localStorage.setItem("appToken", token);
    else localStorage.removeItem("appToken");
  } catch {
    // localStorage 不可用时仅保留内存态
  }
}

export function getAuthToken(): string {
  return authToken;
}

export function onUnauthorizedCallback(cb: (() => void) | null): void {
  onUnauthorized = cb;
}

export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
  }
}

export async function api<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers || {});
  if (authToken) headers.set("Authorization", "Bearer " + authToken);
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    let msg = await res.text();
    try {
      msg = JSON.parse(msg).error || msg;
    } catch {
      // keep raw text
    }
    if (res.status === 401 && onUnauthorized) onUnauthorized();
    throw new ApiError(res.status, msg);
  }
  const ct = res.headers.get("Content-Type") || "";
  return (ct.includes("application/json") ? res.json() : res) as Promise<T>;
}

/** 无需登录态的请求（登录页校验用） */
export async function rawApi<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers || {});
  if (authToken) headers.set("Authorization", "Bearer " + authToken);
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    let msg = await res.text();
    try {
      msg = JSON.parse(msg).error || msg;
    } catch {
      // keep raw text
    }
    throw new ApiError(res.status, msg);
  }
  return (await res.json()) as T;
}

// ---- 类型定义（与后端返回结构对应） ----

export interface WopanFile {
  id?: string;
  fid?: string;
  name?: string;
  size?: number | string;
  type?: number | string;
  fileType?: string;
  createTime?: string;
  updateTime?: string;
  uploadTime?: string;
  modifyTime?: string;
  shootingTime?: string;
  parentDirectoryId?: string;
}

export interface ListResult {
  files: WopanFile[];
  pageNum: number;
  pageSize: number;
  hasMore: boolean;
}

export interface AuthCheckResult {
  ok: boolean;
  /** 首屏合并下发的云盘登录态，省掉一次 login/status 往返 */
  login?: LoginStatusResult;
}

/** 首次部署的初始化状态（公开接口） */
export interface SetupStatusResult {
  initialized: boolean;
}

export interface LoginStatusResult {
  configured: boolean;
  phone: string;
  /** 全局设置：是否显示隐藏文件（以 . 开头） */
  showHidden?: boolean;
  autoRefresh?: {
    enabled: boolean;
    paused?: boolean;
    intervalHours: number;
    lastAttemptAt: number | null;
    lastSuccessAt: number | null;
    nextRefreshAt: number | null;
    lastError: string | null;
  };
}

export interface DetailResult {
  phone: string;
  nickName: string;
  avatarUrl: string;
  totalSize: number;
  usedSize: number;
  imageSize: number;
  videoSize: number;
  audioSize: number;
  otherSize: number;
  vipLevel: string;
  expireTime: string;
  /** 服务端缓存写入时间（毫秒），来自缓存时有值 */
  cachedAt?: number;
  fromCache?: boolean;
  stale?: boolean;
}

/** 应用设置（GET /api/settings） */
export interface AppSettingsResult {
  signParamConfigured: boolean;
  showHidden: boolean;
  webdavConfigured: boolean;
  webdavUsername: string;
  webdavDirectDownload: boolean;
}

/** 联通云盘官方账号资料（GET /api/user/profile） */
export interface UserProfile {
  phone: string;
  userName: string;
  headUrl: string;
  sex: string;
  /** 毫秒时间戳；上游返回的可能是字符串，用 toBirthday() 统一转 number */
  birthday: number | string | null;
}

/** 把上游的生日（数字或数字字符串）统一成毫秒时间戳，非法值返回 null */
export function toBirthday(value: number | string | null | undefined): number | null {
  if (typeof value === "number") return Number.isFinite(value) && value > 0 ? value : null;
  if (typeof value === "string" && value.trim()) {
    const n = Number(value);
    return Number.isFinite(n) && n > 0 ? n : null;
  }
  return null;
}

export interface UpdateProfilePayload {
  userName?: string;
  sex?: string;
  headUrl?: string;
  birthday?: number;
}

export interface UploadSession {
  uploadUrl: string;
  partSize: number;
  totalPart: number;
  fields: Record<string, string>;
}

export interface DirectUrlEntry {
  fid: string;
  url: string;
  bedUrl: string;
}

export interface ShareData {
  shareUrl?: string;
  shareCode?: string;
}

/** 分享方式：官方链接 / 本平台链接+提取码 / 直链 */
export type ShareMethod = "official" | "platform" | "direct";

export interface ShareOfficialResult {
  shareId: string;
  shareCode: string;
  shareUrl: string;
  days: number;
}

export interface SharePlatformResult {
  id: string;
  code: string;
  url: string;
  /** 毫秒时间戳，null 表示永久 */
  expiresAt: number | null;
}

export interface ShareDirectResult {
  files: DirectUrlEntry[];
  packUrl: string;
}

export interface ShareCreateResult {
  official?: ShareOfficialResult;
  platform?: SharePlatformResult;
  direct?: ShareDirectResult;
  /** 某几种方式失败时的原因（其他方式可能已经成功） */
  errors?: Partial<Record<ShareMethod, string>>;
}

export interface OfficialShareRow {
  shareId: string;
  name: string;
  count: number;
  status: string;
  shareChannel: string;
  shareTime: number | null;
  invalidTime: number | null;
  spaceType: string;
}

export interface PlatformShareRow {
  id: string;
  code: string;
  items: Array<{ id: string; fid: string; type: number; name: string; size?: number }>;
  createdAt: number;
  expiresAt: number | null;
}

export interface SharesResult {
  official: { list: OfficialShareRow[]; total: number; error?: string };
  platform: PlatformShareRow[];
  page: number;
  pageSize: number;
}

export interface ResolveResult {
  url: string;
  name?: string;
  packed?: boolean;
}

export interface SelectedItem {
  id: string;
  fid: string;
  type: number;
  name: string;
}
