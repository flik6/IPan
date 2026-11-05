<script setup lang="ts">
import { computed, h } from "vue";
import { NAvatar, NDropdown, NInput } from "naive-ui";
import BrandLogo from "./BrandLogo.vue";
import Icon from "./Icon.vue";
import { auth, logout } from "../stores/auth";
import { wopanUser } from "../composables/useWopanUser";
import { files, resetToRoot, goUp, canGoUp, goRoot, setView } from "../stores/files";
import { ui } from "../stores/ui";
import { maskPhone } from "../utils/format";

const emit = defineEmits<{ upload: [] }>();

const DEFAULT_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><circle cx="32" cy="32" r="30" fill="#6D7BF5"/><circle cx="32" cy="25" r="11" fill="#fff"/><path d="M12 52c3-11 11-16 20-16s17 5 20 16a30 30 0 0 1-40 0z" fill="#fff"/></svg>',
  );

const avatarSrc = computed(() => (wopanUser.loaded && !wopanUser.failed && wopanUser.avatarUrl) || DEFAULT_AVATAR);
const displayName = computed(() => wopanUser.nickName || maskPhone(wopanUser.phone) || "用户");

const userOptions = computed(() => [
  {
    key: "head",
    type: "render",
    render: () =>
      h("div", { class: "user-dropdown-head" }, [
        h("strong", displayName.value),
        h("span", "已登录"),
      ]),
  },
  { key: "d1", type: "divider" },
  ...(auth.cloudConfigured ? [{ key: "profile", label: "账号信息" }] : []),
  { key: "files", label: "全部文件" },
  { key: "shares", label: "分享中心" },
  { key: "settings", label: "设置" },
  { key: "logout", label: "退出登录", props: { style: { color: "var(--gh-danger)" } } },
]);

function onUserSelect(key: string): void {
  if (key === "profile") {
    ui.showProfileEdit = true;
  } else if (key === "files") {
    goHome();
  } else if (key === "shares") {
    setView("shares");
  } else if (key === "settings") {
    setView("settings");
  } else if (key === "logout") {
    logout();
    resetToRoot();
    ui.view = "files";
    ui.fileSearch = "";
  }
}

function goHome(): void {
  goRoot();
}
</script>

<template>
  <header class="topbar">
    <button type="button" class="brand-btn" title="回到文件" @click="goHome">
      <BrandLogo size="sm" />
    </button>

    <button
      v-if="canGoUp()"
      type="button"
      class="back-btn"
      title="返回上一级"
      aria-label="返回上一级"
      @click="goUp()"
    >
      <Icon name="back" :size="18" />
    </button>

    <span class="divider" />

    <span v-if="ui.view === 'settings'" class="page-title">设置</span>
    <span v-else-if="ui.view === 'shares'" class="page-title">分享中心</span>
    <span v-else class="page-title">{{ files.stack[files.stack.length - 1]?.name || "全部文件" }}</span>

    <div class="tools">
      <NInput
        v-if="ui.view === 'files'"
        v-model:value="ui.fileSearch"
        class="search"
        size="small"
        clearable
        placeholder="搜索当前目录"
      >
        <template #prefix><Icon name="search" :size="15" /></template>
      </NInput>

      <button
        v-if="ui.view === 'files' && auth.cloudConfigured"
        type="button"
        class="tool primary"
        title="上传文件"
        @click="emit('upload')"
      >
        <Icon name="upload" :size="17" /><span>上传</span>
      </button>

      <NDropdown :options="userOptions" trigger="click" placement="bottom-end" @select="onUserSelect">
        <button type="button" class="avatar-btn" :title="displayName">
          <NAvatar :src="avatarSrc" round :size="32" />
        </button>
      </NDropdown>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  position: sticky;
  top: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  gap: 12px;
  height: 62px;
  padding: 0 20px;
  background: var(--gh-surface-blur);
  backdrop-filter: blur(14px);
  border-bottom: 1px solid var(--gh-border);
}
.brand-btn {
  border: 0;
  background: none;
  padding: 0;
  cursor: pointer;
  display: flex;
  align-items: center;
}
.brand-btn:hover {
  opacity: 0.82;
}
.back-btn {
  display: none;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  padding: 0;
  border: 1px solid var(--gh-border);
  border-radius: 10px;
  background: var(--gh-surface);
  color: var(--gh-ink);
  cursor: pointer;
}
.back-btn:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  border-color: var(--gh-border-hover);
}
.divider {
  width: 1px;
  height: 22px;
  background: var(--gh-border);
  flex: 0 0 auto;
}
.page-title {
  font-size: 14px;
  font-weight: 700;
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tools {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  flex: 0 0 auto;
}
.search {
  width: 210px;
}
.tool {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 11px;
  border: 1px solid var(--gh-border);
  border-radius: 10px;
  background: var(--gh-surface);
  color: var(--gh-ink);
  font-size: 13px;
  font-family: inherit;
  cursor: pointer;
  white-space: nowrap;
}
.tool:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  border-color: var(--gh-border-hover);
}
.tool.primary {
  background: var(--gh-gradient);
  border: 0;
  border-radius: 10px;
  color: #fff;
  box-shadow: 0 6px 16px rgba(109, 123, 245, 0.35);
}
.tool.primary:hover {
  filter: brightness(1.06);
}
.avatar-btn {
  border: 0;
  background: none;
  padding: 0;
  border-radius: 50%;
  cursor: pointer;
  display: grid;
  place-items: center;
}
.avatar-btn:hover {
  box-shadow: 0 0 0 3px rgba(109, 123, 245, 0.22);
}
@media (max-width: 900px) {
  .search {
    display: none;
  }
  .tool span {
    display: none;
  }
  .tool {
    width: 34px;
    padding: 0;
    justify-content: center;
  }
}
@media (max-width: 560px) {
  .topbar {
    padding: 0 12px;
    gap: 8px;
  }
  .back-btn {
    display: inline-flex;
  }
  .divider {
    display: none;
  }
  .brand-btn :deep(.word) {
    display: none;
  }
}
</style>

<style>
.user-dropdown-head {
  padding: 10px 14px 8px;
  min-width: 160px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.user-dropdown-head strong {
  font-size: 13px;
  color: var(--gh-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.user-dropdown-head span {
  font-size: 11px;
  color: var(--gh-ink-soft);
}
</style>
