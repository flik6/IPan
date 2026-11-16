<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton, NCheckbox, NModal } from "naive-ui";
import Icon from "./Icon.vue";
import LoadingSpinner from "./LoadingSpinner.vue";
import ShareRecord from "./ShareRecord.vue";
import WorkspaceEmpty from "./WorkspaceEmpty.vue";
import { api, type OfficialShareRow, type PlatformShareRow, type SharesResult } from "../api/client";
import { showToast } from "../stores/toast";
import { auth } from "../stores/auth";

const loading = ref(false);
const loadError = ref("");
const official = ref<OfficialShareRow[]>([]);
const officialTotal = ref(0);
const officialError = ref("");
const platform = ref<PlatformShareRow[]>([]);
const page = ref(0);
const pageSize = 20;
const tab = ref<"official" | "platform">("official");
const shareSources = computed(() => [
  { key: "official", icon: "cloud", title: "官方分享", description: "联通云盘生成的分享", count: officialTotal.value },
  { key: "platform", icon: "link", title: "平台分享", description: "iPan 生成的链接与提取码", count: platform.value.length },
] as const);

const checkedOfficial = ref<Set<string>>(new Set());
const checkedPlatform = ref<Set<string>>(new Set());

const busy = ref(false);

const officialStatusText: Record<string, string> = {
  "0": "已删除",
  "1": "生效中",
  "2": "已取消",
  "3": "已过期",
};

const channelText: Record<string, string> = {
  "0": "第三方分享",
  "1": "链接分享",
  "2": "微信好友",
  "3": "QQ",
  "4": "微信朋友圈",
  "5": "飞传",
  "6": "QQ空间",
  "7": "保存海报",
  "8": "云盘好友",
};

/** 只有「生效中」的分享可以被取消 */
function canCancel(row: OfficialShareRow): boolean {
  return row.status === "1";
}

const checkedCancellable = computed(() =>
  official.value.filter((row) => checkedOfficial.value.has(row.shareId) && canCancel(row)),
);
const hasMore = computed(() => official.value.length < officialTotal.value);

function formatTs(ts: number | null): string {
  return ts ? new Date(ts).toLocaleString("zh-CN") : "—";
}

async function load(reset = true): Promise<void> {
  if (reset) {
    page.value = 0;
    checkedOfficial.value = new Set();
    checkedPlatform.value = new Set();
  }
  loading.value = true;
  loadError.value = "";
  try {
    const data = await api<SharesResult>(`/api/shares?page=${page.value}&pageSize=${pageSize}`);
    official.value = data.official.list || [];
    officialTotal.value = data.official.total || 0;
    officialError.value = data.official.error || "";
    platform.value = data.platform || [];
  } catch (e) {
    loadError.value = (e as Error).message || "加载分享列表失败";
  } finally {
    loading.value = false;
  }
}

async function loadMore(): Promise<void> {
  if (loading.value || !hasMore.value) return;
  page.value += 1;
  loading.value = true;
  try {
    const data = await api<SharesResult>(`/api/shares?page=${page.value}&pageSize=${pageSize}`);
    official.value = [...official.value, ...(data.official.list || [])];
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

// ---- 选择 ----
function toggleOfficial(id: string, checked: boolean): void {
  const set = new Set(checkedOfficial.value);
  if (checked) set.add(id);
  else set.delete(id);
  checkedOfficial.value = set;
}

function togglePlatform(id: string, checked: boolean): void {
  const set = new Set(checkedPlatform.value);
  if (checked) set.add(id);
  else set.delete(id);
  checkedPlatform.value = set;
}

const allOfficialChecked = computed(
  () => official.value.length > 0 && official.value.every((row) => checkedOfficial.value.has(row.shareId)),
);

function toggleAllOfficial(checked: boolean): void {
  checkedOfficial.value = checked
    ? new Set(official.value.map((row) => row.shareId))
    : new Set();
}

const allPlatformChecked = computed(
  () => platform.value.length > 0 && platform.value.every((row) => checkedPlatform.value.has(row.id)),
);

function toggleAllPlatform(checked: boolean): void {
  checkedPlatform.value = checked ? new Set(platform.value.map((row) => row.id)) : new Set();
}

// ---- 取消 / 删除 ----
const confirmState = ref<{ kind: "cancel" | "delete" | "platform"; ids: string[] } | null>(null);

function askCancelOfficial(rows: OfficialShareRow[]): void {
  const ids = rows.filter(canCancel).map((r) => r.shareId);
  if (!ids.length) {
    showToast("仅「生效中」的分享可以取消", "warning");
    return;
  }
  confirmState.value = { kind: "cancel", ids };
}

function askDeleteOfficial(rows: OfficialShareRow[]): void {
  const ids = rows.map((r) => r.shareId);
  if (!ids.length) return;
  confirmState.value = { kind: "delete", ids };
}

function askDeletePlatform(rows: PlatformShareRow[]): void {
  const ids = rows.map((r) => r.id);
  if (!ids.length) return;
  confirmState.value = { kind: "platform", ids };
}

const confirmText = computed(() => {
  const state = confirmState.value;
  if (!state) return "";
  const count = state.ids.length;
  if (state.kind === "cancel") return `确定取消选中的 ${count} 个官方分享？取消后链接立即失效。`;
  if (state.kind === "delete") return `确定删除选中的 ${count} 条官方分享记录？`;
  return `确定删除选中的 ${count} 个平台分享？链接将立即失效。`;
});

async function runConfirm(): Promise<void> {
  const state = confirmState.value;
  if (!state) return;
  busy.value = true;
  try {
    if (state.kind === "platform") {
      await api("/api/shares/local-delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ids: state.ids }),
      });
      showToast("已删除平台分享", "success");
    } else {
      await api("/api/shares/cancel", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ shareIds: state.ids, operType: state.kind === "delete" ? 2 : 1 }),
      });
      showToast(state.kind === "delete" ? "已删除分享记录" : "已取消分享", "success");
    }
    confirmState.value = null;
    await load();
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    busy.value = false;
  }
}

const platformUrl = (id: string): string => `${location.origin}/s/${id}`;

async function copyPlatform(row: PlatformShareRow): Promise<void> {
  try {
    await navigator.clipboard.writeText(`${platformUrl(row.id)} 提取码：${row.code}`);
    showToast("已复制平台分享链接", "success");
  } catch {
    showToast("复制失败，请手动复制", "error");
  }
}

async function copyOfficial(row: OfficialShareRow): Promise<void> {
  try {
    await navigator.clipboard.writeText(`https://pan.wo.cn/s/${row.shareId}`);
    showToast("已复制官方分享链接", "success");
  } catch {
    showToast("复制失败，请手动复制", "error");
  }
}

const deleteStyle = { style: { color: "var(--gh-danger)" } };
const platformActions = [{ key: "delete", label: "删除分享", props: deleteStyle }];

function officialActions(row: OfficialShareRow) {
  return [
    { key: "cancel", label: "取消分享", disabled: busy.value || !canCancel(row) },
    { key: "delete", label: "删除记录", disabled: busy.value, props: deleteStyle },
  ];
}

function onOfficialAction(row: OfficialShareRow, key: string): void {
  if (key === "cancel") askCancelOfficial([row]);
  if (key === "delete") askDeleteOfficial([row]);
}

onMounted(() => void load());
</script>

<template>
  <div class="shares-view">
    <header class="page-header shares-page-head">
      <div class="page-header-copy">
        <span class="eyebrow">文件分享</span>
        <h1 class="display">分享中心</h1>
        <p class="display-sub">查看和管理联通云盘官方分享，以及由 iPan 创建的链接分享。</p>
      </div>
      <div class="share-ledger-total">
        <span class="mono">{{ officialTotal + platform.length }}</span>
        <span>条有效记录</span>
      </div>
    </header>

    <section class="shares-panel surface-panel" aria-label="分享中心" :aria-busy="loading">
      <header class="panel-bar">
        <div class="share-types" role="group" aria-label="分享类型">
          <button
            v-for="source in shareSources"
            :key="source.key"
            type="button"
            class="source"
            :aria-pressed="tab === source.key"
            @click="tab = source.key"
          >
            <Icon :name="source.icon" :size="17" />
            <span class="source-title">{{ source.title }}</span>
            <span class="source-count">{{ source.count }}</span>
          </button>
        </div>
        <div class="panel-tools">
          <NCheckbox
            v-if="tab === 'official' && official.length"
            :checked="allOfficialChecked"
            :disabled="loading || busy"
            @update:checked="toggleAllOfficial"
          >全选</NCheckbox>
          <NCheckbox
            v-else-if="tab === 'platform' && platform.length"
            :checked="allPlatformChecked"
            :disabled="loading || busy"
            @update:checked="toggleAllPlatform"
          >全选</NCheckbox>
          <button type="button" class="icon-control panel-tool" title="刷新记录" aria-label="刷新记录" :disabled="loading || busy" @click="load(true)">
            <Icon name="refresh" :size="17" />
          </button>
        </div>
      </header>

      <p v-if="loadError" class="share-feedback" role="alert">{{ loadError }}</p>

      <div v-show="tab === 'official'" class="panel-body">
        <div v-if="checkedOfficial.size" class="workspace-bulk panel-bulk">
          <span class="workspace-bulk-count" role="status"><Icon name="check" :size="16" /> 已选 {{ checkedOfficial.size }} 项，可取消 {{ checkedCancellable.length }} 项</span>
          <div class="workspace-bulk-actions">
            <NButton size="small" :disabled="busy || !checkedCancellable.length" @click="askCancelOfficial(checkedCancellable)">取消分享</NButton>
            <NButton size="small" type="error" secondary :disabled="busy" @click="askDeleteOfficial(official.filter((r) => checkedOfficial.has(r.shareId)))">删除记录</NButton>
            <NButton size="small" quaternary @click="checkedOfficial = new Set()">取消选择</NButton>
          </div>
        </div>

        <p v-if="officialError" class="share-feedback" role="alert">官方分享列表加载失败：{{ officialError }}</p>

        <ul v-if="official.length" class="records">
          <ShareRecord
            v-for="row in official"
            :key="row.shareId"
            :title="row.name || '未命名分享'"
            :description="channelText[row.shareChannel] || '链接分享'"
            icon="cloud"
            :checked="checkedOfficial.has(row.shareId)"
            :options="officialActions(row)"
            @update:checked="toggleOfficial(row.shareId, $event)"
            @copy="copyOfficial(row)"
            @action="onOfficialAction(row, $event)"
          >
            <div>
              <dt>状态</dt>
              <dd><span class="status-tag" :class="{ live: canCancel(row) }">{{ officialStatusText[row.status] || "未知状态" }}</span></dd>
            </div>
            <div><dt>分享时间</dt><dd>{{ formatTs(row.shareTime) }}</dd></div>
            <div><dt>失效时间</dt><dd>{{ row.invalidTime ? formatTs(row.invalidTime) : "永久" }}</dd></div>
          </ShareRecord>
        </ul>
        <div v-else-if="loading" class="share-loading"><LoadingSpinner :size="24" label="正在加载官方分享…" /></div>
        <WorkspaceEmpty v-else-if="!officialError && !loadError" icon="cloud" title="暂无官方分享" description="通过联通云盘创建的分享会出现在这里。" />

        <div v-if="hasMore" class="load-more">
          <NButton :loading="loading" @click="loadMore">加载更多</NButton>
        </div>
      </div>

      <div v-show="tab === 'platform'" class="panel-body">
        <div v-if="checkedPlatform.size" class="workspace-bulk panel-bulk">
          <span class="workspace-bulk-count" role="status"><Icon name="check" :size="16" /> 已选 {{ checkedPlatform.size }} 项</span>
          <div class="workspace-bulk-actions">
            <NButton size="small" type="error" secondary :disabled="busy" @click="askDeletePlatform(platform.filter((r) => checkedPlatform.has(r.id)))">删除分享</NButton>
            <NButton size="small" quaternary @click="checkedPlatform = new Set()">取消选择</NButton>
          </div>
        </div>

        <ul v-if="platform.length" class="records">
          <ShareRecord
            v-for="row in platform"
            :key="row.id"
            :title="row.items.length === 1 ? row.items[0].name : `${row.items[0]?.name || '分享'} 等 ${row.items.length} 项`"
            :description="row.items.length + ' 个项目'"
            icon="link"
            :checked="checkedPlatform.has(row.id)"
            :options="platformActions"
            @update:checked="togglePlatform(row.id, $event)"
            @copy="copyPlatform(row)"
            @action="askDeletePlatform([row])"
          >
            <div><dt>提取码</dt><dd><code class="share-code">{{ row.code }}</code></dd></div>
            <div><dt>创建时间</dt><dd>{{ formatTs(row.createdAt) }}</dd></div>
            <div><dt>失效时间</dt><dd>{{ row.expiresAt ? formatTs(row.expiresAt) : "永久" }}</dd></div>
          </ShareRecord>
        </ul>
        <div v-else-if="loading" class="share-loading"><LoadingSpinner :size="24" label="正在加载平台分享…" /></div>
        <WorkspaceEmpty v-else-if="!loadError" icon="link" title="暂无平台分享" description="从文件页创建的平台分享会出现在这里。" />
      </div>
    </section>

    <NModal
      :show="!!confirmState"
      preset="dialog"
      title="确认操作"
      :content="confirmText"
      positive-text="确定"
      negative-text="取消"
      :positive-button-props="{ type: 'error', loading: busy }"
      :negative-button-props="{ disabled: busy }"
      :close-on-esc="!busy"
      :mask-closable="!busy"
      @positive-click="runConfirm"
      @update:show="(visible) => { if (!visible && !busy) confirmState = null; }"
    />
  </div>
</template>

<style scoped>
.shares-page-head { margin-bottom: 24px; }

.share-ledger-total {
  display: grid;
  justify-items: end;
  gap: 2px;
  color: var(--op-ink-faint);
  font-size: 10px;
}

.share-ledger-total .mono {
  color: var(--op-accent);
  font-family: var(--op-font-display);
  font-size: 42px;
  font-weight: 500;
  letter-spacing: -0.06em;
  line-height: 0.9;
}

.shares-panel {
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-radius: 0;
  background: var(--op-surface);
}

.shares-panel > .panel-bar {
  display: flex;
  align-items: stretch;
  justify-content: space-between;
  gap: 16px;
  min-height: 82px;
  padding: 0;
  border-bottom: 1px solid var(--op-rule-strong);
}

.share-types {
  display: grid;
  grid-template-columns: repeat(2, minmax(200px, 1fr));
  flex: 1;
}

.share-types .source {
  position: relative;
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 15px 20px;
  border: 0;
  border-right: 1px solid var(--op-rule);
  color: var(--op-ink-soft);
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: color 160ms ease, background-color 160ms ease;
}

.share-types .source:hover { color: var(--op-ink); background: var(--op-surface-raised); }

.share-types .source[aria-pressed="true"] {
  color: var(--op-accent);
  background: var(--op-accent-soft);
  box-shadow: inset 0 2px 0 var(--op-accent);
}

.share-types .source svg { color: currentColor; }
.source-title { overflow: hidden; font-size: 13px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }

.source-count {
  color: var(--op-ink-faint);
  font-family: var(--op-font-display);
  font-size: 30px;
  font-weight: 500;
  letter-spacing: -0.04em;
  line-height: 1;
}

.share-types .source[aria-pressed="true"] .source-count { color: var(--op-accent); }

.shares-panel > .panel-bar .panel-tools {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  border-left: 1px solid var(--op-rule);
}

.shares-panel .panel-body { padding: 0; }
.share-feedback { margin: 0; padding: 14px 20px; border-bottom: 1px solid var(--op-rule); color: var(--op-danger); font-size: 12px; }
.shares-panel .workspace-bulk { margin: 12px 16px; border-radius: 0; }
.records { margin: 0; padding: 0; list-style: none; }
.share-loading { display: grid; place-items: center; min-height: 240px; }
.load-more { display: flex; justify-content: center; padding: 18px; border-top: 1px solid var(--op-rule); }

@media (max-width: 760px) {
  .share-ledger-total { display: none; }
  .shares-panel > .panel-bar { flex-direction: column; }
  .share-types { grid-template-columns: 1fr; }
  .share-types .source { border-right: 0; border-bottom: 1px solid var(--op-rule); }
  .shares-panel > .panel-bar .panel-tools { justify-content: flex-end; padding: 10px 14px; border-left: 0; }
}
</style>

<style scoped>
/* 与文件工作台一致：类型切换、工具和记录都放在同一张面板里 */
.shares-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}
.panel-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px;
  padding: 8px 9px 8px 12px;
  border-bottom: 1px solid var(--gh-row-border);
}
.panel-tools { display: flex; align-items: center; gap: 10px; flex: 0 0 auto; }
.panel-tool { width: 40px; height: 40px; border: 1px solid var(--gh-border); background: var(--gh-surface); }
.panel-tool:disabled { opacity: 0.55; cursor: default; }
.panel-body { min-width: 0; }
.panel-bulk { margin: 10px 9px 0; }

.share-types { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; min-width: 0; }
.source {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  min-width: 0;
  padding: 0 12px;
  border: 1px solid var(--gh-border);
  border-radius: var(--gh-radius-pill);
  color: var(--gh-ink-soft);
  background: var(--gh-surface-2);
  font-size: 13px;
  font-weight: 650;
  cursor: pointer;
  transition: color 160ms ease, border-color 160ms ease, background 160ms ease;
}
.source svg { flex: none; }
.source:hover { color: var(--gh-primary); border-color: var(--gh-border-hover); }
.source[aria-pressed="true"] {
  color: var(--gh-primary);
  border-color: var(--gh-border-hover);
  background: var(--gh-primary-soft);
  font-weight: 750;
}
.source-count {
  min-width: 24px;
  padding: 2px 7px;
  border-radius: 999px;
  background: var(--gh-surface);
  font-size: 12px;
  font-weight: 750;
  font-variant-numeric: tabular-nums;
  text-align: center;
}
.source[aria-pressed="true"] .source-count { background: var(--gh-surface-pop); }

.records { list-style: none; margin: 0; padding: 6px; }
.status-tag, .share-code {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--gh-surface-2);
  color: var(--gh-ink-soft);
  line-height: 1.35;
}
.status-tag.live { color: var(--gh-success); background: var(--ac-green-soft); }
.share-code { background: var(--gh-primary-soft); color: var(--gh-primary); font: 700 12px/1.35 var(--gh-mono); }
.share-feedback {
  margin: 10px 12px;
  padding: 11px 14px;
  border: 1px solid var(--gh-danger-border);
  border-radius: var(--gh-radius-control);
  color: var(--gh-danger);
  background: var(--gh-danger-bg);
}
.share-loading, .load-more { display: grid; justify-items: center; padding: 20px; }
.share-loading { padding-block: 48px; }

@media (max-width: 760px) {
  .panel-bar { padding: 8px; }
  .panel-tools { width: 100%; justify-content: space-between; }
  .records { padding: 4px; }
  .source { min-height: 36px; padding: 0 10px; font-size: 12px; }
  .source svg { width: 15px; height: 15px; }
}
</style>

<style scoped>
.shares-view .shares-page-head { margin-bottom: 24px; }

.shares-view .share-ledger-total {
  display: grid;
  justify-items: end;
  gap: 2px;
  color: var(--op-ink-faint);
  font-size: 10px;
}

.shares-view .share-ledger-total .mono {
  color: var(--op-accent);
  font-family: var(--op-font-display);
  font-size: 42px;
  font-weight: 500;
  letter-spacing: -0.06em;
  line-height: 0.9;
}

.shares-view .shares-panel {
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--op-rule-strong);
  border-radius: 0;
  background: var(--op-surface);
}

.shares-view .shares-panel > .panel-bar {
  display: flex;
  align-items: stretch;
  justify-content: space-between;
  gap: 16px;
  min-height: 82px;
  padding: 0;
  border-bottom: 1px solid var(--op-rule-strong);
}

.shares-view .share-types {
  display: grid;
  grid-template-columns: repeat(2, minmax(200px, 1fr));
  flex: 1;
  gap: 0;
}

.shares-view .share-types .source {
  position: relative;
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-width: 0;
  min-height: 82px;
  padding: 15px 20px;
  border: 0;
  border-right: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 13px;
  text-align: left;
}

.shares-view .share-types .source:hover {
  color: var(--op-ink);
  background: var(--op-surface-raised);
  border-color: var(--op-rule);
}

.shares-view .share-types .source[aria-pressed="true"] {
  color: var(--op-accent);
  background: var(--op-accent-soft);
  box-shadow: inset 0 2px 0 var(--op-accent);
}

.shares-view .share-types .source svg { width: 18px; height: 18px; color: currentColor; }
.shares-view .source-title { overflow: hidden; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }

.shares-view .source-count {
  padding: 0;
  color: var(--op-ink-faint);
  background: transparent;
  font-family: var(--op-font-display);
  font-size: 30px;
  font-weight: 500;
  letter-spacing: -0.04em;
  line-height: 1;
}

.shares-view .shares-panel > .panel-bar .panel-tools {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  border-left: 1px solid var(--op-rule);
}

.shares-view .panel-body { padding: 0; }
.shares-view .share-feedback { margin: 0; padding: 14px 20px; border-bottom: 1px solid var(--op-rule); color: var(--op-danger); font-size: 12px; }
.shares-view .workspace-bulk { margin: 12px 16px; border-radius: 0; }
.shares-view .records { margin: 0; padding: 0; }
.shares-view .share-loading { display: grid; place-items: center; min-height: 240px; padding: 0; }
.shares-view .load-more { display: flex; justify-content: center; padding: 18px; border-top: 1px solid var(--op-rule); }

@media (max-width: 760px) {
  .shares-view .share-ledger-total { display: none; }
  .shares-view .shares-panel > .panel-bar { flex-direction: column; min-height: 0; }
  .shares-view .share-types { grid-template-columns: 1fr; }
  .shares-view .share-types .source { min-height: 66px; padding: 11px 14px; border-right: 0; border-bottom: 1px solid var(--op-rule); }
  .shares-view .shares-panel > .panel-bar .panel-tools { justify-content: flex-end; padding: 10px 14px; border-left: 0; }
}
</style>

<style scoped>
.shares-view .shares-page-head {
  margin-bottom: 18px;
  padding-bottom: 0;
  animation: op-rise 240ms ease-out both;
}

.shares-view .share-ledger-total { display: none; }

.shares-view .shares-panel {
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--op-rule);
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--op-shadow-sm);
}

.shares-view .shares-panel > .panel-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 56px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--op-rule);
}

.shares-view .share-types {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
}

.shares-view .share-types .source {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  padding: 0 11px;
  border: 0;
  border-radius: 7px;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 13px;
  text-align: left;
}

.shares-view .share-types .source:hover { color: var(--db-blue); background: var(--db-blue-soft); border-color: transparent; }
.shares-view .share-types .source[aria-pressed="true"] { color: var(--db-blue); background: var(--db-blue-soft); box-shadow: none; font-weight: 600; }
.shares-view .share-types .source svg { width: 18px; height: 18px; color: currentColor; }
.shares-view .source-title { font-size: 13px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; overflow: hidden; }

.shares-view .source-count {
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 999px;
  color: var(--op-ink-soft);
  background: var(--op-surface-muted);
  font-family: var(--op-font-sans);
  font-size: 11px;
  font-weight: 600;
  line-height: 20px;
  text-align: center;
}

.shares-view .share-types .source[aria-pressed="true"] .source-count { color: var(--db-blue); background: #fff; }

.shares-view .shares-panel > .panel-bar .panel-tools { display: flex; align-items: center; gap: 8px; padding: 0; border-left: 0; }
.shares-view .panel-body { padding: 0; }
.shares-view .share-feedback { margin: 0; padding: 12px 16px; border-bottom: 1px solid var(--op-rule); color: var(--op-danger); background: var(--op-danger-soft); font-size: 12px; }
.shares-view .workspace-bulk { margin: 10px 12px; border-radius: 8px; }
.shares-view .records { margin: 0; padding: 0; list-style: none; }
.shares-view .share-loading { display: grid; place-items: center; min-height: 220px; padding: 0; }
.shares-view .load-more { display: flex; justify-content: center; padding: 14px; border-top: 1px solid var(--op-rule); }

@media (max-width: 760px) {
  .shares-view .shares-panel { border-right: 0; border-left: 0; border-radius: 0; }
  .shares-view .shares-panel > .panel-bar { flex-direction: column; align-items: stretch; }
  .shares-view .share-types { display: grid; grid-template-columns: 1fr 1fr; }
  .shares-view .share-types .source { justify-content: center; }
  .shares-view .shares-panel > .panel-bar .panel-tools { justify-content: flex-end; }
}
</style>
