<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NCheckbox, NInput, NModal } from "naive-ui";
import Icon from "./Icon.vue";
import { api, type ShareCreateResult, type ShareMethod } from "../api/client";
import { ui } from "../stores/ui";
import { showToast } from "../stores/toast";

const DAY_OPTIONS = [
  { value: 1, label: "1 天" },
  { value: 7, label: "7 天" },
  { value: 30, label: "30 天" },
  { value: 0, label: "永久" },
];

const METHOD_OPTIONS: Array<{ value: ShareMethod; label: string; desc: string; icon: string }> = [
  {
    value: "official",
    label: "官方分享链接",
    desc: "生成联通云盘分享页，对方在 pan.wo.cn 打开",
    icon: "cloud",
  },
  {
    value: "platform",
    label: "本平台链接 + 提取码",
    desc: "指向当前站点，需输入提取码才能查看",
    icon: "link",
  },
  {
    value: "direct",
    label: "直链下载",
    desc: "直接给出文件下载地址（文件夹给出打包链接）",
    icon: "download",
  },
];

const days = ref(7);
const submitting = ref(false);
const statusText = ref("");

const targets = computed(() => ui.shareTargets);
const title = computed(() => {
  const list = targets.value;
  if (list.length === 1) return list[0].name || "分享";
  return `分享 ${list.length} 个项目`;
});

/** 直链与前两种方式的语义不同：文件夹无法给单个直链，只能打包 */
const hasFolder = computed(() => targets.value.some((it) => it.type === 0));

/** 部分方式创建失败时的说明（其余方式的结果照常展示） */
const failureNotes = computed(() => {
  const errors = ui.shareResult?.errors;
  if (!errors) return [];
  return METHOD_OPTIONS.filter((m) => errors[m.value]).map((m) => ({
    label: m.label,
    message: String(errors[m.value]),
  }));
});

function toggleMethod(value: ShareMethod, checked: boolean): void {
  const set = new Set(ui.shareMethods);
  if (checked) set.add(value);
  else set.delete(value);
  ui.shareMethods = METHOD_OPTIONS.map((m) => m.value).filter((m) => set.has(m));
}

function isSelected(value: ShareMethod): boolean {
  return ui.shareMethods.includes(value);
}

async function submit(): Promise<void> {
  if (!ui.shareMethods.length) {
    ui.shareError = "请至少选择一种分享方式";
    return;
  }
  ui.shareError = "";
  ui.shareLoading = true;
  submitting.value = true;
  ui.shareResult = null;
  statusText.value = "";
  try {
    const result = await api<ShareCreateResult>("/api/share", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        items: targets.value.map((it) => ({ id: it.id, fid: it.fid || it.id, type: it.type, name: it.name })),
        days: days.value,
        methods: ui.shareMethods,
      }),
    });
    ui.shareResult = result;
  } catch (e) {
    ui.shareError = (e as Error).message || "分享创建失败";
  } finally {
    ui.shareLoading = false;
    submitting.value = false;
  }
}

async function copyText(text: string, okMsg: string): Promise<void> {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    statusText.value = okMsg;
    setTimeout(() => {
      if (statusText.value === okMsg) statusText.value = "";
    }, 3000);
  } catch {
    showToast("复制失败，请手动选择文本复制", "error");
  }
}

function copyOfficial(): void {
  const r = ui.shareResult?.official;
  if (!r) return;
  void copyText(r.shareCode ? `${r.shareUrl} 提取码：${r.shareCode}` : r.shareUrl, "已复制官方分享链接");
}

function copyPlatform(): void {
  const r = ui.shareResult?.platform;
  if (!r) return;
  void copyText(`${r.url} 提取码：${r.code}`, "已复制平台分享链接");
}

function close(): void {
  ui.showShareCreate = false;
  ui.shareResult = null;
  ui.shareError = "";
}

/** 打开弹窗时复位到默认状态 */
watch(
  () => ui.showShareCreate,
  (show) => {
    if (!show) return;
    days.value = 7;
    statusText.value = "";
  },
);
</script>

<template>
  <NModal :show="ui.showShareCreate" @update:show="(v: boolean) => (!v && close())">
    <div class="share-box glass-panel">
      <div class="box-head">
        <span class="title" :title="title">{{ title }}</span>
        <button type="button" class="icon-btn" title="关闭" @click="close">
          <Icon name="close" :size="17" />
        </button>
      </div>

      <div class="box-body">
        <!-- 第一步：选择分享方式 -->
        <div class="section">
          <div class="section-title">有效期</div>
          <div class="days-row">
            <button
              v-for="opt in DAY_OPTIONS"
              :key="opt.value"
              type="button"
              class="chip"
              :class="{ active: days === opt.value }"
              @click="days = opt.value"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <div class="section">
          <div class="section-title">分享方式（可多选）</div>
          <div class="method-list">
            <label
              v-for="opt in METHOD_OPTIONS"
              :key="opt.value"
              class="method"
              :class="{ selected: isSelected(opt.value) }"
            >
              <NCheckbox
                :checked="isSelected(opt.value)"
                :disabled="opt.value === 'direct' && hasFolder && false"
                @update:checked="(v: boolean) => toggleMethod(opt.value, v)"
              />
              <span class="method-icon"><Icon :name="opt.icon" :size="17" /></span>
              <span class="method-text">
                <strong>{{ opt.label }}</strong>
                <span class="muted">{{ opt.desc }}</span>
              </span>
            </label>
          </div>
        </div>

        <div v-if="ui.shareError" class="error-text">{{ ui.shareError }}</div>

        <div v-if="failureNotes.length" class="failure-list">
          <div v-for="note in failureNotes" :key="note.label" class="failure-item">
            <Icon name="warning" :size="14" />
            <span><b>{{ note.label }}</b> 创建失败：{{ note.message }}</span>
          </div>
        </div>

        <!-- 第二步：结果 -->
        <div v-if="ui.shareResult" class="results">
          <div v-if="ui.shareResult.official" class="result-card">
            <div class="result-head">
              <strong>官方分享链接</strong>
              <NButton size="tiny" type="primary" secondary @click="copyOfficial">复制</NButton>
            </div>
            <div class="result-line">{{ ui.shareResult.official.shareUrl }}</div>
            <div v-if="ui.shareResult.official.shareCode" class="result-meta">
              提取码：<b>{{ ui.shareResult.official.shareCode }}</b>
            </div>
          </div>

          <div v-if="ui.shareResult.platform" class="result-card">
            <div class="result-head">
              <strong>本平台链接</strong>
              <NButton size="tiny" type="primary" secondary @click="copyPlatform">复制</NButton>
            </div>
            <div class="result-line">{{ ui.shareResult.platform.url }}</div>
            <div class="result-meta">
              提取码：<b>{{ ui.shareResult.platform.code }}</b>
              <span class="muted">
                · {{ ui.shareResult.platform.expiresAt ? new Date(ui.shareResult.platform.expiresAt).toLocaleString("zh-CN") + " 失效" : "永久有效" }}
              </span>
            </div>
          </div>

          <div v-if="ui.shareResult.direct" class="result-card">
            <div class="result-head"><strong>直链</strong></div>
            <div
              v-for="(file, i) in ui.shareResult.direct.files"
              :key="'d' + i"
              class="link-row"
            >
              <NInput :value="file.url" readonly size="small" />
              <NButton size="tiny" @click="copyText(file.url, '已复制直链')">复制</NButton>
            </div>
            <div v-if="ui.shareResult.direct.packUrl" class="link-row top-gap">
              <NInput :value="ui.shareResult.direct.packUrl" readonly size="small" />
              <NButton size="tiny" @click="copyText(ui.shareResult.direct.packUrl, '已复制打包直链')">复制</NButton>
            </div>
            <div v-if="!ui.shareResult.direct.files.length && !ui.shareResult.direct.packUrl" class="muted">
              没有可用的直链
            </div>
          </div>
        </div>
      </div>

      <div class="box-foot">
        <span class="status">{{ statusText }}</span>
        <div class="foot-actions">
          <NButton v-if="ui.shareResult" @click="close">完成</NButton>
          <template v-else>
            <NButton :disabled="submitting" @click="close">取消</NButton>
            <NButton type="primary" :loading="submitting" @click="submit">
              {{ submitting ? "正在创建分享…" : "创建分享" }}
            </NButton>
          </template>
        </div>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.share-box {
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
  width: min(620px, calc(100vw - 40px));
  max-height: 86vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--gh-border);
}
.title {
  font-size: 14px;
  font-weight: 700;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.icon-btn {
  border: 0;
  background: none;
  padding: 4px;
  border-radius: 8px;
  color: var(--gh-ink-soft);
  cursor: pointer;
  display: grid;
  place-items: center;
}
.icon-btn:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
.box-body {
  padding: 16px 18px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--gh-ink-soft);
  margin-bottom: 8px;
}
.days-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.chip {
  height: 30px;
  padding: 0 14px;
  border: 1px solid var(--gh-border);
  border-radius: 999px;
  background: var(--gh-surface);
  color: var(--gh-ink-soft);
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
  transition: all 0.15s ease-out;
}
.chip:hover {
  border-color: var(--gh-border-hover);
  color: var(--gh-primary);
}
.chip.active {
  background: var(--gh-primary-soft);
  border-color: var(--gh-primary);
  color: var(--gh-primary);
  font-weight: 600;
}
.method-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.method {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 11px 12px;
  border: 1px solid var(--gh-border);
  border-radius: 12px;
  cursor: pointer;
  transition: border-color 0.15s ease-out, background 0.15s ease-out;
}
.method:hover {
  border-color: var(--gh-border-hover);
  background: var(--gh-surface-2);
}
.method.selected {
  border-color: var(--gh-primary);
  background: var(--gh-primary-soft);
}
.method-icon {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--gh-surface);
  color: var(--gh-primary);
  flex: 0 0 auto;
}
.method-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.method-text strong {
  font-size: 13px;
}
.method-text .muted {
  font-size: 11.5px;
  line-height: 1.5;
}
.results {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 4px;
  border-top: 1px dashed var(--gh-border);
}
.result-card {
  padding: 12px;
  border: 1px solid var(--gh-border);
  border-radius: 12px;
  background: var(--gh-surface-2);
}
.result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}
.result-head strong {
  font-size: 13px;
}
.result-line {
  font-size: 12px;
  color: var(--gh-ink);
  word-break: break-all;
  line-height: 1.6;
}
.result-meta {
  margin-top: 6px;
  font-size: 12px;
  color: var(--gh-ink-soft);
}
.result-meta b {
  color: var(--gh-primary);
  letter-spacing: 0.08em;
}
.link-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 6px;
}
.top-gap {
  margin-top: 8px;
}
.error-text {
  color: var(--gh-danger);
  font-size: 12px;
}
.failure-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--ac-orange-soft);
  border: 1px solid var(--gh-border);
}
.failure-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--gh-warning);
}
.failure-item svg {
  flex: 0 0 auto;
  margin-top: 2px;
}
.box-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 18px 16px;
  border-top: 1px solid var(--gh-border);
  min-height: 24px;
}
.status {
  font-size: 12px;
  color: var(--gh-success);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.foot-actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 12px;
}
@media (max-width: 560px) {
  .box-body {
    padding: 14px;
  }
  .box-foot {
    padding: 10px 14px 14px;
  }
}
</style>

<style scoped>
.share-box { width: min(640px, calc(100vw - 32px)); max-height: min(86vh, 800px); overflow: hidden; border: 1px solid var(--op-rule); border-radius: 12px; background: #fff; box-shadow: var(--op-shadow-lg); }
.box-head { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 15px 18px; border-bottom: 1px solid var(--op-rule); }
.box-head .title { overflow: hidden; color: var(--op-ink); font-family: var(--op-font-sans); font-size: 19px; font-weight: 600; letter-spacing: -0.02em; text-overflow: ellipsis; white-space: nowrap; }
.icon-btn { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 8px; color: var(--op-ink-soft); background: transparent; cursor: pointer; }
.icon-btn:hover { color: var(--db-blue); background: var(--db-blue-soft); }
.box-body { display: grid; gap: 20px; max-height: calc(86vh - 140px); padding: 18px; overflow: auto; }
.section-title { margin-bottom: 8px; color: var(--op-ink); font-size: 13px; font-weight: 600; }
.days-row { display: flex; flex-wrap: wrap; gap: 6px; }
.chip { min-height: 34px; padding: 0 12px; border: 1px solid var(--op-rule-strong); border-radius: 8px; color: var(--op-ink-soft); background: #fff; font-size: 12px; font-weight: 500; cursor: pointer; }
.chip:hover { border-color: var(--db-blue); color: var(--db-blue); }
.chip.active { border-color: var(--db-blue); color: var(--db-blue); background: var(--db-blue-soft); }
.method-list { display: grid; gap: 8px; }
.method { display: grid; grid-template-columns: auto 34px minmax(0, 1fr); align-items: center; gap: 10px; padding: 11px 12px; border: 1px solid var(--op-rule); border-radius: 8px; cursor: pointer; }
.method.selected { border-color: var(--db-blue); background: var(--db-blue-soft); box-shadow: none; }
.method-icon { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 8px; color: var(--db-blue); background: #fff; }
.method-text { display: grid; gap: 2px; min-width: 0; }
.method-text strong { color: var(--op-ink); font-size: 13px; font-weight: 500; }
.method-text .muted { color: var(--op-ink-soft); font-size: 12px; line-height: 1.4; }
.error-text, .failure-list { padding: 9px 11px; border: 1px solid #fecdca; border-radius: 8px; color: var(--op-danger); background: var(--op-danger-soft); font-size: 12px; }
.failure-list { display: grid; gap: 4px; }
.failure-item { display: flex; align-items: flex-start; gap: 5px; }
.results { display: grid; gap: 10px; }
.result-card { padding: 13px 14px; border: 1px solid var(--op-rule); border-radius: 8px; background: #f9fafb; }
.result-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.result-head strong { color: var(--op-ink); font-size: 13px; font-weight: 600; }
.result-line { margin-top: 7px; overflow-wrap: anywhere; color: var(--op-ink); font-family: var(--op-font-mono); font-size: 11px; line-height: 1.55; }
.result-meta { margin-top: 6px; color: var(--op-ink-soft); font-size: 12px; }
.result-meta b { color: var(--db-blue); }
.link-row { display: flex; gap: 7px; margin-top: 7px; align-items: center; }
.link-row .n-input { flex: 1; }
.box-foot { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 12px 18px; border-top: 1px solid var(--op-rule); }
.status { color: var(--op-success); font-size: 12px; }
.foot-actions { display: flex; gap: 8px; margin-left: auto; }
</style>

<style scoped>
.share-box {
  width: min(680px, calc(100vw - 32px));
  max-height: min(86vh, 820px);
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-top: 3px solid var(--op-accent);
  border-radius: 0;
  background: var(--op-surface);
  box-shadow: var(--op-shadow-lg);
}

.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--op-rule-strong);
}

.box-head .title {
  overflow: hidden;
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 28px;
  font-weight: 500;
  letter-spacing: -0.04em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.icon-btn,
.icon-btn.n-button {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  padding: 0;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-ink-soft);
  background: transparent;
  cursor: pointer;
}

.icon-btn:hover { color: var(--op-accent); border-color: var(--op-accent); }

.box-body {
  display: grid;
  gap: 24px;
  max-height: calc(86vh - 150px);
  padding: 20px;
  overflow: auto;
}

.section-title {
  margin-bottom: 9px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.days-row { display: flex; flex-wrap: wrap; gap: 7px; }

.chip {
  min-height: 36px;
  padding: 0 13px;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}

.chip:hover { color: var(--op-accent); border-color: var(--op-accent); }
.chip.active { color: var(--op-feature-ink); background: var(--op-feature-bg); border-color: var(--op-feature-bg); }

.method-list { display: grid; border-top: 1px solid var(--op-rule); }

.method {
  display: grid;
  grid-template-columns: auto 36px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 13px 2px;
  border-bottom: 1px solid var(--op-rule);
  cursor: pointer;
}

.method.selected { background: var(--op-accent-soft); box-shadow: inset 2px 0 0 var(--op-accent); }

.method-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--op-rule);
  color: var(--op-accent);
  background: var(--op-surface-raised);
}

.method-text { display: grid; gap: 2px; min-width: 0; }
.method-text strong { color: var(--op-ink); font-size: 12px; }
.method-text .muted { color: var(--op-ink-faint); font-size: 10px; line-height: 1.5; }

.error-text,
.failure-list {
  padding: 10px 12px;
  border-left: 2px solid var(--op-danger);
  color: var(--op-danger);
  background: var(--op-danger-soft);
  font-size: 11px;
}

.failure-list { display: grid; gap: 5px; }
.failure-item { display: flex; align-items: flex-start; gap: 6px; }

.results { display: grid; gap: 12px; }

.result-card {
  padding: 15px 16px;
  border: 1px solid var(--op-rule);
  border-left: 2px solid var(--op-accent);
  background: var(--op-surface-raised);
}

.result-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.result-head strong { color: var(--op-ink); font-family: var(--op-font-display); font-size: 20px; font-weight: 500; letter-spacing: -0.02em; }
.result-line { margin-top: 8px; overflow-wrap: anywhere; color: var(--op-ink); font-family: var(--op-font-mono); font-size: 10px; line-height: 1.6; }
.result-meta { margin-top: 7px; color: var(--op-ink-soft); font-size: 10px; }
.result-meta b { color: var(--op-accent); font-family: var(--op-font-mono); }
.link-row { display: flex; gap: 7px; margin-top: 8px; align-items: center; }
.link-row .n-input { flex: 1; }
.top-gap { margin-top: 10px; }

.box-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 20px;
  border-top: 1px solid var(--op-rule-strong);
}

.status { color: var(--op-success); font-size: 11px; }
.foot-actions { display: flex; gap: 8px; margin-left: auto; }

@media (max-width: 560px) {
  .share-box { width: calc(100vw - 20px); }
  .box-head, .box-body, .box-foot { padding-inline: 14px; }
  .box-head .title { font-size: 23px; }
  .method { grid-template-columns: auto 32px minmax(0, 1fr); }
}
</style>
