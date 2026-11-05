/** Passkey（WebAuthn）客户端封装：支持检测、注册引导与免密登录 */
import { browserSupportsWebAuthn, startAuthentication, startRegistration } from "@simplewebauthn/browser";
import type {
  AuthenticationResponseJSON,
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
  RegistrationResponseJSON,
} from "@simplewebauthn/browser";
import { api, rawApi } from "../api/client";

/** 当前浏览器/环境是否支持 Passkey */
export function passkeySupported(): boolean {
  try {
    return browserSupportsWebAuthn();
  } catch {
    return false;
  }
}

/** 本设备"已注册过 Passkey"的本地标记 */
const ENROLL_FLAG = "wopan.passkeyEnrolled";

/** 本设备是否已注册过 Passkey（localStorage 异常时视为已注册，避免反复打扰用户） */
export function passkeyEnrolled(): boolean {
  try {
    return localStorage.getItem(ENROLL_FLAG) === "1";
  } catch {
    return true;
  }
}

/** 注册本设备的 Passkey：取服务端 options → 浏览器弹窗采集 → 提交校验 */
export async function registerPasskey(): Promise<void> {
  const { options } = await api<{ options: PublicKeyCredentialCreationOptionsJSON }>(
    "/api/passkey/register/options",
    { method: "POST" }
  );
  const attResp: RegistrationResponseJSON = await startRegistration({ optionsJSON: options });
  await api("/api/passkey/register/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ response: attResp }),
  });
  // 注册成功后记录标记，下次登录不再弹出注册引导
  try {
    localStorage.setItem(ENROLL_FLAG, "1");
  } catch {
    // localStorage 不可用时忽略，仅影响下次引导
  }
}

/** 使用 Passkey 免密登录，成功后返回新的访问令牌 */
export async function loginWithPasskey(): Promise<string> {
  const { options } = await rawApi<{ options: PublicKeyCredentialRequestOptionsJSON }>(
    "/api/passkey/auth/options",
    { method: "POST" }
  );
  const authResp: AuthenticationResponseJSON = await startAuthentication({ optionsJSON: options });
  const data = await rawApi<{ token: string }>("/api/passkey/auth/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ response: authResp }),
  });
  return data.token;
}
