import { reactive } from "vue";
import { api, getAuthToken, type UploadSession } from "../api/client";

export type UploadTaskStatus = "pending" | "preparing" | "uploading" | "done" | "error" | "cancelled";

export interface UploadTask {
  id: number;
  file: File;
  name: string;
  size: number;
  uploaded: number;
  confirmedUploaded: number;
  inFlightLoaded: number;
  percent: number;
  partIndex: number;
  totalPart: number;
  status: UploadTaskStatus;
  error: string;
}

interface UploadState {
  tasks: UploadTask[];
  uploading: boolean;
  speed: number;
  panelOpen: boolean;
  fabVisible: boolean;
  cancelling: boolean;
}

export const upload = reactive<UploadState>({
  tasks: [],
  uploading: false,
  speed: 0,
  panelOpen: false,
  fabVisible: false,
  cancelling: false,
});

let uploadController: AbortController | null = null;
let progressTimer: ReturnType<typeof setInterval> | null = null;
let lastTime = 0;
let autoCollapseTimer: ReturnType<typeof setTimeout> | null = null;

export function throwIfUploadCancelled(): void {
  if (upload.cancelling) throw new DOMException("Upload cancelled", "AbortError");
}

function createUploadController(): AbortController {
  uploadController = new AbortController();
  return uploadController;
}

/**
 * 批量上传文件到目录。完成后回调 onSuccess/onError。
 */
export async function uploadFiles(
  filesToUpload: File[],
  directoryId: string,
  opts: { onSuccess?: (count: number) => void; onError?: (msg: string) => void; onFinished?: () => void } = {},
): Promise<void> {
  const pending = Array.from(filesToUpload || []).filter((file) => file && typeof file.slice === "function");
  if (!pending.length) {
    opts.onError?.("没有可上传的文件");
    return;
  }
  if (upload.uploading) {
    opts.onError?.("已有文件正在上传，请稍候");
    return;
  }
  upload.uploading = true;
  upload.cancelling = false;
  upload.speed = 0;
  upload.fabVisible = true;
  if (autoCollapseTimer) {
    clearTimeout(autoCollapseTimer);
    autoCollapseTimer = null;
  }
  lastTime = performance.now();
  startProgressTimer();
  upload.tasks = pending.map((file, index) => ({
    id: index,
    file,
    name: file.name,
    size: file.size,
    uploaded: 0,
    confirmedUploaded: 0,
    inFlightLoaded: 0,
    percent: 0,
    partIndex: 0,
    totalPart: 0,
    status: index === 0 ? "preparing" : "pending",
    error: "",
  }));

  try {
    for (const task of upload.tasks) {
      throwIfUploadCancelled();
      task.status = "preparing";
      await uploadFileInParts(task, directoryId);
      task.uploaded = task.size;
      task.percent = 100;
      task.status = "done";
    }
    opts.onSuccess?.(pending.length);
  } catch (e) {
    const err = e as Error;
    if (upload.cancelling || err?.name === "AbortError") {
      upload.tasks.forEach((task) => {
        if (!["done", "error"].includes(task.status)) task.status = "cancelled";
      });
      opts.onError?.("已放弃此次上传任务");
    } else {
      const activeTask = upload.tasks.find((task) => ["preparing", "uploading"].includes(task.status));
      if (activeTask) {
        activeTask.status = "error";
        activeTask.error = err.message;
      }
      opts.onError?.("上传失败：" + err.message);
    }
  } finally {
    upload.uploading = false;
    upload.cancelling = false;
    uploadController = null;
    stopProgressTimer();
    refreshVisibleProgress();
    upload.speed = 0;
    scheduleAutoCollapse();
    opts.onFinished?.();
  }
}

async function uploadFileInParts(task: UploadTask, directoryId: string): Promise<void> {
  throwIfUploadCancelled();
  const file = task.file;
  const session = await api<UploadSession>("/api/upload/init", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ directoryId, fileName: file.name, fileSize: file.size }),
  });
  createUploadController();
  const partSize = Number(session.partSize);
  const totalPart = Number(session.totalPart);
  if (!Number.isFinite(partSize) || partSize <= 0 || !Number.isInteger(totalPart) || totalPart < 1) {
    throw new Error("Invalid upload session");
  }
  task.totalPart = totalPart;
  task.status = "uploading";
  for (let partIndex = 1; partIndex <= totalPart; partIndex++) {
    throwIfUploadCancelled();
    const start = (partIndex - 1) * partSize;
    const chunk = file.slice(start, Math.min(start + partSize, file.size), file.type || "application/octet-stream");
    const form = new FormData();
    Object.entries(session.fields || {}).forEach(([key, value]) => form.set(key, String(value)));
    form.set("partSize", String(chunk.size));
    form.set("partIndex", String(partIndex));
    form.set("file", chunk, file.name);
    task.partIndex = partIndex;
    task.inFlightLoaded = 0;
    await uploadPartWithRetry(session.uploadUrl, form, partIndex, (loaded) => {
      task.inFlightLoaded = Math.min(chunk.size, loaded);
    });
    task.confirmedUploaded = Math.min(task.size, start + chunk.size);
    task.uploaded = task.confirmedUploaded;
    task.inFlightLoaded = 0;
    task.percent = task.size ? Math.round((task.uploaded / task.size) * 100) : 100;
  }
}

async function uploadPartWithRetry(uploadUrl: string, form: FormData, partIndex: number, onProgress: (loaded: number) => void): Promise<void> {
  let lastError: unknown;
  for (let attempt = 0; attempt <= 2; attempt++) {
    throwIfUploadCancelled();
    try {
      return void (await uploadFormWithProgress(uploadUrl, form, partIndex, onProgress));
    } catch (e) {
      if (upload.cancelling || (e as Error)?.name === "AbortError") throw e;
      lastError = e;
      if (attempt < 2) await sleep(300 * (attempt + 1));
    }
  }
  throw lastError;
}

function uploadFormWithProgress(uploadUrl: string, form: FormData, partIndex: number, onProgress: (loaded: number) => void): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    uploadController = { abort: () => xhr.abort() } as AbortController;
    xhr.open("POST", uploadUrl, true);
    xhr.setRequestHeader("Authorization", "Bearer " + getAuthToken());
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) onProgress(event.loaded);
    };
    xhr.onload = () => {
      if (xhr.status < 200 || xhr.status >= 300) return reject(new Error(xhr.responseText || "HTTP " + xhr.status));
      let result: { code?: string } | string;
      try {
        result = xhr.responseText ? JSON.parse(xhr.responseText) : {};
      } catch {
        result = xhr.responseText;
      }
      if (result && typeof result === "object" && result.code && result.code !== "0000") {
        return reject(new Error("Upload part " + partIndex + " failed: " + JSON.stringify(result)));
      }
      resolve(result);
    };
    xhr.onerror = () => reject(new Error("网络错误，上传分片失败"));
    xhr.onabort = () => reject(new DOMException("Upload cancelled", "AbortError"));
    xhr.send(form);
  });
}

/** 取消当前上传 */
export function cancelUpload(): void {
  if (!upload.uploading) return;
  upload.cancelling = true;
  if (uploadController && typeof uploadController.abort === "function") uploadController.abort();
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ---- 进度/速度计算（每秒刷新） ----

function refreshVisibleProgress(): void {
  const now = performance.now();
  const visibleUploaded = upload.tasks.reduce(
    (sum, task) => sum + Math.min(task.size, (task.confirmedUploaded || 0) + (task.inFlightLoaded || 0)),
    0,
  );
  const totalElapsed = Math.max(1, now - lastTime);
  upload.speed = upload.uploading ? visibleUploaded / (totalElapsed / 1000) : 0;
  upload.tasks.forEach((task) => {
    task.uploaded = Math.min(task.size, (task.confirmedUploaded || 0) + (task.inFlightLoaded || 0));
    task.percent = task.size ? Math.round((task.uploaded / task.size) * 100) : 100;
  });
}

function startProgressTimer(): void {
  stopProgressTimer();
  progressTimer = setInterval(refreshVisibleProgress, 1000);
}

function stopProgressTimer(): void {
  if (progressTimer !== null) {
    clearInterval(progressTimer);
    progressTimer = null;
  }
}

// ---- 面板开合 ----

export function openPanel(): void {
  upload.panelOpen = true;
}

export function closePanel(): void {
  upload.panelOpen = false;
}

export function togglePanel(): void {
  upload.panelOpen = !upload.panelOpen;
}

/** 上传全部结束后自动收起抽屉 */
function scheduleAutoCollapse(): void {
  if (autoCollapseTimer) clearTimeout(autoCollapseTimer);
  autoCollapseTimer = setTimeout(() => {
    if (!upload.uploading) closePanel();
    autoCollapseTimer = null;
  }, 4000);
}
