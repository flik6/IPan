<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton, NInput } from "naive-ui";
import AuthLayout from "./AuthLayout.vue";
import Icon from "./Icon.vue";
import LoadingSpinner from "./LoadingSpinner.vue";
import FileIcon from "./FileIcon.vue";
import { formatSize, fileTime, iconKindForFile } from "../utils/format";
import type { WopanFile } from "../api/client";

interface PublicShareItem {
  id: string;
  fid: string;
  type: number;
  name: string;
  size?: number;
}

interface PublicShareMeta {
  id: string;
  items: PublicShareItem[];
  createdAt: number;
  expiresAt: number | null;
}

/** 从 /s/<id> 读取分享 ID */
const shareId = computed(() => {
  const m = location.pathname.match(/^\/s\/([^/]+)\/?$/);
  return m ? m[1] : "";
});

const loading = ref(true);
const meta = ref<PublicShareMeta | null>(null);
const loadError = ref("");
const code = ref("");
const verified = ref(false);
const verifying = ref(false);
const codeError = ref("");

const asFile = (it: PublicShareItem): WopanFile => ({
  id: it.id,
  fid: it.fid,
  name: it.name,
  size: it.size,
  type: it.type,
});

/** 未通过提取码校验前不暴露文件名，只显示项目数量 */
const totalLabel = computed(() => {
  const items = meta.value?.items || [];
  if (!verified.value) return `共 ${items.length} 个项目`;
  if (items.length === 1) return items[0].name;
  return `${items[0]?.name || "分享"} 等 ${items.length} 个文件`;
});

const expireLabel = computed(() => {
  const ts = meta.value?.expiresAt;
  return ts ? `${new Date(ts).toLocaleString()} 失效` : "永久有效";
});

async function load(): Promise<void> {
  if (!shareId.value) {
    loadError.value = "分享链接无效";
    loading.value = false;
    return;
  }
  try {
    const res = await fetch(`/api/public/share?id=${encodeURIComponent(shareId.value)}`);
    if (!res.ok) {
      const msg = await res.json().catch(() => ({}));
      throw new Error(msg.error || "分享不存在或已失效");
    }
    meta.value = (await res.json()) as PublicShareMeta;
    // 提取码通过 URL 传入时（复制链接分享时常见），自动校验一次
    const fromUrl = new URLSearchParams(location.search).get("code") || "";
    if (fromUrl) {
      code.value = fromUrl;
      await verify();
    }
  } catch (e) {
    loadError.value = (e as Error).message || "加载分享失败";
  } finally {
    loading.value = false;
  }
}

async function verify(): Promise<void> {
  codeError.value = "";
  if (!code.value.trim()) {
    codeError.value = "请输入提取码";
    return;
  }
  verifying.value = true;
  try {
    const res = await fetch("/api/public/share/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: shareId.value, code: code.value.trim() }),
    });
    if (!res.ok) {
      const msg = await res.json().catch(() => ({}));
      throw new Error(msg.error || "提取码不正确");
    }
    verified.value = true;
  } catch (e) {
    codeError.value = (e as Error).message || "验证失败";
  } finally {
    verifying.value = false;
  }
}

/** 下载链接带上提取码，服务端会再校验一次 */
function downloadHref(fid?: string): string {
  const p = new URLSearchParams({ id: shareId.value, code: code.value.trim() });
  if (fid) p.set("fid", fid);
  return (fid ? "/api/public/share/file?" : "/api/public/share/download?") + p.toString();
}

onMounted(() => void load());
</script>

<template>
  <AuthLayout title="分享的文件" subtitle="由 iPan 生成的分享链接">
    <div v-if="loading" class="center">
      <LoadingSpinner :size="26" label="正在加载分享…" />
    </div>

    <div v-else-if="loadError" class="center">
      <div class="error-icon"><Icon name="warning" :size="26" /></div>
      <p class="error-text">{{ loadError }}</p>
    </div>

    <div v-else-if="meta" class="share-body">
      <div class="share-head">
        <div class="head-icon"><Icon name="share" :size="20" /></div>
        <div class="head-text">
          <strong>{{ totalLabel }}</strong>
          <span class="muted">{{ meta.items.length }} 个项目 · {{ expireLabel }}</span>
        </div>
      </div>

      <!-- 未验证：只展示提取码表单，不泄露文件列表 -->
      <form v-if="!verified" class="code-form" @submit.prevent="verify">
        <label class="label" for="share-code">提取码</label>
        <NInput
          id="share-code"
          v-model:value="code"
          size="large"
          placeholder="请输入 4 位提取码"
          maxlength="8"
          :disabled="verifying"
          @keydown.enter.prevent="verify"
        >
          <template #prefix><Icon name="shield" :size="17" /></template>
        </NInput>
        <div v-if="codeError" class="field-error">{{ codeError }}</div>
        <NButton type="primary" size="large" block attr-type="submit" :loading="verifying" class="submit">
          验证并查看
        </NButton>
      </form>

      <!-- 已验证：文件列表 + 下载 -->
      <template v-else>
        <ul class="file-list">
          <li v-for="it in meta.items" :key="it.fid + it.name" class="file-row">
            <FileIcon :kind="iconKindForFile(asFile(it), it.type === 0)" />
            <div class="file-meta">
              <span class="file-name" :title="it.name">{{ it.name }}</span>
              <span class="muted small">
                <template v-if="it.type === 0">文件夹</template>
                <template v-else>{{ formatSize(Number(it.size || 0)) }}</template>
                <template v-if="fileTime(asFile(it)) !== '-'"> · {{ fileTime(asFile(it)) }}</template>
              </span>
            </div>
            <a
              v-if="it.type !== 0"
              class="file-download"
              :href="downloadHref(it.fid)"
              rel="noreferrer"
              title="下载"
            >
              <Icon name="download" :size="17" />
            </a>
            <span v-else class="muted small pack-hint">打包下载</span>
          </li>
        </ul>

        <a class="download-all" :href="downloadHref()" rel="noreferrer">
          <NButton type="primary" size="large" block>
            <template #icon><Icon name="download" :size="17" /></template>
            下载全部（zip 打包）
          </NButton>
        </a>
      </template>
    </div>

    <template #footer>
      <span class="muted tiny">分享由 iPan 生成 · 请勿用于侵权内容</span>
    </template>
  </AuthLayout>
</template>

<style scoped>
.center {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px 0;
}
.error-icon {
  width: 56px;
  height: 56px;
  border-radius: 16px;
  display: grid;
  place-items: center;
  background: #fff0f1;
  color: var(--gh-danger);
}
.error-text {
  margin: 0;
  font-size: 13px;
  color: var(--gh-danger);
  text-align: center;
}
.share-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.share-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.head-icon {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  flex: 0 0 auto;
}
.head-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.head-text strong {
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.head-text .muted {
  font-size: 12px;
}
.code-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.label {
  font-size: 13px;
  font-weight: 600;
}
.field-error {
  font-size: 12px;
  color: var(--gh-danger);
}
.submit {
  height: 42px;
  border-radius: 12px;
  font-weight: 600;
}
.file-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--gh-border);
  border-radius: 12px;
  overflow: hidden;
}
.file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid #eef0f9;
}
.file-row:last-child {
  border-bottom: 0;
}
.file-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.file-name {
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-download {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--gh-border);
  border-radius: 9px;
  color: var(--gh-ink-soft);
  flex: 0 0 auto;
  transition: all 0.15s ease-out;
}
.file-download:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  border-color: #dde2fc;
}
.pack-hint {
  flex: 0 0 auto;
}
.download-all {
  display: block;
  text-decoration: none;
}
.muted {
  color: var(--gh-ink-soft);
}
.small {
  font-size: 11.5px;
}
.tiny {
  font-size: 11px;
}
</style>
