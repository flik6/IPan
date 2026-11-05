import type { WopanFile } from "../api/client";

export function formatSize(n: number): string {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return n.toFixed(i ? 1 : 0) + " " + units[i];
}

export function formatSpeed(bytesPerSecond: number): string {
  return formatSize(bytesPerSecond || 0) + "/s";
}

export function formatTime(value: unknown): string {
  const raw = String(value || "");
  if (!raw) return "-";
  if (/^\d{14}$/.test(raw)) {
    return (
      raw.slice(0, 4) +
      "-" +
      raw.slice(4, 6) +
      "-" +
      raw.slice(6, 8) +
      " " +
      raw.slice(8, 10) +
      ":" +
      raw.slice(10, 12) +
      ":" +
      raw.slice(12, 14)
    );
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    date.getFullYear() +
    "-" +
    pad(date.getMonth() + 1) +
    "-" +
    pad(date.getDate()) +
    " " +
    pad(date.getHours()) +
    ":" +
    pad(date.getMinutes()) +
    ":" +
    pad(date.getSeconds())
  );
}

export function fileTime(f: WopanFile): string {
  return formatTime(f.createTime || f.updateTime || f.uploadTime || f.modifyTime || "");
}

export function maskPhone(p: unknown): string {
  const s = String(p || "");
  return s.length >= 7 ? s.slice(0, 3) + "****" + s.slice(-4) : s;
}

export type FileIconKind = "image" | "folder" | "video" | "music" | "archive" | "file";

const IMAGE_EXTS = ["jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "heic", "avif"];
const VIDEO_EXTS = ["mp4", "mkv", "mov", "avi", "flv", "webm", "m4v", "wmv", "mpeg", "mpg"];
const MUSIC_EXTS = ["mp3", "flac", "wav", "aac", "ogg", "wma", "m4a", "ape", "alac", "opus", "mid", "midi", "aiff"];
const ARCHIVE_EXTS = ["zip", "rar", "7z", "tar", "gz", "bz2", "xz", "iso", "tgz"];

/** 文件类型 → 图标种类（SVG 图标在 FileIcon 组件中按 kind 渲染） */
export function iconKindForFile(file: WopanFile, isDir: boolean): FileIconKind {
  if (isDir) return "folder";
  const name = String(file.name || "").toLowerCase();
  const ext = name.includes(".") ? name.split(".").pop() || "" : "";
  if (IMAGE_EXTS.includes(ext)) return "image";
  if (VIDEO_EXTS.includes(ext)) return "video";
  if (MUSIC_EXTS.includes(ext)) return "music";
  if (ARCHIVE_EXTS.includes(ext)) return "archive";
  return "file";
}

export const IMG_EXT = ["jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "heic", "avif", "tiff", "tif", "ico"];
export const VID_EXT = ["mp4", "mkv", "mov", "avi", "flv", "webm", "m4v", "wmv", "mpeg", "mpg", "ogv", "m3u8", "ts", "rmvb", "rm", "3gp"];
export const AUD_EXT = ["mp3", "flac", "wav", "aac", "ogg", "m4a", "ape", "wma", "opus", "amr", "mid", "midi", "aiff", "alac", "tta", "wv", "caf"];
export const TXT_EXT = ["txt", "md", "csv", "log", "json", "xml", "html", "htm", "text", "yml", "yaml", "ini", "conf", "bat", "sh"];

export function previewKind(name: string): "image" | "video" | "audio" | "text" | "unknown" {
  const ext = (name.split(".").pop() || "").toLowerCase();
  if (IMG_EXT.includes(ext)) return "image";
  if (VID_EXT.includes(ext)) return "video";
  if (AUD_EXT.includes(ext)) return "audio";
  if (TXT_EXT.includes(ext)) return "text";
  return "unknown";
}
