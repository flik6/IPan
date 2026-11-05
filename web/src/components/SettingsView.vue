<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NAvatar, NButton, NInput, NRadioButton, NRadioGroup, NSwitch } from "naive-ui";
import Icon from "./Icon.vue";
import CloudAccountForm from "./CloudAccountForm.vue";
import { auth } from "../stores/auth";
import { theme, setThemeMode, type ThemeMode } from "../stores/theme";
import { ui } from "../stores/ui";
import { settings, loadSettings, setShowHidden } from "../stores/settings";
import { wopanUser, refreshWopanUser } from "../composables/useWopanUser";
import { api, setAuthToken } from "../api/client";
import { passkeyEnrolled as passkeyEnrolledFn, passkeySupported as passkeySupportedFn, registerPasskey } from "../utils/passkey";
import { showToast } from "../stores/toast";
import { maskPhone } from "../utils/format";

const DEFAULT_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><circle cx="32" cy="32" r="30" fill="#6D7BF5"/><circle cx="32" cy="25" r="11" fill="#fff"/><path d="M12 52c3-11 11-16 20-16s17 5 20 16a30 30 0 0 1-40 0z" fill="#fff"/></svg>',
  );

const avatarSrc = computed(() => wopanUser.avatarUrl || DEFAULT_AVATAR);
const showName = computed(() => wopanUser.nickName || maskPhone(wopanUser.phone) || "尚未获取账号信息");
const canEditProfile = computed(() => auth.cloudConfigured);
const syncedText = computed(() =>
  wopanUser.syncedAt ? new Date(wopanUser.syncedAt).toLocaleString() : "尚未获取",
);

const pkSupported = passkeySupportedFn();
const pkEnrolled = ref(passkeyEnrolledFn());
const passkeyBusy = ref(false);

// ---- 隐藏文件显隐 ----
const hiddenBusy = ref(false);

async function toggleHidden(value: boolean): Promise<void> {
  hiddenBusy.value = true;
  try {
    await setShowHidden(value);
    showToast(value ? "已显示隐藏文件" : "已隐藏以 . 开头的文件", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    hiddenBusy.value = false;
  }
}

// ---- 外观主题 ----
function onThemeChange(value: string | number): void {
  if (value === "light" || value === "dark" || value === "system") {
    setThemeMode(value as ThemeMode);
  }
}

// ---- 修改访问密码 ----
const pwdOpen = ref(false);
const oldPwd = ref("");
const newPwd = ref("");
const confirmPwd = ref("");
const pwdBusy = ref(false);

function openPwdForm(): void {
  oldPwd.value = "";
  newPwd.value = "";
  confirmPwd.value = "";
  pwdOpen.value = true;
}

async function submitPassword(): Promise<void> {
  if (newPwd.value.trim().length < 4) {
    showToast("新密码至少 4 位", "warning");
    return;
  }
  if (newPwd.value.trim() !== confirmPwd.value.trim()) {
    showToast("两次输入的新密码不一致", "warning");
    return;
  }
  pwdBusy.value = true;
  try {
    const res = await api<{ token?: string }>("/api/settings/password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ oldPassword: oldPwd.value.trim(), password: newPwd.value.trim() }),
    });
    // 旧密码立即失效，把本地登录态换成新密码，避免被踢回登录页
    if (res?.token) setAuthToken(res.token);
    pwdOpen.value = false;
    showToast("访问密码已更新", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    pwdBusy.value = false;
  }
}

// ---- 账号信息（缓存 + 手动获取） ----
const detailBusy = ref(false);

async function fetchAccountInfo(): Promise<void> {
  detailBusy.value = true;
  try {
    await refreshWopanUser({ force: true });
    if (wopanUser.failed) throw new Error("获取失败，请确认云盘账号是否正常");
    showToast("已从云端获取最新账号信息", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    detailBusy.value = false;
  }
}

async function handleRegisterPasskey(): Promise<void> {
  passkeyBusy.value = true;
  try {
    await registerPasskey();
    showToast("Passkey 注册成功，下次可免密登录", "success");
    pkEnrolled.value = true;
  } catch (e) {
    showToast((e as Error).message || "Passkey 注册失败", "error");
  } finally {
    passkeyBusy.value = false;
  }
}

onMounted(() => void loadSettings());
</script>

<template>
  <div class="settings">
    <header class="page-head">
      <div>
        <h1>设置</h1>
        <p class="muted">管理云盘账号、显示偏好与访问权限。</p>
      </div>
    </header>

    <div class="grid">
      <div class="col-main">
        <section class="card">
          <div class="card-head">
            <div class="card-title">
              <span class="card-icon"><Icon name="user" :size="18" /></span>
              <div>
                <h2>账号信息</h2>
                <p class="muted">昵称、头像等资料同步到联通云盘官方账号。</p>
              </div>
            </div>
          </div>
          <div class="card-body">
            <div class="profile-row">
              <NAvatar :src="avatarSrc" round :size="56" />
              <div class="profile-meta">
                <strong>{{ showName }}</strong>
                <span class="muted">{{ maskPhone(wopanUser.phone) || "—" }}</span>
                <span class="muted tiny">上次获取：{{ syncedText }}</span>
              </div>
              <div v-if="auth.cloudConfigured" class="profile-actions">
                <NButton size="small" :loading="detailBusy" @click="fetchAccountInfo">
                  <template #icon><Icon name="refresh" :size="15" /></template>
                  获取账号信息
                </NButton>
                <NButton v-if="canEditProfile" size="small" type="primary" secondary @click="ui.showProfileEdit = true">
                  编辑资料
                </NButton>
              </div>
              <span v-else class="muted small">绑定云盘账号后可查看</span>
            </div>
          </div>
        </section>

        <section class="card">
          <div class="card-head">
            <div class="card-title">
              <span class="card-icon"><Icon name="cloud" :size="18" /></span>
              <div>
                <h2>云盘账号</h2>
                <p class="muted">短信登录后自动维护令牌。</p>
              </div>
            </div>
          </div>
          <div class="card-body">
            <CloudAccountForm mode="settings" />
          </div>
        </section>
      </div>

      <aside class="col-side">
        <section class="card">
          <div class="card-head compact">
            <div class="card-title">
              <span class="card-icon"><Icon name="eye" :size="18" /></span>
              <h2>显示偏好</h2>
            </div>
          </div>
          <div class="card-body">
            <div class="switch-row">
              <div class="switch-text">
                <strong>显示隐藏文件</strong>
                <p class="muted">开启后列出以 . 开头的文件与文件夹。</p>
              </div>
              <NSwitch :value="settings.showHidden" :loading="hiddenBusy" @update:value="toggleHidden" />
            </div>

            <div class="theme-row">
              <div class="switch-text">
                <strong>外观主题</strong>
                <p class="muted">跟随系统时会随设备的深浅色设置自动切换。</p>
              </div>
              <NRadioGroup :value="theme.mode" size="small" @update:value="onThemeChange">
                <NRadioButton value="light">浅色</NRadioButton>
                <NRadioButton value="dark">深色</NRadioButton>
                <NRadioButton value="system">跟随系统</NRadioButton>
              </NRadioGroup>
            </div>
          </div>
        </section>

        <section class="card">
          <div class="card-head compact">
            <div class="card-title">
              <span class="card-icon"><Icon name="shield" :size="18" /></span>
              <h2>访问与安全</h2>
            </div>
          </div>
          <div class="card-body">
            <dl class="kv">
              <div><dt>访问方式</dt><dd>Authorization: Bearer &lt;访问密码&gt;</dd></div>
            </dl>

            <div class="pwd-block">
              <button v-if="!pwdOpen" type="button" class="text-btn" @click="openPwdForm">修改访问密码</button>
              <div v-else class="pwd-form">
                <NInput v-model:value="oldPwd" type="password" show-password-on="click" size="small" placeholder="当前密码" />
                <NInput v-model:value="newPwd" type="password" show-password-on="click" size="small" placeholder="新密码（至少 4 位）" />
                <NInput
                  v-model:value="confirmPwd"
                  type="password"
                  show-password-on="click"
                  size="small"
                  placeholder="确认新密码"
                  @keydown.enter.prevent="submitPassword"
                />
                <div class="pwd-actions">
                  <NButton size="tiny" :disabled="pwdBusy" @click="pwdOpen = false">取消</NButton>
                  <NButton size="tiny" type="primary" :loading="pwdBusy" @click="submitPassword">保存</NButton>
                </div>
              </div>
            </div>

            <div v-if="pkSupported" class="passkey-row">
              <p class="muted">注册本设备的 Passkey 后，下次打开无需输入密码，指纹 / 刷脸一键登录。</p>
              <NButton v-if="pkEnrolled" size="small" disabled>本设备已注册 Passkey</NButton>
              <NButton v-else size="small" type="primary" secondary :loading="passkeyBusy" @click="handleRegisterPasskey">
                注册 Passkey
              </NButton>
            </div>
          </div>
        </section>

        <section class="card">
          <div class="card-head compact">
            <div class="card-title">
              <span class="card-icon"><Icon name="file" :size="18" /></span>
              <h2>关于</h2>
            </div>
          </div>
          <div class="card-body">
            <dl class="kv">
              <div><dt>项目</dt><dd>iPan</dd></div>
              <div><dt>运行环境</dt><dd>Go + Vue</dd></div>
              <div><dt>令牌保活</dt><dd>{{ auth.cloud?.enabled ? "已启用" : auth.cloud?.paused ? "已暂停" : "未启用" }}</dd></div>
              <div><dt>文件床签名</dt><dd>{{ settings.signParamConfigured ? "已生成" : "未配置" }}</dd></div>
            </dl>
          </div>
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 1160px;
  margin: 0 auto;
}
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 20px;
}
.page-head h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 750;
  letter-spacing: 0.01em;
}
.page-head p {
  margin: 6px 0 0;
  font-size: 13px;
}
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 18px;
  align-items: start;
}
.col-main,
.col-side {
  display: flex;
  flex-direction: column;
  gap: 18px;
  min-width: 0;
}
.card {
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 16px;
  box-shadow: var(--gh-shadow-sm);
  overflow: hidden;
}
.card-head {
  padding: 18px 20px 0;
}
.card-head.compact {
  padding-bottom: 4px;
}
.card-title {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.card-icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 12px;
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
  flex: 0 0 auto;
}
.card-title h2 {
  margin: 1px 0 0;
  font-size: 15px;
  font-weight: 700;
}
.card-title p {
  margin: 5px 0 0;
  font-size: 12px;
  line-height: 1.6;
}
.card-body {
  padding: 16px 20px 20px;
}
.kv {
  margin: 0;
  display: grid;
  gap: 10px;
}
.kv > div {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 14px;
}
.kv dt {
  font-size: 12px;
  color: var(--gh-ink-soft);
  flex: 0 0 auto;
}
.kv dd {
  margin: 0;
  font-size: 12px;
  text-align: right;
  overflow-wrap: anywhere;
}
.switch-row {
  display: flex;
  align-items: center;
  gap: 14px;
}
.theme-row {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed var(--gh-border);
}
.theme-row :deep(.n-radio-group) {
  flex: 0 0 auto;
}
.switch-text {
  flex: 1;
  min-width: 0;
}
.switch-text strong {
  font-size: 13px;
}
.switch-text p {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.6;
}
.pwd-block {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed var(--gh-border);
}
.text-btn {
  border: 0;
  background: none;
  padding: 0;
  color: var(--gh-primary);
  font-size: 12px;
  font-family: inherit;
  cursor: pointer;
}
.text-btn:hover {
  text-decoration: underline;
}
.pwd-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.pwd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.passkey-row {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed var(--gh-border);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.passkey-row p {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
}
.profile-row {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
}
.profile-meta {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  flex: 1;
}
.profile-meta strong {
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.profile-meta span {
  font-size: 12px;
}
.profile-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.small {
  font-size: 12px;
}
.tiny {
  font-size: 11px;
}
@media (max-width: 900px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
