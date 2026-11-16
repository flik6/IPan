<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { NConfigProvider, NMessageProvider, NDialogProvider, darkTheme, zhCN, dateZhCN, type GlobalThemeOverrides } from "naive-ui";
import { auth, checkAuth, checkSetup, refreshLoginStatus } from "./stores/auth";
import { isDark, initTheme, watchSystemTheme } from "./stores/theme";
import { load as loadFiles, initNavHistory, setView } from "./stores/files";
import { ui } from "./stores/ui";
import { refreshWopanUser, hydrateWopanUser, startWopanUserSync, stopWopanUserSync } from "./composables/useWopanUser";
import LoginPage from "./components/LoginPage.vue";
import SetupPage from "./components/SetupPage.vue";
import CloudSetupPage from "./components/CloudSetupPage.vue";
import AppTopbar from "./components/AppTopbar.vue";
import Icon from "./components/Icon.vue";
import LoadingLogo from "./components/LoadingLogo.vue";
import FileTable from "./components/FileTable.vue";
import SettingsView from "./components/SettingsView.vue";
import SharesView from "./components/SharesView.vue";
import PublicSharePage from "./components/PublicSharePage.vue";
import UploadPanel from "./components/UploadPanel.vue";
import GlobalToasts from "./components/GlobalToasts.vue";
import DropOverlay from "./components/DropOverlay.vue";
import PreviewModal from "./components/PreviewModal.vue";
import DetailModal from "./components/DetailModal.vue";
import ShareDialog from "./components/ShareDialog.vue";
import ResolveModal from "./components/ResolveModal.vue";
import FolderPicker from "./components/FolderPicker.vue";
import ProfileEditModal from "./components/ProfileEditModal.vue";

const skipSetup = ref(false);

const themeOverrides = computed<GlobalThemeOverrides>(() => {
  const dark = isDark.value;
  return {
    common: {
      primaryColor: dark ? "#4d8dff" : "#0061ff",
      primaryColorHover: dark ? "#76a7ff" : "#0052d9",
      primaryColorPressed: dark ? "#94b9ff" : "#0047ba",
      primaryColorSuppl: dark ? "#4d8dff" : "#0061ff",
      infoColor: dark ? "#84caff" : "#175cd3",
      successColor: dark ? "#75e0a7" : "#067647",
      errorColor: dark ? "#fda29b" : "#d92d20",
      warningColor: dark ? "#fec84b" : "#b54708",
      borderRadius: "8px",
      textColor1: dark ? "#f2f4f7" : "#111827",
      textColor2: dark ? "#f2f4f7" : "#111827",
      textColor3: dark ? "#98a2b3" : "#6b7280",
      fontFamily: '"Inter", "Segoe UI Variable Text", "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif',
      bodyColor: dark ? "#0f141c" : "#f7f8fa",
      cardColor: dark ? "#161d27" : "#ffffff",
      modalColor: dark ? "#161d27" : "#ffffff",
      popoverColor: dark ? "#1a222d" : "#ffffff",
      tableColor: dark ? "#161d27" : "#ffffff",
      inputColor: dark ? "#1a222d" : "#ffffff",
      borderColor: dark ? "#263140" : "#e5e7eb",
      dividerColor: dark ? "#263140" : "#e5e7eb",
    },
    Input: {
      placeholderColor: dark ? "#98a2b3" : "#9ca3af",
      color: dark ? "#1a222d" : "#ffffff",
      border: `1px solid ${dark ? "#344054" : "#d1d5db"}`,
    },
    Button: {
      borderRadiusMedium: "8px",
      fontWeight: "500",
      textColorPrimary: "#ffffff",
      textColorHoverPrimary: "#ffffff",
      textColorPressedPrimary: "#ffffff",
      textColorFocusPrimary: "#ffffff",
      colorPrimary: dark ? "#4d8dff" : "#0061ff",
      colorHoverPrimary: dark ? "#76a7ff" : "#0052d9",
      colorPressedPrimary: dark ? "#94b9ff" : "#0047ba",
    },
    Dropdown: {
      color: dark ? "#1a222d" : "#ffffff",
      borderRadius: "8px",
      optionTextColor: dark ? "#f2f4f7" : "#111827",
      optionTextColorHover: dark ? "#4d8dff" : "#0061ff",
    },
    Popover: {
      color: dark ? "#1a222d" : "#ffffff",
      borderRadius: "8px",
      textColor: dark ? "#f2f4f7" : "#111827",
    },
    Dialog: {
      color: dark ? "#161d27" : "#ffffff",
      borderRadius: "12px",
    },
  };
});

const stage = computed<"boot" | "setup" | "login" | "cloud-setup" | "app">(() => {
  if (auth.checking) return "boot";
  if (auth.needsSetup) return "setup";
  if (!auth.loggedIn || auth.loginInProgress) return "login";
  if (!auth.ready) return "boot";
  if (auth.cloudChecked && !auth.cloudConfigured && !skipSetup.value) return "cloud-setup";
  return "app";
});

const isPublicShareRoute = computed(() => /^\/s\/[^/]+\/?$/.test(location.pathname));

async function enterApp(): Promise<void> {
  ui.view = "files";
  ui.fileSearch = "";
  initNavHistory();
  if (auth.cloudConfigured) {
    hydrateWopanUser();
    startWopanUserSync();
    void loadFiles();
  }
}

async function handleSetupDone(): Promise<void> {
  auth.ready = true;
  skipSetup.value = false;
  auth.checking = false;
  await refreshLoginStatus();
  await enterApp();
}

async function handleLoginSuccess(): Promise<void> {
  auth.ready = true;
  skipSetup.value = false;
  await enterApp();
}

async function handleCloudSetupDone(): Promise<void> {
  auth.ready = true;
  skipSetup.value = false;
  await refreshLoginStatus();
  await enterApp();
}

async function handleSkipSetup(): Promise<void> {
  auth.ready = true;
  skipSetup.value = true;
  await enterApp();
}

function goSettings(): void {
  setView("settings");
}

let stopWatchingTheme: (() => void) | null = null;

onMounted(async () => {
  initTheme();
  stopWatchingTheme = watchSystemTheme();
  if (await checkSetup()) {
    auth.checking = false;
    auth.loggedIn = false;
    return;
  }
  if (await checkAuth()) {
    if (!auth.cloudChecked) await refreshLoginStatus();
    auth.ready = true;
    if (auth.cloudConfigured) await enterApp();
    else initNavHistory();
  }
});

onBeforeUnmount(() => {
  stopWatchingTheme?.();
  stopWopanUserSync();
});
</script>

<template>
  <NConfigProvider :theme="isDark ? darkTheme : null" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <NMessageProvider>
      <NDialogProvider>
        <PublicSharePage v-if="isPublicShareRoute" />

        <div v-else-if="stage === 'boot'" class="boot-screen op-fade">
          <LoadingLogo />
        </div>

        <SetupPage v-else-if="stage === 'setup'" @done="handleSetupDone" />

        <LoginPage v-else-if="stage === 'login'" @success="handleLoginSuccess" />

        <CloudSetupPage v-else-if="stage === 'cloud-setup'" @done="handleCloudSetupDone" @skip="handleSkipSetup" />

        <div v-else class="app-shell">
          <a href="#workspace-main" class="skip-link">跳到主内容</a>
          <AppTopbar />

          <main id="workspace-main" tabindex="-1" class="app-main">
            <div class="workspace-content">
              <SettingsView v-if="ui.view === 'settings'" />
              <SharesView v-else-if="ui.view === 'shares'" />

              <div v-else-if="!auth.cloudConfigured" class="unbound-stage op-rise">
                <div class="unbound-copy">
                  <span class="eyebrow">云盘尚未连接</span>
                  <h1 class="display">连接你的云盘账号</h1>
                  <p class="display-sub">绑定联通云盘后，即可在这里浏览文件、创建分享并管理账号。</p>
                  <button type="button" class="btn btn-primary" @click="goSettings">
                    去设置绑定
                    <Icon name="chevron" :size="15" />
                  </button>
                </div>
              </div>

              <FileTable v-else />
            </div>
          </main>

          <UploadPanel />
          <DropOverlay />
          <PreviewModal />
          <DetailModal />
          <ShareDialog />
          <ResolveModal />
          <FolderPicker />
          <ProfileEditModal v-model:show="ui.showProfileEdit" @saved="refreshWopanUser" />
        </div>

        <GlobalToasts />
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>

<style scoped>
.boot-screen {
  min-height: 100dvh;
  display: grid;
  place-items: center;
  background: var(--op-bg);
}

.app-shell { min-height: 100dvh; }

.app-main {
  min-height: 100dvh;
  margin-left: var(--op-sidebar);
  background: var(--op-bg);
}

.workspace-content {
  width: 100%;
  max-width: var(--op-shell);
  margin-inline: auto;
  padding: clamp(30px, 4.2vw, 58px) var(--op-gutter) 80px;
}

.skip-link {
  position: fixed;
  top: 10px;
  left: 10px;
  z-index: 1000;
  padding: 9px 14px;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
  font-size: 13px;
  font-weight: 650;
  transform: translateY(-180%);
}

.skip-link:focus { transform: none; }

.unbound-stage {
  display: flex;
  align-items: flex-start;
  min-height: 58vh;
  padding-top: clamp(36px, 9vh, 92px);
}

.unbound-copy { max-width: 600px; }
.unbound-copy .display { max-width: 14ch; }
.unbound-copy .btn { margin-top: 22px; }

@media (max-width: 1040px) {
  .app-main { margin-left: 0; padding-top: 70px; }
}

@media (max-width: 760px) {
  .workspace-content { padding-top: 24px; padding-bottom: calc(var(--gh-mobile-dock-clearance) + 34px); }
  .app-main { padding-top: 64px; }
  .unbound-stage { min-height: 48vh; padding-top: 24px; }
}
</style>
