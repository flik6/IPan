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
  if (!ts) return "永久有效";
  if (ts <= Date.now()) return "已失效";
  return `有效期至 ${new Date(ts).toLocaleString("zh-CN")}`;
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
  <AuthLayout title="分享的文件" subtitle="验证提取码后即可查看和下载文件。" variant="share">
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
          :input-props="{
            id: 'share-code',
            'aria-invalid': !!codeError,
            'aria-describedby': codeError ? 'share-code-error' : undefined,
          }"
          v-model:value="code"
          size="large"
          placeholder="请输入 4 位提取码"
          maxlength="8"
          :disabled="verifying"
          @keydown.enter.prevent="verify"
        >
          <template #prefix><Icon name="shield" :size="17" /></template>
        </NInput>
        <div v-if="codeError" id="share-code-error" class="field-error" role="alert">{{ codeError }}</div>
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
              :aria-label="'下载 ' + it.name"
              title="下载"
            >
              <Icon name="download" :size="17" />
            </a>
            <span v-else class="muted small pack-hint">打包下载</span>
          </li>
        </ul>

        <a class="action-control download-all" :href="downloadHref()" rel="noreferrer">
          <Icon name="download" :size="17" />
          下载全部（zip 打包）
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
  background: var(--gh-danger-bg);
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
  border-bottom: 1px solid var(--gh-row-border);
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
  width: 44px;
  height: 44px;
  border: 1px solid var(--gh-border);
  border-radius: 9px;
  color: var(--gh-ink-soft);
  flex: 0 0 auto;
  transition: all 0.15s ease-out;
}
.file-download:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  border-color: var(--gh-border-hover);
}
.pack-hint {
  flex: 0 0 auto;
}
.download-all {
  width: 100%;
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

<style scoped>
.center { display: grid; justify-items: center; gap: 10px; min-height: 160px; place-content: center; text-align: center; }
.error-icon { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 10px; color: var(--op-danger); background: var(--op-danger-soft); }
.error-text { margin: 0; color: var(--op-ink-soft); font-size: 13px; }
.share-body { display: grid; gap: 20px; }
.share-head { display: flex; align-items: flex-start; gap: 12px; padding-bottom: 16px; border-bottom: 1px solid var(--op-rule); }
.head-icon { display: grid; place-items: center; flex: 0 0 40px; height: 40px; border: 0; border-radius: 9px; color: var(--db-blue); background: var(--db-blue-soft); }
.head-text { display: grid; gap: 3px; min-width: 0; }
.head-text strong { display: block; overflow-wrap: anywhere; color: var(--op-ink); font-family: var(--op-font-sans); font-size: 22px; font-weight: 600; letter-spacing: -0.02em; line-height: 1.3; text-overflow: clip; white-space: normal; }
.head-text .muted { color: var(--op-ink-soft); font-family: var(--op-font-sans); font-size: 12px; letter-spacing: 0; text-transform: none; }
.code-form { display: grid; gap: 10px; padding: 16px; border: 1px solid var(--op-rule); border-radius: 10px; background: #f9fafb; }
.label { color: var(--op-ink); font-size: 12px; font-weight: 600; }
.field-error { color: var(--op-danger); font-size: 12px; }
.submit { border-radius: 8px !important; }
.file-list { display: grid; gap: 0; margin: 0; padding: 0; border: 1px solid var(--op-rule); border-radius: 10px; overflow: hidden; list-style: none; }
.file-row { counter-increment: none; display: flex; align-items: center; gap: 10px; min-height: 58px; padding: 9px 12px; border-bottom: 1px solid var(--op-rule); }
.file-row:last-child { border-bottom: 0; }
.file-row::before { display: none; }
.file-row .file-icon { width: 28px; height: 28px; flex-basis: 28px; }
.file-meta { display: grid; gap: 2px; flex: 1; min-width: 0; }
.file-name { overflow: hidden; color: var(--op-ink); font-size: 13px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.file-meta .small { color: var(--op-ink-faint); font-family: var(--op-font-sans); font-size: 11px; text-transform: none; }
.file-download { display: grid; place-items: center; width: 32px; height: 32px; border: 1px solid var(--op-rule); border-radius: 8px; color: var(--op-ink-soft); background: #fff; }
.file-download:hover { color: var(--db-blue); border-color: var(--db-blue); background: var(--db-blue-soft); }
.pack-hint { color: var(--op-ink-faint); font-size: 11px; }
.download-all { width: 100%; min-height: 44px; border-radius: 8px; color: #fff; background: var(--db-blue); border-color: var(--db-blue); }
.download-all:hover { color: #fff; background: var(--db-blue-hover); border-color: var(--db-blue-hover); }
</style>

<style scoped>
.center { display: grid; justify-items: center; gap: 12px; min-height: 180px; place-content: center; text-align: center; }
.error-icon { display: grid; place-items: center; width: 46px; height: 46px; color: var(--op-danger); background: var(--op-danger-soft); }
.error-text { margin: 0; color: var(--op-ink-soft); font-size: 13px; }

.share-body { display: grid; gap: 24px; }

.share-head {
  display: flex;
  align-items: flex-start;
  gap: 13px;
  padding-bottom: 18px;
  border-bottom: 1px solid var(--op-rule-strong);
}

.head-icon {
  display: grid;
  place-items: center;
  flex: 0 0 42px;
  height: 42px;
  border: 1px solid var(--op-rule);
  color: var(--op-accent);
  background: var(--op-accent-soft);
}

.head-text { display: grid; gap: 4px; min-width: 0; }
.head-text strong {
  display: block;
  overflow-wrap: anywhere;
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: clamp(24px, 4vw, 36px);
  font-weight: 500;
  letter-spacing: -0.04em;
  line-height: 1.1;
  text-overflow: clip;
  white-space: normal;
}

.head-text .muted { color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 11px; letter-spacing: 0.04em; text-transform: uppercase; }

.code-form { display: grid; gap: 12px; padding: 20px; border: 1px solid var(--op-rule); background: var(--op-surface-raised); }
.label { color: var(--op-ink-soft); font-family: var(--op-font-mono); font-size: 11px; font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; }
.field-error { color: var(--op-danger); font-size: 11px; }
.submit { border-radius: 0 !important; }

.file-list {
  counter-reset: shared-file;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--op-rule-strong);
  list-style: none;
}

.file-row {
  counter-increment: shared-file;
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 66px;
  padding: 10px 2px;
  border-bottom: 1px solid var(--op-rule);
}

.file-row::before {
  content: counter(shared-file, decimal-leading-zero);
  flex: 0 0 24px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
}

.file-row .file-icon {
  width: 34px;
  height: 34px;
  flex-basis: 34px;
  padding: 5px;
  border: 1px solid var(--op-rule);
  background: var(--op-surface-raised);
}

.file-meta { display: grid; gap: 3px; flex: 1; min-width: 0; }
.file-name { overflow: hidden; color: var(--op-ink); font-size: 13px; font-weight: 650; text-overflow: ellipsis; white-space: nowrap; }
.file-meta .small { color: var(--op-ink-faint); font-family: var(--op-font-mono); font-size: 11px; text-transform: uppercase; }

.file-download {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border: 1px solid var(--op-rule);
  color: var(--op-ink-soft);
}

.file-download:hover { color: var(--op-accent); border-color: var(--op-accent); background: var(--op-accent-soft); }
.pack-hint { font-family: var(--op-font-mono); font-size: 11px; text-transform: uppercase; }

.download-all {
  width: 100%;
  min-height: 48px;
  border-radius: 0;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
  border-color: var(--op-feature-bg);
}

.download-all:hover { color: var(--op-on-accent); background: var(--op-accent); border-color: var(--op-accent); }
</style>
