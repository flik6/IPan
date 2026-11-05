<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { NConfigProvider, NMessageProvider, NDialogProvider, darkTheme, zhCN, dateZhCN, type GlobalThemeOverrides } from "naive-ui";
import { auth, checkAuth, checkSetup, refreshLoginStatus } from "./stores/auth";
import { isDark, initTheme, watchSystemTheme } from "./stores/theme";
import { load as loadFiles, initNavHistory, setView } from "./stores/files";
import { ui } from "./stores/ui";
import { refreshWopanUser, hydrateWopanUser } from "./composables/useWopanUser";
import LoginPage from "./components/LoginPage.vue";
import SetupPage from "./components/SetupPage.vue";
import CloudSetupPage from "./components/CloudSetupPage.vue";
import AppTopbar from "./components/AppTopbar.vue";
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

const fileTableRef = ref<InstanceType<typeof FileTable> | null>(null);
const skipSetup = ref(false);

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#6D7BF5",
    primaryColorHover: "#5E6CF3",
    primaryColorPressed: "#4F5DE8",
    primaryColorSuppl: "#5E6CF3",
    infoColor: "#4DABF7",
    successColor: "#22C55E",
    errorColor: "#F25767",
    warningColor: "#FFA940",
    borderRadius: "10px",
    fontFamily: '"Inter", "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif',
  },
};

const stage = computed<"boot" | "setup" | "login" | "cloud-setup" | "app">(() => {
  if (auth.checking) return "boot";
  if (auth.needsSetup) return "setup";
  if (!auth.loggedIn || auth.loginInProgress) return "login";
  if (!auth.ready) return "boot";
  if (auth.cloudChecked && !auth.cloudConfigured && !skipSetup.value) return "cloud-setup";
  return "app";
});

/** /s/<id> 是公开分享页，走独立分支，不参与登录态判断 */
const isPublicShareRoute = computed(() => /^\/s\/[^/]+\/?$/.test(location.pathname));

async function enterApp(): Promise<void> {
  ui.view = "files";
  ui.fileSearch = "";
  initNavHistory();
  if (auth.cloudConfigured) {
    // 账号信息优先用本地缓存，只有缓存缺失时才请求接口
    hydrateWopanUser();
    void refreshWopanUser();
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
    // checkAuth 已经拿到合并下发的登录态；只有旧版响应没带 login 时才补一次请求
    if (!auth.cloudChecked) await refreshLoginStatus();
    auth.ready = true;
    if (auth.cloudConfigured) await enterApp();
    else initNavHistory();
  }
});

onBeforeUnmount(() => {
  stopWatchingTheme?.();
});
</script>

<template>
  <NConfigProvider :theme="isDark ? darkTheme : null" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <NMessageProvider>
      <NDialogProvider>
        <!-- 公开分享页：/s/<id>，无需登录 -->
        <PublicSharePage v-if="isPublicShareRoute" />

        <div v-else-if="stage === 'boot'" class="boot">
          <LoadingLogo />
        </div>

        <SetupPage v-else-if="stage === 'setup'" @done="handleSetupDone" />

        <LoginPage v-else-if="stage === 'login'" @success="handleLoginSuccess" />

        <CloudSetupPage v-else-if="stage === 'cloud-setup'" @done="handleCloudSetupDone" @skip="handleSkipSetup" />

        <div v-else class="shell">
          <AppTopbar @upload="fileTableRef?.pickFiles()" />

          <div class="app-body">
            <div class="main">
              <main class="content">
                <template v-if="ui.view === 'settings'">
                  <SettingsView />
                </template>

                <template v-else-if="ui.view === 'shares'">
                  <SharesView />
                </template>

                <template v-else-if="!auth.cloudConfigured">
                  <div class="setup-empty">
                    <div class="setup-empty-icon">☁</div>
                    <h2>还没有绑定云盘账号</h2>
                    <p class="muted">绑定手机号后即可管理文件。</p>
                    <button type="button" class="primary-btn" @click="goSettings">去设置绑定</button>
                  </div>
                </template>

                <FileTable v-else ref="fileTableRef" />
              </main>
            </div>
          </div>

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
.shell {
  min-height: 100vh;
}
.app-body {
  min-height: calc(100vh - 62px);
}
.main {
  min-height: calc(100vh - 62px);
  display: flex;
  flex-direction: column;
}
.content {
  flex: 1;
  width: 100%;
  max-width: 1200px;
  margin: 0 auto;
  padding: 22px 22px 40px;
}
.boot {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}
.setup-empty {
  margin: 8vh auto 0;
  max-width: 440px;
  text-align: center;
  padding: 40px 28px;
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 18px;
  box-shadow: var(--gh-shadow-sm);
}
.setup-empty-icon {
  font-size: 30px;
  width: 58px;
  height: 58px;
  margin: 0 auto 14px;
  border-radius: 14px;
  display: grid;
  place-items: center;
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
.setup-empty h2 {
  margin: 0 0 8px;
  font-size: 18px;
}
.setup-empty p {
  margin: 0 0 20px;
  font-size: 13px;
  line-height: 1.7;
}
.primary-btn {
  height: 38px;
  padding: 0 18px;
  border: 0;
  border-radius: 10px;
  background: var(--gh-gradient);
  color: #fff;
  font-size: 13px;
  font-family: inherit;
  cursor: pointer;
  box-shadow: 0 6px 16px rgba(109, 123, 245, 0.35);
}
.primary-btn:hover {
  filter: brightness(1.06);
}
@media (max-width: 900px) {
  .content {
    padding: 16px 14px 32px;
  }
}
</style>
