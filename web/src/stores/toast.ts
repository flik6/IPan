import { reactive } from "vue";

export type ToastType = "info" | "success" | "warning" | "error";

export interface ToastItem {
  id: number;
  text: string;
  type: ToastType;
}

interface ToastState {
  toasts: ToastItem[];
}

export const toast = reactive<ToastState>({
  toasts: [],
});

let nextId = 1;

export function showToast(text: string, type: ToastType = "info"): void {
  if (!text) return;
  const id = nextId++;
  toast.toasts.push({ id, text, type });
  if (toast.toasts.length > 4) toast.toasts.shift();
  setTimeout(() => hideToast(id), 5000);
}

export function hideToast(id: number): void {
  const idx = toast.toasts.findIndex((t) => t.id === id);
  if (idx >= 0) toast.toasts.splice(idx, 1);
}
