<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton, NCheckbox, NModal, NTag } from "naive-ui";
import Icon from "./Icon.vue";
import LoadingSpinner from "./LoadingSpinner.vue";
import { api, type OfficialShareRow, type PlatformShareRow, type SharesResult } from "../api/client";
import { showToast } from "../stores/toast";

const loading = ref(false);
const loadError = ref("");
const official = ref<OfficialShareRow[]>([]);
const officialTotal = ref(0);
const officialError = ref("");
const platform = ref<PlatformShareRow[]>([]);
const page = ref(0);
const pageSize = 20;
const tab = ref<"official" | "platform">("official");

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

function formatTs(ts: number | null, suffix = ""): string {
  if (!ts) return suffix ? `永久${suffix}` : "-";
  return new Date(ts).toLocaleString() + suffix;
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

onMounted(() => void load());
</script>

<template>
  <div class="shares-view">
    <header class="page-head">
      <div>
        <h1>分享中心</h1>
        <p class="muted">管理官方分享链接与本平台分享，可单独或批量取消。</p>
      </div>
      <div class="head-actions">
        <NButton size="small" :loading="loading" @click="load(true)">
          <template #icon><Icon name="refresh" :size="15" /></template>
          刷新
        </NButton>
      </div>
    </header>

    <div class="tabs" role="tablist">
      <button
        type="button"
        class="tab"
        :class="{ active: tab === 'official' }"
        @click="tab = 'official'"
      >
        官方分享
        <span class="badge">{{ officialTotal }}</span>
      </button>
      <button
        type="button"
        class="tab"
        :class="{ active: tab === 'platform' }"
        @click="tab = 'platform'"
      >
        平台分享
        <span class="badge">{{ platform.length }}</span>
      </button>
    </div>

    <div v-if="loadError" class="error-banner">{{ loadError }}</div>

    <!-- 官方分享 -->
    <section v-show="tab === 'official'" class="card">
      <div v-if="checkedOfficial.size" class="bulk-bar">
        <span class="muted">已选择 {{ checkedOfficial.size }} 项（可取消 {{ checkedCancellable.length }} 项）</span>
        <div class="bulk-actions">
          <NButton size="small" :disabled="busy || !checkedCancellable.length" @click="askCancelOfficial(checkedCancellable)">
            取消分享
          </NButton>
          <NButton size="small" type="error" secondary :disabled="busy" @click="askDeleteOfficial(official.filter((r) => checkedOfficial.has(r.shareId)))">
            删除记录
          </NButton>
          <NButton size="small" @click="checkedOfficial = new Set()">取消选择</NButton>
        </div>
      </div>

      <div v-if="officialError" class="inline-hint">
        官方分享列表加载失败：{{ officialError }}
      </div>

      <table v-if="official.length">
        <thead>
          <tr>
            <th class="col-check">
              <NCheckbox :checked="allOfficialChecked" @update:checked="toggleAllOfficial" />
            </th>
            <th>文件</th>
            <th class="col-mid">分享方式</th>
            <th class="col-mid">状态</th>
            <th class="col-time">分享时间</th>
            <th class="col-time">失效时间</th>
            <th class="col-ops">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in official" :key="row.shareId">
            <td class="col-check">
              <NCheckbox
                :checked="checkedOfficial.has(row.shareId)"
                @update:checked="(v: boolean) => toggleOfficial(row.shareId, v)"
              />
            </td>
            <td class="col-name" :title="row.name">{{ row.name }}</td>
            <td class="col-mid">{{ channelText[row.shareChannel] || "链接分享" }}</td>
            <td class="col-mid">
              <NTag size="small" :bordered="false" :type="canCancel(row) ? 'success' : 'default'">
                {{ officialStatusText[row.status] || "生效中" }}
              </NTag>
            </td>
            <td class="col-time">{{ formatTs(row.shareTime) }}</td>
            <td class="col-time">{{ formatTs(row.invalidTime, "失效") }}</td>
            <td class="col-ops">
              <div class="row-actions">
                <NButton size="tiny" quaternary :disabled="!canCancel(row)" @click="askCancelOfficial([row])">
                  取消
                </NButton>
                <NButton size="tiny" quaternary @click="copyOfficial(row)">复制</NButton>
                <NButton size="tiny" quaternary type="error" @click="askDeleteOfficial([row])">删除</NButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="loading" class="empty">
        <LoadingSpinner :size="24" label="正在加载官方分享…" />
      </div>
      <div v-else class="empty muted">暂无官方分享</div>

      <div v-if="hasMore" class="load-more">
        <NButton size="small" :loading="loading" @click="loadMore">加载更多</NButton>
      </div>
    </section>

    <!-- 平台分享 -->
    <section v-show="tab === 'platform'" class="card">
      <div v-if="checkedPlatform.size" class="bulk-bar">
        <span class="muted">已选择 {{ checkedPlatform.size }} 项</span>
        <div class="bulk-actions">
          <NButton
            size="small"
            type="error"
            secondary
            :disabled="busy"
            @click="askDeletePlatform(platform.filter((r) => checkedPlatform.has(r.id)))"
          >
            删除分享
          </NButton>
          <NButton size="small" @click="checkedPlatform = new Set()">取消选择</NButton>
        </div>
      </div>

      <table v-if="platform.length">
        <thead>
          <tr>
            <th class="col-check">
              <NCheckbox :checked="allPlatformChecked" @update:checked="toggleAllPlatform" />
            </th>
            <th>内容</th>
            <th class="col-mid">提取码</th>
            <th class="col-time">创建时间</th>
            <th class="col-time">失效时间</th>
            <th class="col-ops">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in platform" :key="row.id">
            <td class="col-check">
              <NCheckbox
                :checked="checkedPlatform.has(row.id)"
                @update:checked="(v: boolean) => togglePlatform(row.id, v)"
              />
            </td>
            <td class="col-name" :title="row.items.map((i) => i.name).join(', ')">
              {{ row.items.length === 1 ? row.items[0].name : `${row.items[0]?.name || "分享"} 等 ${row.items.length} 项` }}
            </td>
            <td class="col-mid"><b class="code">{{ row.code }}</b></td>
            <td class="col-time">{{ formatTs(row.createdAt) }}</td>
            <td class="col-time">{{ row.expiresAt ? formatTs(row.expiresAt) : "永久" }}</td>
            <td class="col-ops">
              <div class="row-actions">
                <NButton size="tiny" quaternary @click="copyPlatform(row)">复制</NButton>
                <NButton size="tiny" quaternary type="error" @click="askDeletePlatform([row])">删除</NButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="loading" class="empty">
        <LoadingSpinner :size="24" label="正在加载平台分享…" />
      </div>
      <div v-else class="empty muted">暂无平台分享</div>
    </section>

    <NModal
      :show="!!confirmState"
      preset="dialog"
      title="确认操作"
      :content="confirmText"
      positive-text="确定"
      negative-text="取消"
      @positive-click="runConfirm"
      @negative-click="confirmState = null"
      @close="confirmState = null"
    />
  </div>
</template>

<style scoped>
.shares-view {
  max-width: 1160px;
  margin: 0 auto;
}
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.page-head h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 750;
}
.page-head p {
  margin: 6px 0 0;
  font-size: 13px;
}
.tabs {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  margin-bottom: 14px;
  background: var(--gh-surface-2);
  border: 1px solid var(--gh-border);
  border-radius: 11px;
}
.tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 14px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--gh-ink-soft);
  font-size: 13px;
  font-family: inherit;
  cursor: pointer;
  transition: all 0.15s ease-out;
}
.tab:hover {
  color: var(--gh-ink);
}
.tab.active {
  background: var(--gh-surface);
  color: var(--gh-primary);
  font-weight: 600;
  box-shadow: var(--gh-shadow-sm);
}
.badge {
  min-width: 18px;
  height: 17px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  font-size: 11px;
  line-height: 17px;
  text-align: center;
}
.card {
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 16px;
  box-shadow: var(--gh-shadow-sm);
  overflow: hidden;
}
.bulk-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: var(--gh-primary-soft);
  border-bottom: 1px solid var(--gh-border-hover);
  flex-wrap: wrap;
}
.bulk-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
  flex-wrap: wrap;
}
table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  table-layout: fixed;
}
thead th {
  background: var(--gh-surface-2);
  color: var(--gh-ink-soft);
  font-weight: 600;
  font-size: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--gh-border);
  text-align: left;
}
tbody td {
  padding: 11px 14px;
  border-bottom: 1px solid var(--gh-row-border);
  font-size: 13px;
  vertical-align: middle;
}
tbody tr:last-child td {
  border-bottom: 0;
}
tbody tr:hover td {
  background: var(--gh-surface-2);
}
.col-check {
  width: 44px;
}
.col-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.col-mid {
  width: 110px;
}
.col-time {
  width: 160px;
  white-space: nowrap;
  color: var(--gh-ink-soft);
  font-size: 12px;
}
.col-ops {
  width: 190px;
}
.row-actions {
  display: flex;
  gap: 2px;
}
.code {
  color: var(--gh-primary);
  letter-spacing: 0.08em;
}
.empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
  font-size: 13px;
}
.load-more {
  display: flex;
  justify-content: center;
  padding: 12px;
  border-top: 1px solid var(--gh-border);
}
.error-banner {
  margin-bottom: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--gh-danger-bg);
  border: 1px solid var(--gh-danger-border);
  color: var(--gh-danger);
  font-size: 12px;
}
.inline-hint {
  padding: 10px 14px;
  font-size: 12px;
  color: var(--gh-warning);
  border-bottom: 1px solid var(--gh-border);
}
.muted {
  color: var(--gh-ink-soft);
  font-size: 12px;
}
@media (max-width: 900px) {
  .col-mid,
  .col-time {
    display: none;
  }
  .col-ops {
    width: 150px;
  }
  table {
    table-layout: auto;
  }
}
</style>
