import { reactive } from "vue";
import { api, type AppSettingsResult } from "../api/client";
import { load as loadFiles } from "./files";

interface SettingsState {
  /** 是否显示隐藏文件（以 . 开头） */
  showHidden: boolean;
  /** 签名密钥是否已配置（未配置时服务端会随机生成一个） */
  signParamConfigured: boolean;
  webdavConfigured: boolean;
  webdavUsername: string;
  webdavDirectDownload: boolean;
  loaded: boolean;
}

export const settings = reactive<SettingsState>({
  showHidden: false,
  signParamConfigured: false,
  webdavConfigured: false,
  webdavUsername: "",
  webdavDirectDownload: true,
  loaded: false,
});

/** 从服务端读取完整设置（设置页使用） */
export async function loadSettings(): Promise<AppSettingsResult | null> {
  try {
    const data = await api<AppSettingsResult>("/api/settings");
    applySettings(data);
    return data;
  } catch {
    return null;
  }
}

export function applySettings(data: Partial<AppSettingsResult>): void {
  if (data.showHidden !== undefined) settings.showHidden = !!data.showHidden;
  if (data.signParamConfigured !== undefined) settings.signParamConfigured = !!data.signParamConfigured;
  if (data.webdavConfigured !== undefined) settings.webdavConfigured = !!data.webdavConfigured;
  if (data.webdavUsername !== undefined) settings.webdavUsername = data.webdavUsername;
  if (data.webdavDirectDownload !== undefined) settings.webdavDirectDownload = !!data.webdavDirectDownload;
  settings.loaded = true;
}

/** 切换隐藏文件显隐，成功后重新加载当前目录 */
export async function setShowHidden(value: boolean): Promise<boolean> {
  const prev = settings.showHidden;
  settings.showHidden = value;
  try {
    await api("/api/settings/show-hidden", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ showHidden: value }),
    });
    void loadFiles();
    return value;
  } catch (e) {
    settings.showHidden = prev;
    throw e;
  }
}

/** 切换 WebDAV 下载直连 */
export async function setWebdavDirectDownload(value: boolean): Promise<boolean> {
  const prev = settings.webdavDirectDownload;
  settings.webdavDirectDownload = value;
  try {
    await api("/api/settings/webdav/direct-download", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ directDownload: value }),
    });
    return value;
  } catch (e) {
    settings.webdavDirectDownload = prev;
    throw e;
  }
}
