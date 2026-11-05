import { api } from "../api/client";
import type { SelectedItem } from "../api/client";
import { showToast } from "./toast";

/** 执行移动/复制到目标目录（由 FolderPicker 确认后调用） */
export async function doMoveCopy(
  action: "move" | "copy",
  items: SelectedItem[],
  targetId: string,
  targetName: string,
  refresh: () => Promise<void>,
): Promise<void> {
  try {
    const dirList = items.filter((it) => it.type === 0).map((it) => it.id);
    const fileList = items.filter((it) => it.type === 1).map((it) => it.id);
    const path = action === "move" ? "/api/move" : "/api/copy";
    await api(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ targetDirId: targetId, dirList, fileList }),
    });
    showToast((action === "move" ? "移动" : "复制") + "成功，目标：" + targetName, "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  }
  await refresh();
}
