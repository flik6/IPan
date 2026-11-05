import { reactive } from "vue";
import { api, rawApi, setAuthToken, getAuthToken, onUnauthorizedCallback, type AuthCheckResult, type LoginStatusResult, type SetupStatusResult } from "../api/client";
import { applySettings } from "./settings";
import { resetWopanUser } from "../composables/useWopanUser";

interface AuthState {
  loggedIn: boolean;
  checking: boolean;
  ready: boolean;
  loginInProgress: boolean;
  cloudConfigured: boolean;
  cloudChecked: boolean;
  cloud: LoginStatusResult["autoRefresh"] | null;
  /** 首次部署尚未设置访问密码 */
  needsSetup: boolean;
}

export const auth = reactive<AuthState>({
  loggedIn: !!getAuthToken(),
  checking: true,
  ready: false,
  loginInProgress: false,
  cloudConfigured: false,
  cloudChecked: false,
  cloud: null,
  needsSetup: false,
});

/** 把服务端下发的登录态写入本地，供首屏复用 */
export function applyLoginStatus(data: LoginStatusResult): void {
  auth.cloudConfigured = !!data.configured;
  auth.cloud = data.autoRefresh ?? null;
  auth.cloudChecked = true;
  if (data.showHidden !== undefined) applySettings({ showHidden: data.showHidden });
}

/** 查询是否已完成初始化（未初始化时展示「设置访问密码」页） */
export async function checkSetup(): Promise<boolean> {
  try {
    const res = await fetch("/api/setup/status");
    if (!res.ok) return false;
    const data = (await res.json()) as SetupStatusResult;
    auth.needsSetup = !data.initialized;
    return auth.needsSetup;
  } catch {
    return false;
  }
}

/** 首次初始化：写入访问密码（签名密钥由服务端随机生成） */
export async function completeSetup(data: {
  password: string;
  signParam?: string;
  appToken?: string;
}): Promise<void> {
  const res = await fetch("/api/setup", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    let msg = await res.text();
    try {
      msg = JSON.parse(msg).error || msg;
    } catch {
      // keep raw
    }
    throw new Error(msg || "初始化失败");
  }
  const result = (await res.json()) as { token?: string };
  const token = result.token || data.password;
  if (token) {
    setAuthToken(token);
    auth.loggedIn = true;
  }
  // 注意：这里不能立刻把 needsSetup 置为 false。
  // 一旦翻转，App 的 stage 会立即从 setup 变成 boot，初始化页被卸载，
  // 紧接着组件再 emit("done") 就没人接收，界面会卡在加载动画上。
  // 由调用方在 emit 之后显式调用 markSetupDone()。
}

/** 由初始化页在完成回调里调用，关闭初始化引导 */
export function markSetupDone(): void {
  auth.needsSetup = false;
}

/** 启动时校验本地保存的密码；成功进入主界面，失败回登录页 */
export async function checkAuth(): Promise<boolean> {
  const v = getAuthToken();
  if (!v) {
    auth.loggedIn = false;
    auth.checking = false;
    return false;
  }
  try {
    const data = await rawApi<AuthCheckResult>("/api/auth-check");
    auth.loggedIn = true;
    auth.checking = false;
    // 服务端已合并返回云盘登录态，省掉一次串行请求
    if (data.login) applyLoginStatus(data.login);
    return true;
  } catch {
    setAuthToken("");
    auth.loggedIn = false;
    auth.checking = false;
    return false;
  }
}

/** 登录页提交访问密码 */
export async function gateLogin(token: string): Promise<void> {
  const res = await fetch("/api/auth-check", { headers: { Authorization: "Bearer " + token } });
  if (!res.ok) {
    let m = await res.text();
    try {
      m = JSON.parse(m).error || m;
    } catch {
      // keep raw
    }
    throw new Error(res.status === 401 ? "访问密码错误，请重试" : m || "登录失败");
  }
  setAuthToken(token);
  auth.loggedIn = true;
}

/** 重新拉取云盘登录态 */
export async function refreshLoginStatus(): Promise<LoginStatusResult | null> {
  try {
    const data = await api<LoginStatusResult>("/api/login/status");
    applyLoginStatus(data);
    return data;
  } catch {
    auth.cloudChecked = true;
    return null;
  }
}

export function logout(): void {
  setAuthToken("");
  auth.loggedIn = false;
  auth.ready = false;
  auth.loginInProgress = false;
  auth.cloudConfigured = false;
  auth.cloudChecked = false;
  auth.cloud = null;
  resetWopanUser();
}

// 任何 API 返回 401 时统一踢回登录页
onUnauthorizedCallback(() => {
  if (auth.loggedIn) {
    logout();
  }
});
