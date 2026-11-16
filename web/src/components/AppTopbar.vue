<script setup lang="ts">
import { computed, h } from "vue";
import { NAvatar, NDropdown } from "naive-ui";
import BrandLogo from "./BrandLogo.vue";
import Icon from "./Icon.vue";
import WorkspaceNav from "./WorkspaceNav.vue";
import { auth, logout } from "../stores/auth";
import { wopanUser } from "../composables/useWopanUser";
import { goRoot, resetToRoot, setView } from "../stores/files";
import { ui } from "../stores/ui";
import { formatSize, maskPhone } from "../utils/format";

const displayName = computed(() => wopanUser.nickName || maskPhone(wopanUser.phone) || "本机访问");
const avatarSrc = computed(() => (wopanUser.loaded && !wopanUser.failed ? wopanUser.avatarUrl : ""));
const usagePercent = computed(() =>
  wopanUser.totalSize ? Math.min(100, Math.max(0, (wopanUser.usedSize / wopanUser.totalSize) * 100)) : 0,
);
const usedLabel = computed(() => (wopanUser.loaded ? formatSize(wopanUser.usedSize) : "—"));
const totalLabel = computed(() => (wopanUser.loaded ? formatSize(wopanUser.totalSize) : "—"));

const userOptions = computed(() => [
  {
    key: "head",
    type: "render",
    render: () =>
      h("div", { class: "user-dropdown-head" }, [
        h("strong", displayName.value),
        h("span", auth.cloudConfigured ? "联通云盘已连接" : "尚未绑定云盘"),
      ]),
  },
  { key: "divider", type: "divider" },
  ...(auth.cloudConfigured ? [{ key: "profile", label: "账号资料" }] : []),
  { key: "settings", label: "设置" },
  { key: "logout", label: "退出登录", props: { style: { color: "var(--op-danger)" } } },
]);

function onUserSelect(key: string): void {
  if (key === "profile") {
    ui.settingsSection = "account";
    setView("settings");
  }
  if (key === "settings") {
    ui.settingsSection = "appearance";
    setView("settings");
  }
  if (key === "logout") {
    logout();
    resetToRoot();
    ui.view = "files";
    ui.fileSearch = "";
  }
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-top">
      <button type="button" class="brand-button" aria-label="iPan 文件首页" @click="goRoot">
        <BrandLogo subtitle="个人云盘" />
      </button>
    </div>

    <div class="sidebar-label">工作区</div>
    <WorkspaceNav />

    <div class="sidebar-spacer" />

    <section v-if="auth.cloudConfigured" class="storage-card" aria-label="存储空间">
      <div class="storage-head">
        <span>存储空间</span>
        <span class="storage-percent">{{ Math.round(usagePercent) }}%</span>
      </div>
      <div class="storage-value">
        <strong>{{ usedLabel }}</strong>
        <span>/ {{ totalLabel }}</span>
      </div>
      <div class="storage-track" aria-hidden="true">
        <span :style="{ width: usagePercent + '%' }" />
      </div>
      <div class="storage-foot">
        <span>{{ auth.cloud?.enabled ? "自动同步已开启" : "自动同步已关闭" }}</span>
        <button type="button" class="storage-link" @click="ui.settingsSection = 'cloud'; setView('settings')">
          管理
        </button>
      </div>
    </section>

    <div class="sidebar-actions">
      <button
        v-if="auth.cloudConfigured"
        type="button"
        class="sidebar-action"
        aria-label="解析分享链接"
        @click="ui.showResolve = true"
      >
        <Icon name="link" :size="18" />
        <span>链接解析</span>
      </button>

      <NDropdown :options="userOptions" trigger="click" placement="top-start" @select="onUserSelect">
        <button type="button" class="account-button" :aria-label="'账号菜单，' + displayName" aria-haspopup="menu">
          <NAvatar :src="avatarSrc || undefined" :size="32" class="account-avatar">
            {{ (displayName || "iP").slice(0, 1) }}
          </NAvatar>
          <span class="account-copy">
            <strong>{{ displayName }}</strong>
            <small>{{ auth.cloudConfigured ? maskPhone(wopanUser.phone) || "云盘已连接" : "尚未绑定云盘" }}</small>
          </span>
          <Icon name="chevron-down" :size="14" class="account-chevron" />
        </button>
      </NDropdown>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  position: fixed;
  z-index: 30;
  inset: 0 auto 0 0;
  display: flex;
  flex-direction: column;
  width: var(--op-sidebar);
  padding: 18px 14px 14px;
  border-right: 1px solid var(--op-rule);
  background: #ffffff;
}

.brand-button {
  min-height: 44px;
  padding: 0 6px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.brand-button:hover { background: var(--op-surface-muted); }

.sidebar-label {
  margin: 20px 0 8px;
  padding: 0 11px;
  color: var(--op-ink-faint);
  font-size: 11px;
  font-weight: 600;
}

.sidebar-spacer { flex: 1; min-height: 24px; }

.storage-card {
  padding: 13px;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: var(--op-surface-muted);
}

.storage-head,
.storage-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.storage-head {
  color: var(--op-ink);
  font-size: 13px;
  font-weight: 600;
}

.storage-percent {
  color: var(--db-blue);
  font-size: 12px;
  font-weight: 600;
}

.storage-value {
  display: flex;
  align-items: baseline;
  gap: 5px;
  margin-top: 7px;
}

.storage-value strong {
  color: var(--op-ink);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.storage-value span { color: var(--op-ink-soft); font-size: 11px; }

.storage-track {
  height: 4px;
  margin-top: 9px;
  overflow: hidden;
  border-radius: 999px;
  background: #dfe3e8;
}

.storage-track span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--db-blue);
  transition: width 500ms ease;
}

.storage-foot {
  margin-top: 8px;
  color: var(--op-ink-faint);
  font-size: 11px;
}

.storage-link {
  padding: 0;
  border: 0;
  color: var(--db-blue);
  background: transparent;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}

.storage-link:hover { text-decoration: underline; }

.sidebar-actions {
  display: grid;
  gap: 6px;
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--op-rule);
}

.sidebar-action,
.account-button {
  width: 100%;
  border: 0;
  border-radius: 8px;
  color: var(--op-ink-soft);
  background: transparent;
  cursor: pointer;
  transition: background-color 140ms ease, color 140ms ease;
}

.sidebar-action {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  padding: 0 10px;
  font-size: 13px;
  text-align: left;
}

.sidebar-action:hover,
.account-button:hover { color: var(--db-blue); background: var(--db-blue-soft); }

.account-button {
  display: flex;
  align-items: center;
  gap: 9px;
  min-height: 50px;
  padding: 6px 8px;
  text-align: left;
}

.account-avatar {
  flex: none;
  color: var(--db-blue);
  background: var(--db-blue-soft);
  font-size: 13px;
  font-weight: 600;
}

.account-copy {
  display: grid;
  flex: 1;
  min-width: 0;
  line-height: 1.25;
}

.account-copy strong,
.account-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-copy strong { font-size: 13px; font-weight: 600; }
.account-copy small { margin-top: 2px; color: var(--op-ink-faint); font-size: 11px; }
.account-chevron { flex: none; color: var(--op-ink-faint); }

@media (max-width: 1040px) {
  .sidebar {
    inset: 0 0 auto;
    flex-direction: row;
    align-items: center;
    width: 100%;
    min-height: 62px;
    padding: 9px 16px;
    border-right: 0;
    border-bottom: 1px solid var(--op-rule);
  }

  .sidebar-top { flex: 1; min-width: 0; }
  .sidebar-label,
  .sidebar-spacer,
  .storage-card { display: none; }

  .sidebar-actions {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-top: 0;
    padding-top: 0;
    border-top: 0;
  }

  .sidebar-action {
    width: 38px;
    min-height: 38px;
    justify-content: center;
    padding: 0;
  }

  .sidebar-action span { display: none; }

  .account-button {
    width: auto;
    min-height: 38px;
    padding: 3px;
  }

  .account-copy,
  .account-chevron { display: none; }
}

@media (max-width: 760px) {
  .sidebar { padding-inline: 14px; }
}
</style>

<style>
.user-dropdown-head {
  display: grid;
  gap: 3px;
  max-width: 230px;
  min-width: 170px;
  padding: 10px 14px 7px;
}

.user-dropdown-head strong {
  overflow: hidden;
  color: var(--op-ink);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-dropdown-head span {
  color: var(--op-ink-faint);
  font-size: 11px;
}
</style>
