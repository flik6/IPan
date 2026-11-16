<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from "vue";
import { NButton, NInput, NSwitch, type InputInst } from "naive-ui";
import Icon from "./Icon.vue";
import InfoHint from "./InfoHint.vue";
import CloudAccountForm from "./CloudAccountForm.vue";
import { auth } from "../stores/auth";
import { theme, setThemeMode, type ThemeMode } from "../stores/theme";
import { ui } from "../stores/ui";
import { settings, loadSettings, setShowHidden } from "../stores/settings";
import { wopanUser } from "../composables/useWopanUser";
import { api, setAuthToken } from "../api/client";
import { passkeyEnrolled as passkeyEnrolledFn, passkeySupported as passkeySupportedFn, registerPasskey } from "../utils/passkey";
import { showToast } from "../stores/toast";
import { maskPhone } from "../utils/format";

type SectionKey = "account" | "cloud" | "webdav" | "security" | "appearance" | "about";

const sections: { key: SectionKey; label: string; icon: string }[] = [
  { key: "account", label: "账号信息", icon: "user" },
  { key: "cloud", label: "云盘账号", icon: "cloud" },
  { key: "webdav", label: "WebDAV", icon: "link" },
  { key: "security", label: "访问与安全", icon: "shield" },
  { key: "appearance", label: "显示偏好", icon: "eye" },
  { key: "about", label: "关于", icon: "file" },
];
// 顶栏等入口可通过 ui.settingsSection 指定要打开的分类
const active = computed<SectionKey>({
  get: () => (sections.some((s) => s.key === ui.settingsSection) ? (ui.settingsSection as SectionKey) : "account"),
  set: (value) => { ui.settingsSection = value; },
});

const showName = computed(() => wopanUser.nickName || maskPhone(wopanUser.phone) || "尚未获取账号信息");
const syncedText = computed(() =>
  wopanUser.syncedAt ? new Date(wopanUser.syncedAt).toLocaleString("zh-CN") : "尚未获取",
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

const themeOptions: { mode: ThemeMode; icon: string; label: string }[] = [
  { mode: "light", icon: "sun", label: "浅色" },
  { mode: "dark", icon: "moon", label: "深色" },
  { mode: "system", icon: "monitor", label: "跟随系统" },
];

// ---- 修改访问密码 ----
const pwdOpen = ref(false);
const oldPwd = ref("");
const newPwd = ref("");
const confirmPwd = ref("");
const pwdBusy = ref(false);

const passwordInput = ref<InputInst | null>(null);
const passwordTrigger = ref<InstanceType<typeof NButton> | null>(null);

async function openPwdForm(): Promise<void> {
  oldPwd.value = "";
  newPwd.value = "";
  confirmPwd.value = "";
  pwdOpen.value = true;
  await nextTick();
  passwordInput.value?.focus();
}

async function closePwdForm(): Promise<void> {
  pwdOpen.value = false;
  await nextTick();
  passwordTrigger.value?.$el.focus();
}

async function submitPassword(): Promise<void> {
  if (pwdBusy.value) return;
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
    await closePwdForm();
    showToast("访问密码已更新", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    pwdBusy.value = false;
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

// ---- WebDAV ----
const webdavOpen = ref(false);
const webdavUser = ref("");
const webdavPass = ref("");
const webdavConfirm = ref("");
const webdavBusy = ref(false);
const webdavDisableBusy = ref(false);
const webdavUrl = computed(() => `${window.location.origin}/dav/`);

function openWebdavForm(): void {
  webdavUser.value = settings.webdavUsername || "webdav";
  webdavPass.value = "";
  webdavConfirm.value = "";
  webdavOpen.value = true;
}

function closeWebdavForm(): void {
  if (webdavBusy.value) return;
  webdavOpen.value = false;
}

async function saveWebdav(): Promise<void> {
  if (webdavBusy.value) return;
  const username = webdavUser.value.trim();
  const password = webdavPass.value;
  if (!username) {
    showToast("请输入 WebDAV 用户名", "warning");
    return;
  }
  if (username.includes(":")) {
    showToast("WebDAV 用户名不能包含冒号", "warning");
    return;
  }
  if (password.length < 4) {
    showToast("WebDAV 密码至少 4 位", "warning");
    return;
  }
  if (password !== webdavConfirm.value) {
    showToast("两次输入的 WebDAV 密码不一致", "warning");
    return;
  }
  webdavBusy.value = true;
  try {
    await api("/api/settings/webdav", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    await loadSettings();
    webdavOpen.value = false;
    showToast("WebDAV 已启用", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    webdavBusy.value = false;
  }
}

async function disableWebdav(): Promise<void> {
  if (webdavDisableBusy.value) return;
  webdavDisableBusy.value = true;
  try {
    await api("/api/settings/webdav", { method: "DELETE" });
    await loadSettings();
    webdavOpen.value = false;
    showToast("WebDAV 已关闭", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
  } finally {
    webdavDisableBusy.value = false;
  }
}

async function copyWebdavUrl(): Promise<void> {
  try {
    await navigator.clipboard.writeText(webdavUrl.value);
    showToast("已复制 WebDAV 地址", "success");
  } catch {
    showToast("复制失败，请手动复制", "error");
  }
}

onMounted(() => void loadSettings());
</script>

<template>
  <div class="settings">
    <header class="page-header settings-page-head">
      <div class="page-header-copy">
        <span class="eyebrow">账户与偏好</span>
        <h1 class="display">设置</h1>
        <p class="display-sub">管理账号资料、云盘连接、安全设置和显示偏好。</p>
      </div>
      <div class="settings-page-mark" aria-hidden="true">
        <span class="mono">{{ String(sections.findIndex((s) => s.key === active) + 1).padStart(2, "0") }}</span>
        <span>{{ sections.find((s) => s.key === active)?.label }}</span>
      </div>
    </header>

    <div class="settings-shell">
      <!-- 左侧分类 -->
      <nav class="settings-nav" aria-label="设置分类">
        <button
          v-for="item in sections"
          :key="item.key"
          type="button"
          class="nav-item"
          :aria-current="active === item.key ? 'true' : undefined"
          @click="active = item.key"
        >
          <Icon :name="item.icon" :size="17" />
          <span>{{ item.label }}</span>
        </button>
      </nav>

      <!-- 右侧内容 -->
      <section class="settings-panel surface-panel" :aria-label="sections.find((s) => s.key === active)?.label">

        <!-- 账号信息 -->
        <div v-show="active === 'account'" class="pane">
          <div class="pane-card identity">
            <div class="avatar-ring">
              <img v-if="wopanUser.avatarUrl" :src="wopanUser.avatarUrl" class="profile-avatar" alt="" />
              <span v-else class="profile-avatar profile-fallback" aria-hidden="true">
                {{ (wopanUser.nickName || "iP").slice(0, 1) }}
              </span>
            </div>
            <div class="identity-copy">
              <h2>{{ showName }}</h2>
              <p v-if="auth.cloudConfigured" class="muted">
                {{ maskPhone(wopanUser.phone) || "—" }}
                <span class="dot" aria-hidden="true">·</span> 同步于 {{ syncedText }}
                <InfoHint text="账号资料由服务器在后台自动同步，无需手动获取。" />
              </p>
              <p v-else class="muted">绑定联通云盘后可查看和编辑资料。</p>
            </div>
            <div v-if="auth.cloudConfigured" class="identity-actions">
              <NButton type="primary" @click="ui.showProfileEdit = true">编辑资料</NButton>
            </div>
          </div>

          <dl class="service-info">
            <div><dt>昵称</dt><dd>{{ wopanUser.nickName || "—" }}</dd></div>
            <div><dt>手机号</dt><dd>{{ wopanUser.phone ? maskPhone(wopanUser.phone) : "—" }}</dd></div>
            <div>
              <dt>会员等级<InfoHint text="取自联通云盘账号的 VIP 等级。" /></dt>
              <dd>{{ wopanUser.vipLevel ? "VIP" + wopanUser.vipLevel : "普通用户" }}</dd>
            </div>
            <div>
              <dt>资料更新时间<InfoHint text="服务器每次成功获取账号信息的时间，后台会自动刷新。" /></dt>
              <dd>{{ syncedText }}</dd>
            </div>
          </dl>
        </div>

        <!-- 云盘账号 -->
        <div v-show="active === 'cloud'" class="pane">
          <header class="pane-head">
            <h2>云盘账号</h2>
            <InfoHint text="查看绑定状态、容量与令牌自动保活情况。" />
          </header>
          <CloudAccountForm mode="settings" />
        </div>

        <!-- WebDAV -->
        <div v-show="active === 'webdav'" class="pane">
          <header class="pane-head">
            <h2>WebDAV</h2>
            <InfoHint text="通过 /dav/ 挂载云盘。WebDAV 凭据独立于网页访问密码。" />
          </header>

          <div class="webdav-summary">
            <div>
              <span class="webdav-status" :class="{ enabled: settings.webdavConfigured }">
                {{ settings.webdavConfigured ? "已启用" : "未启用" }}
              </span>
              <strong>{{ settings.webdavConfigured ? settings.webdavUsername : "配置独立访问凭据" }}</strong>
              <p class="muted">客户端填写下方地址，并使用这里设置的用户名和密码登录。</p>
            </div>
            <NButton type="primary" :disabled="webdavBusy" @click="openWebdavForm">
              {{ settings.webdavConfigured ? "修改凭据" : "启用 WebDAV" }}
            </NButton>
          </div>

          <div class="webdav-url">
            <code>{{ webdavUrl }}</code>
            <NButton size="small" @click="copyWebdavUrl">复制地址</NButton>
          </div>

          <form v-if="webdavOpen" class="pwd-form webdav-form" @submit.prevent="saveWebdav">
            <div class="password-field">
              <label for="webdav-username">用户名</label>
              <NInput
                v-model:value="webdavUser"
                :disabled="webdavBusy"
                placeholder="例如 webdav"
                :input-props="{ id: 'webdav-username', autocomplete: 'username' }"
              />
            </div>
            <div class="password-field">
              <label for="webdav-password">密码</label>
              <NInput
                v-model:value="webdavPass"
                type="password"
                show-password-on="click"
                :disabled="webdavBusy"
                placeholder="至少 4 位"
                :input-props="{ id: 'webdav-password', autocomplete: 'new-password' }"
              />
            </div>
            <div class="password-field">
              <label for="webdav-confirm">确认密码</label>
              <NInput
                v-model:value="webdavConfirm"
                type="password"
                show-password-on="click"
                :disabled="webdavBusy"
                :input-props="{ id: 'webdav-confirm', autocomplete: 'new-password' }"
              />
            </div>
            <div class="pwd-actions">
              <NButton v-if="settings.webdavConfigured" type="error" secondary :loading="webdavDisableBusy" :disabled="webdavBusy" @click="disableWebdav">
                关闭 WebDAV
              </NButton>
              <span class="form-spacer" />
              <NButton :disabled="webdavBusy" @click="closeWebdavForm">取消</NButton>
              <NButton type="primary" attr-type="submit" :loading="webdavBusy">保存</NButton>
            </div>
          </form>
        </div>

        <!-- 访问与安全 -->
        <div v-show="active === 'security'" class="pane">
          <header class="pane-head">
            <h2>访问与安全</h2>
            <InfoHint text="iPan 自身访问密码与本设备 Passkey，与联通云盘账号无关。" />
          </header>

          <div class="setting-row">
            <div>
              <strong>访问密码 <InfoHint text="打开 iPan 时需要输入的密码，也可作为 API 的 Bearer 令牌。" /></strong>
            </div>
            <NButton v-if="!pwdOpen" ref="passwordTrigger" aria-controls="password-form" @click="openPwdForm">修改访问密码</NButton>
          </div>

          <form v-if="pwdOpen" id="password-form" class="pwd-form" @submit.prevent="submitPassword">
            <div class="password-field">
              <label for="old-password">当前密码</label>
              <NInput ref="passwordInput" v-model:value="oldPwd" type="password" show-password-on="click" :disabled="pwdBusy" :input-props="{ id: 'old-password', autocomplete: 'current-password' }" />
            </div>
            <div class="password-field">
              <label for="new-password">新密码</label>
              <NInput v-model:value="newPwd" type="password" show-password-on="click" placeholder="至少 4 位" :disabled="pwdBusy" :input-props="{ id: 'new-password', autocomplete: 'new-password' }" />
            </div>
            <div class="password-field">
              <label for="confirm-password">确认新密码</label>
              <NInput v-model:value="confirmPwd" type="password" show-password-on="click" :disabled="pwdBusy" :input-props="{ id: 'confirm-password', autocomplete: 'new-password' }" />
            </div>
            <div class="pwd-actions">
              <NButton :disabled="pwdBusy" @click="closePwdForm">取消</NButton>
              <NButton type="primary" attr-type="submit" :loading="pwdBusy">保存密码</NButton>
            </div>
          </form>

          <div v-if="pkSupported" class="setting-row passkey-row">
            <div>
              <strong>Passkey <InfoHint text="使用指纹、面容或设备 PIN 代替密码登录，凭据只保存在本设备。" /></strong>
              <p class="muted">{{ pkEnrolled ? "本设备已注册，可使用指纹或面容登录。" : "注册后可通过指纹或面容免密码登录。" }}</p>
            </div>
            <NButton v-if="!pkEnrolled" type="primary" secondary :loading="passkeyBusy" @click="handleRegisterPasskey">注册 Passkey</NButton>
            <span v-else class="enrolled"><Icon name="check" :size="16" /> 已注册</span>
          </div>
        </div>

        <!-- 显示偏好 -->
        <div v-show="active === 'appearance'" class="pane">
          <header class="pane-head">
            <h2>显示偏好</h2>
            <InfoHint text="外观主题会保存在本机，不随账号同步。" />
          </header>

          <fieldset class="theme-fieldset">
            <legend>外观主题</legend>
            <div class="theme-options">
              <label v-for="option in themeOptions" :key="option.mode" class="theme-option">
                <input type="radio" name="settings-theme" :value="option.mode" :checked="theme.mode === option.mode" @change="setThemeMode(option.mode)" />
                <span class="theme-preview" :class="option.mode" aria-hidden="true"><i /><i /><i /></span>
                <span class="theme-label"><Icon :name="option.icon" :size="15" /> {{ option.label }}</span>
              </label>
            </div>
          </fieldset>

          <div class="setting-row">
            <div>
              <strong id="hidden-label">显示隐藏文件 <InfoHint text="列出以点号开头的文件和文件夹，例如 .gitignore。" /></strong>
            </div>
            <NSwitch :value="settings.showHidden" :loading="hiddenBusy" aria-labelledby="hidden-label" @update:value="toggleHidden" />
          </div>
        </div>

        <!-- 关于 -->
        <div v-show="active === 'about'" class="pane">
          <header class="pane-head">
            <h2>关于 iPan</h2>
            <InfoHint text="iPan 是自托管的联通云盘网页客户端。" />
          </header>

          <dl class="service-info">
            <div><dt>运行环境</dt><dd>Go + Vue</dd></div>
            <div>
              <dt>令牌保活<InfoHint text="服务器在后台定期用 refresh token 换新令牌，无需手动刷新。" /></dt>
              <dd>{{ auth.cloud?.enabled ? "已启用" : auth.cloud?.paused ? "已暂停" : "未启用" }}</dd>
            </div>
            <div>
              <dt>文件床签名<InfoHint text="下载直链所需的签名密钥，由服务器随机生成并保存在本地数据库。" /></dt>
              <dd>{{ settings.signParamConfigured ? "已生成" : "未配置" }}</dd>
            </div>
          </dl>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.settings-page-head { margin-bottom: 32px; }

.settings-page-mark {
  display: grid;
  justify-items: end;
  gap: 3px;
  color: var(--op-ink-faint);
  font-size: 11px;
  text-align: right;
}

.settings-page-mark .mono {
  color: var(--op-accent);
  font-size: 28px;
  letter-spacing: -0.06em;
  line-height: 1;
}

.settings-shell {
  display: grid;
  grid-template-columns: 232px minmax(0, 1fr);
  align-items: start;
  gap: clamp(28px, 4vw, 64px);
}

.settings-nav {
  position: sticky;
  top: 28px;
  display: grid;
  counter-reset: settings-section;
  border-top: 1px solid var(--op-rule-strong);
}

.settings-nav .nav-item {
  counter-increment: settings-section;
  display: grid;
  grid-template-columns: 24px 18px minmax(0, 1fr);
  align-items: center;
  gap: 9px;
  min-height: 50px;
  padding: 0 10px;
  border: 0;
  border-bottom: 1px solid var(--op-rule);
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 12px;
  font-weight: 600;
  text-align: left;
  cursor: pointer;
  transition: color 160ms ease, background-color 160ms ease;
}

.settings-nav .nav-item::before {
  content: counter(settings-section, decimal-leading-zero);
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
}

.settings-nav .nav-item:hover { color: var(--op-ink); background: var(--op-surface); }

.settings-nav .nav-item[aria-current="true"] {
  color: var(--op-accent);
  background: var(--op-accent-soft);
  box-shadow: inset 2px 0 0 var(--op-accent);
}

.settings-nav .nav-item[aria-current="true"]::before { color: var(--op-accent); }

.settings-panel {
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
}

.pane { display: grid; gap: 26px; }

.pane-card.identity {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 18px;
  padding: 24px;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
  border: 0;
}

.avatar-ring { flex: none; }
.profile-avatar { border-radius: 0 !important; background: var(--op-accent-soft); color: var(--op-accent); }

.identity-copy h2 {
  margin: 0;
  font-family: var(--op-font-display);
  font-size: clamp(30px, 4vw, 44px);
  font-weight: 500;
  letter-spacing: -0.04em;
  line-height: 1.05;
}

.identity-copy p {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  margin: 7px 0 0;
  color: color-mix(in srgb, var(--op-feature-ink) 60%, transparent);
  font-size: 11px;
}

.identity-copy .dot { color: var(--op-accent); }

.identity-actions .n-button {
  --n-color: transparent !important;
  --n-text-color: var(--op-feature-ink) !important;
  --n-border: 1px solid color-mix(in srgb, var(--op-feature-ink) 35%, transparent) !important;
}

.service-info {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin: 0;
  border-top: 1px solid var(--op-rule-strong);
}

.service-info > div {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 18px 4px;
  border-bottom: 1px solid var(--op-rule);
}

.service-info > div:nth-child(odd) { padding-right: 24px; border-right: 1px solid var(--op-rule); }
.service-info > div:nth-child(even) { padding-left: 24px; }

.service-info dt {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.service-info dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--op-ink);
  font-size: 14px;
  font-weight: 600;
}

.pane-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--op-rule-strong);
}

.pane-head h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-family: var(--op-font-display);
  font-size: clamp(34px, 4vw, 48px);
  font-weight: 500;
  letter-spacing: -0.045em;
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 0;
  border-bottom: 1px solid var(--op-rule);
}

.setting-row strong { color: var(--op-ink); font-size: 14px; }
.setting-row .muted { margin: 5px 0 0; font-size: 12px; }

.pwd-form {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
  padding: 20px;
  border: 1px solid var(--op-rule);
  background: var(--op-surface);
}

.password-field { display: grid; gap: 6px; }
.password-field:first-child { grid-column: 1 / -1; }
.password-field label { color: var(--op-ink-soft); font-size: 11px; font-weight: 650; }
.pwd-actions { grid-column: 1 / -1; display: flex; justify-content: flex-end; gap: 8px; }

.theme-fieldset { margin: 0; padding: 0; border: 0; }

.theme-fieldset legend {
  margin-bottom: 12px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.theme-options { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }

.theme-option {
  position: relative;
  display: grid;
  gap: 9px;
  padding: 10px;
  border: 1px solid var(--op-rule);
  background: var(--op-surface);
  cursor: pointer;
}

.theme-option:has(input:checked) { border-color: var(--op-accent); box-shadow: inset 0 2px 0 var(--op-accent); }
.theme-option input { position: absolute; opacity: 0; pointer-events: none; }

.theme-preview {
  display: grid;
  grid-template-columns: 28px 1fr;
  grid-template-rows: 12px 12px;
  gap: 4px;
  height: 54px;
  padding: 8px;
  border: 1px solid var(--op-rule);
  background: var(--op-bg);
}

.theme-preview i { display: block; background: var(--op-ink); opacity: 0.16; }
.theme-preview i:first-child { grid-row: 1 / 3; opacity: 0.42; }
.theme-preview.dark { background: #11100d; }
.theme-preview.dark i { background: #f5f0e5; opacity: 0.22; }
.theme-preview.dark i:first-child { opacity: 0.54; }
.theme-preview.system { background: linear-gradient(90deg, var(--op-bg) 50%, #11100d 50%); }

.theme-label {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--op-ink);
  font-size: 11px;
  font-weight: 650;
}

.passkey-row { padding-top: 18px; }
.enrolled {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--op-success);
  font-size: 12px;
  font-weight: 650;
}

@media (max-width: 900px) {
  .settings-page-mark { display: none; }
  .settings-shell { grid-template-columns: 1fr; gap: 24px; }
  .settings-nav {
    position: static;
    grid-template-columns: repeat(6, minmax(120px, 1fr));
    overflow-x: auto;
    border-bottom: 1px solid var(--op-rule-strong);
  }
  .settings-nav .nav-item { grid-template-columns: 1fr; justify-items: start; gap: 4px; border-bottom: 0; border-right: 1px solid var(--op-rule); }
  .settings-nav .nav-item::before { font-size: 8px; }
}

@media (max-width: 640px) {
  .pane-card.identity { grid-template-columns: auto 1fr; padding: 18px; }
  .identity-actions { grid-column: 1 / -1; }
  .service-info { grid-template-columns: 1fr; }
  .service-info > div:nth-child(odd) { padding-right: 0; border-right: 0; }
  .service-info > div:nth-child(even) { padding-left: 0; }
  .pwd-form,
  .theme-options { grid-template-columns: 1fr; }
  .password-field:first-child { grid-column: auto; }
  .setting-row { align-items: flex-start; flex-direction: column; gap: 12px; }
}
</style>

<style scoped>
.settings-shell {
  display: grid;
  grid-template-columns: 196px minmax(0, 1fr);
  align-items: start;
  gap: 16px;
}
.settings-nav {
  position: sticky;
  top: 96px;
  display: grid;
  gap: 3px;
  padding: 7px;
  border: 1px solid var(--gh-border);
  border-radius: var(--gh-radius-card);
  background: var(--gh-surface);
  box-shadow: var(--gh-shadow-sm);
}
.settings-nav .nav-item {
  display: flex;
  align-items: center;
  gap: 9px;
  min-height: 40px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--gh-radius-control);
  color: var(--gh-ink-soft);
  background: transparent;
  font: inherit;
  font-size: 13px;
  font-weight: 650;
  text-align: start;
  cursor: pointer;
  transition: color 160ms ease, background 160ms ease;
}
.settings-nav .nav-item svg { flex: none; }
.settings-nav .nav-item:hover { color: var(--gh-ink); background: var(--gh-surface-2); }
.settings-nav .nav-item[aria-current="true"] {
  color: var(--gh-primary);
  background: var(--gh-primary-soft);
  font-weight: 750;
}

.settings-panel { min-width: 0; overflow: hidden; }
.pane { display: grid; gap: 16px; align-content: start; padding: clamp(16px, 2.4vw, 26px); }
.pane-head { display: flex; align-items: center; gap: 6px; }
.pane-head h2 { margin: 0; font-size: 17px; }

.pane-card {
  padding: clamp(14px, 2vw, 20px);
  border: 1px solid var(--gh-border);
  border-radius: var(--gh-radius-control);
  background: var(--gh-surface-2);
}
.identity { display: flex; align-items: center; gap: 15px; min-width: 0; flex-wrap: wrap; }
.avatar-ring {
  display: grid;
  place-items: center;
  flex: 0 0 66px;
  height: 66px;
  border: 1px solid var(--gh-glint);
  border-radius: 50%;
  background: var(--gh-lens);
  box-shadow: var(--gh-shadow-sm);
}
.profile-avatar { color: var(--gh-primary); background: var(--gh-primary-soft); }
.profile-avatar :deep(.n-avatar__text) { inset: 0; display: grid; place-items: center; transform: none !important; }
.identity-copy { display: grid; gap: 4px; min-width: 0; flex: 1 1 180px; }
.identity-copy h2 { margin: 0; font-size: clamp(17px, 2vw, 22px); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.identity-copy p { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 6px; margin: 0; font-size: 12px; }
.identity-copy .dot { color: var(--gh-ink-faint); }
.identity-actions { flex: 0 0 auto; margin-left: auto; }

.setting-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; }
.setting-row > div { min-width: 0; }
.setting-row strong { display: inline-flex; align-items: center; gap: 4px; font-size: 14px; }
.setting-row p { margin: 3px 0 0; font-size: 12px; }
.setting-row :deep(.n-switch), .setting-row :deep(.n-button) { flex: none; }
.passkey-row { padding-top: 15px; border-top: 1px solid var(--gh-row-border); }
.enrolled { display: inline-flex; align-items: center; gap: 5px; color: var(--gh-success); font-weight: 700; white-space: nowrap; }

.theme-fieldset { min-width: 0; margin: 0; padding: 0; border: 0; }
.theme-fieldset legend { padding: 0 0 9px; font-size: 14px; font-weight: 700; }
.theme-options { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
.theme-option {
  position: relative;
  display: grid;
  gap: 7px;
  min-width: 0;
  padding: 6px;
  border: 1px solid var(--gh-border);
  border-radius: 13px;
  color: var(--gh-ink-soft);
  background: var(--gh-surface-2);
  cursor: pointer;
}
.theme-option:hover, .theme-option:has(input:checked) { border-color: var(--gh-primary); color: var(--gh-primary); }
.theme-option:has(input:checked) { background: var(--gh-primary-soft); box-shadow: inset 0 0 0 1px var(--gh-primary); }
.theme-option:has(input:focus-visible) { outline: 2px solid var(--gh-focus); outline-offset: 2px; }
.theme-option input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.theme-preview {
  display: grid;
  align-content: center;
  justify-items: start;
  gap: 4px;
  height: clamp(36px, 5vw, 57px);
  padding: 7px;
  border-radius: 8px;
  background: linear-gradient(140deg, #fff, #bdd8ff);
}
.theme-preview.dark { background: linear-gradient(140deg, #1d2c4a, #304b80); }
.theme-preview.system { background: linear-gradient(122deg, #dce8ff 49%, #253e6d 51%); }
.theme-preview i { width: 68%; height: 3px; border-radius: 5px; background: #5578bd; }
.theme-preview i:first-child { width: 44%; height: 5px; }
.theme-preview i:last-child { width: 54%; }
.theme-preview.dark i, .theme-preview.system i { background: #8da8da; }
.theme-label { display: flex; justify-content: center; align-items: center; gap: 4px; font-size: 12px; font-weight: 700; }
.theme-label svg { flex: none; }

.pwd-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--gh-border);
  border-radius: var(--gh-radius-control);
  background: var(--gh-surface-2);
}
.password-field { display: grid; gap: 5px; min-width: 0; }
.password-field:first-child { grid-column: 1 / -1; }
.password-field label { color: var(--gh-ink); font-size: 12px; font-weight: 650; }
.pwd-actions { grid-column: 1 / -1; display: flex; justify-content: end; flex-wrap: wrap; gap: 7px; }

.service-info { display: grid; margin: 0; padding: 0 12px; border-radius: var(--gh-radius-control); background: var(--gh-surface-2); }
.service-info > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; }
.service-info > div + div { border-top: 1px solid var(--gh-row-border); }
.service-info dt { display: inline-flex; align-items: center; gap: 4px; color: var(--gh-ink-soft); font-size: 12px; }
.service-info dd { margin: 0; font-size: 13px; font-weight: 650; overflow-wrap: anywhere; text-align: end; }
.settings :deep(.n-button) { min-height: 40px; }

@media (max-width: 860px) {
  .settings-shell { grid-template-columns: minmax(0, 1fr); gap: 10px; }
  .settings-nav {
    position: static;
    display: flex;
    gap: 4px;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .settings-nav::-webkit-scrollbar { display: none; }
  .settings-nav .nav-item { flex: 0 0 auto; min-height: 40px; padding: 0 12px; }
}
@media (max-width: 560px) {
  .pane { padding: 14px; }
  .identity-actions { width: 100%; margin-left: 0; }
  .settings :deep(.n-button) { min-height: 44px; }
  .setting-row { flex-wrap: wrap; }
  .pwd-form { grid-template-columns: minmax(0, 1fr); }
  .theme-label { font-size: 11px; }
}
</style>

<style scoped>
.settings .settings-page-head { margin-bottom: 32px; }

.settings .settings-shell {
  display: grid;
  grid-template-columns: 232px minmax(0, 1fr);
  align-items: start;
  gap: clamp(28px, 4vw, 64px);
}

.settings .settings-nav {
  position: sticky;
  top: 28px;
  display: grid;
  counter-reset: settings-section;
  align-items: stretch;
  gap: 0;
  border-top: 1px solid var(--op-rule-strong);
  border-right: 0;
}

.settings .settings-nav .nav-item {
  counter-increment: settings-section;
  display: grid;
  grid-template-columns: 24px 18px minmax(0, 1fr);
  align-items: center;
  justify-items: start;
  gap: 9px;
  width: auto;
  min-height: 50px;
  padding: 0 10px;
  border: 0;
  border-bottom: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 12px;
  font-weight: 600;
  text-align: left;
}

.settings .settings-nav .nav-item::before {
  content: counter(settings-section, decimal-leading-zero);
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
}

.settings .settings-nav .nav-item:hover {
  color: var(--op-ink);
  background: var(--op-surface);
}

.settings .settings-nav .nav-item[aria-current="true"] {
  color: var(--op-accent);
  background: var(--op-accent-soft);
  box-shadow: inset 2px 0 0 var(--op-accent);
}

.settings .settings-nav .nav-item[aria-current="true"]::before { color: var(--op-accent); }

.settings .settings-panel {
  padding: 0;
  overflow: visible;
  border: 0;
  background: transparent;
}

.settings .pane {
  display: grid;
  gap: 26px;
  align-content: start;
  padding: 0;
}

.settings .pane-card.identity {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 18px;
  padding: 24px;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
  border: 0;
  border-radius: 0;
}

.settings .avatar-ring { flex: none; }
.settings .profile-avatar {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  overflow: hidden;
  border-radius: 0 !important;
  background: var(--op-accent-soft);
  color: var(--op-accent);
  object-fit: cover;
}

.settings .profile-avatar :deep(svg) { color: var(--op-accent); }

.settings .profile-fallback {
  color: var(--op-accent-deep);
  font-family: var(--op-font-display);
  font-size: 27px;
  font-weight: 600;
}

.settings .identity-copy h2 {
  margin: 0;
  overflow: visible;
  font-family: var(--op-font-display);
  font-size: clamp(30px, 4vw, 44px);
  font-weight: 500;
  letter-spacing: -0.04em;
  line-height: 1.05;
  white-space: normal;
}

.settings .identity-copy p {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  margin: 7px 0 0;
  color: color-mix(in srgb, var(--op-feature-ink) 60%, transparent);
  font-size: 11px;
}

.settings .identity-copy .dot { color: var(--op-accent); }

.settings .identity-actions .n-button {
  --n-color: transparent !important;
  --n-text-color: var(--op-feature-ink) !important;
  --n-border: 1px solid color-mix(in srgb, var(--op-feature-ink) 35%, transparent) !important;
}

.settings .service-info {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--op-rule-strong);
  border-radius: 0;
  background: transparent;
}

.settings .service-info > div {
  display: grid;
  align-items: start;
  justify-content: initial;
  gap: 5px;
  padding: 18px 4px;
  border-bottom: 1px solid var(--op-rule);
}

.settings .service-info > div + div { border-top: 0; }
.settings .service-info > div:nth-child(odd) { padding-right: 24px; border-right: 1px solid var(--op-rule); }
.settings .service-info > div:nth-child(even) { padding-left: 24px; }

.settings .service-info dt {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.settings .service-info dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--op-ink);
  font-size: 14px;
  font-weight: 600;
  text-align: start;
}

.settings .pane-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--op-rule-strong);
}

.settings .pane-head h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-family: var(--op-font-display);
  font-size: clamp(34px, 4vw, 48px);
  font-weight: 500;
  letter-spacing: -0.045em;
}

.settings .setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 0;
  border-bottom: 1px solid var(--op-rule);
}

.settings .setting-row strong { color: var(--op-ink); font-size: 14px; }
.settings .setting-row .muted { margin: 5px 0 0; font-size: 12px; }

.settings .pwd-form {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
  padding: 20px;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  background: var(--op-surface);
}

.settings .password-field:first-child { grid-column: 1 / -1; }
.settings .pwd-actions { grid-column: 1 / -1; display: flex; justify-content: flex-end; gap: 8px; }

.settings .theme-fieldset { margin: 0; padding: 0; border: 0; }

.settings .theme-fieldset legend {
  margin-bottom: 12px;
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.settings .theme-options {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.settings .theme-option {
  display: grid;
  grid-template-columns: 1fr;
  gap: 9px;
  padding: 10px;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  background: var(--op-surface);
}

.settings .theme-option:has(input:checked) {
  border-color: var(--op-accent);
  background: var(--op-surface);
  color: var(--op-ink);
  box-shadow: inset 0 2px 0 var(--op-accent);
}

.settings .theme-preview {
  display: grid;
  grid-template-columns: 28px 1fr;
  grid-template-rows: 12px 12px;
  gap: 4px;
  height: 54px;
  padding: 8px;
  overflow: hidden;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  background: var(--op-bg);
}

.settings .theme-preview i { display: block; background: var(--op-ink); opacity: 0.16; }
.settings .theme-preview i:first-child { grid-row: 1 / 3; opacity: 0.42; }
.settings .theme-preview.light { background: #f2eee5; }
.settings .theme-preview.light i { background: #171612; }
.settings .theme-preview.dark { background: #11100d; }
.settings .theme-preview.dark i { background: #f5f0e5; opacity: 0.22; }
.settings .theme-preview.dark i:first-child { opacity: 0.54; }
.settings .theme-preview.system { background: linear-gradient(90deg, #f2eee5 50%, #11100d 50%); }
.settings .theme-preview.system i:first-child { background: linear-gradient(90deg, #171612 50%, #f5f0e5 50%); }

.settings .theme-label { display: flex; align-items: center; gap: 6px; font-size: 11px; }

.settings .settings-page-mark {
  display: grid;
  justify-items: end;
  gap: 3px;
  color: var(--op-ink-faint);
  font-size: 11px;
  text-align: right;
}

.settings .settings-page-mark .mono {
  color: var(--op-accent);
  font-size: 28px;
  letter-spacing: -0.06em;
  line-height: 1;
}

@media (max-width: 900px) {
  .settings .settings-page-mark { display: none; }
  .settings .settings-shell { grid-template-columns: 1fr; gap: 24px; }
  .settings .settings-nav {
    position: static;
    grid-template-columns: repeat(5, minmax(120px, 1fr));
    grid-auto-flow: column;
    overflow-x: auto;
    border-bottom: 1px solid var(--op-rule-strong);
  }
  .settings .settings-nav .nav-item {
    grid-template-columns: 1fr;
    gap: 4px;
    border-bottom: 0;
    border-right: 1px solid var(--op-rule);
  }
}

@media (max-width: 640px) {
  .settings .pane-card.identity { grid-template-columns: auto 1fr; padding: 18px; }
  .settings .identity-actions { grid-column: 1 / -1; width: auto; margin-left: 0; }
  .settings .service-info { grid-template-columns: 1fr; }
  .settings .service-info > div:nth-child(odd) { padding-right: 0; border-right: 0; }
  .settings .service-info > div:nth-child(even) { padding-left: 0; }
  .settings .pwd-form,
  .settings .theme-options { grid-template-columns: 1fr; }
  .settings .password-field:first-child { grid-column: auto; }
  .settings .setting-row { align-items: flex-start; flex-direction: column; gap: 12px; }
}
</style>

<style scoped>
.settings .settings-page-head {
  margin-bottom: 20px;
  padding-bottom: 0;
  animation: op-rise 240ms ease-out both;
}

.settings .settings-page-mark { display: none; }

.settings .settings-shell {
  display: grid;
  grid-template-columns: 216px minmax(0, 1fr);
  align-items: start;
  gap: 28px;
}

.settings .settings-nav {
  position: sticky;
  top: 16px;
  display: grid;
  gap: 3px;
  padding: 6px;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: #fff;
  box-shadow: var(--op-shadow-sm);
}

.settings .settings-nav .nav-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  min-height: 38px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 13px;
  font-weight: 500;
  text-align: left;
  box-shadow: none;
}

.settings .settings-nav .nav-item::before { display: none; }
.settings .settings-nav .nav-item:hover { color: var(--db-blue); background: var(--db-blue-soft); }
.settings .settings-nav .nav-item[aria-current="true"] { color: var(--db-blue); background: var(--db-blue-soft); font-weight: 600; box-shadow: none; }
.settings .settings-nav .nav-item svg { flex: none; }

.settings .settings-panel {
  padding: 0;
  overflow: visible;
  border: 0;
  background: transparent;
}

.settings .pane { display: grid; gap: 18px; align-content: start; padding: 0; }

.settings .pane-card.identity {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  color: var(--op-ink);
  background: #fff;
  border: 1px solid var(--op-rule);
  border-radius: 12px;
  box-shadow: var(--op-shadow-sm);
}

.settings .avatar-ring { flex: none; }

.settings .profile-avatar {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  overflow: hidden;
  border-radius: 50% !important;
  background: var(--db-blue-soft);
  color: var(--db-blue);
  object-fit: cover;
}

.settings .profile-fallback {
  font-size: 22px;
  font-weight: 600;
}

.settings .identity-copy { display: grid; flex: 1; gap: 4px; min-width: 0; }
.settings .identity-copy h2 { margin: 0; color: var(--op-ink); font-size: 22px; font-weight: 700; letter-spacing: -0.025em; line-height: 1.25; }
.settings .identity-copy p { display: flex; align-items: center; flex-wrap: wrap; gap: 5px; margin: 0; color: var(--op-ink-soft); font-size: 12px; }
.settings .identity-copy .dot { color: var(--op-ink-faint); }
.settings .identity-actions { margin-left: auto; }
.settings .identity-actions .n-button {
  --n-color: var(--db-blue) !important;
  --n-color-hover: var(--db-blue-hover) !important;
  --n-color-pressed: var(--db-blue-pressed) !important;
  --n-text-color: #ffffff !important;
  --n-text-color-hover: #ffffff !important;
  --n-text-color-pressed: #ffffff !important;
  --n-border: 1px solid var(--db-blue) !important;
}

.settings .service-info {
  display: block;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: #fff;
}

.settings .service-info > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 13px 16px;
  border: 0;
  border-bottom: 1px solid var(--op-rule);
}

.settings .service-info > div:last-child { border-bottom: 0; }
.settings .service-info dt { display: flex; align-items: center; gap: 4px; color: var(--op-ink-soft); font-size: 13px; font-weight: 500; }
.settings .service-info dd { margin: 0; color: var(--op-ink); font-size: 13px; font-weight: 600; text-align: right; overflow-wrap: anywhere; }

.settings .pane-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--op-rule);
}

.settings .pane-head h2 { margin: 0; color: var(--op-ink); font-size: 22px; font-weight: 700; letter-spacing: -0.025em; }

.settings .setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: #fff;
}

.settings .setting-row strong { color: var(--op-ink); font-size: 14px; font-weight: 600; }
.settings .setting-row .muted { margin: 4px 0 0; color: var(--op-ink-soft); font-size: 12px; }

.settings .pwd-form {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: #fff;
}

.settings .password-field { display: grid; gap: 6px; }
.settings .password-field:first-child { grid-column: 1 / -1; }
.settings .password-field label { color: var(--op-ink-soft); font-size: 12px; font-weight: 500; }
.settings .pwd-actions { grid-column: 1 / -1; display: flex; justify-content: flex-end; gap: 8px; }

.settings .theme-fieldset { margin: 0; padding: 16px; border: 1px solid var(--op-rule); border-radius: 10px; background: #fff; }
.settings .theme-fieldset legend { margin-bottom: 10px; color: var(--op-ink); font-size: 13px; font-weight: 600; }
.settings .theme-options { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }

.settings .theme-option {
  display: grid;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--op-rule);
  border-radius: 8px;
  background: #fff;
}

.settings .theme-option:hover { border-color: #b8ccf5; }
.settings .theme-option:has(input:checked) { border-color: var(--db-blue); background: var(--db-blue-soft); box-shadow: 0 0 0 1px var(--db-blue) inset; }

.settings .theme-preview {
  display: grid;
  grid-template-columns: 24px 1fr;
  grid-template-rows: 10px 10px;
  gap: 3px;
  height: 48px;
  padding: 7px;
  overflow: hidden;
  border: 1px solid var(--op-rule);
  border-radius: 6px;
}

.settings .theme-preview i { display: block; border-radius: 2px; background: #111827; opacity: 0.13; }
.settings .theme-preview i:first-child { grid-row: 1 / 3; opacity: 0.3; }
.settings .theme-preview.light { background: #f7f8fa; }
.settings .theme-preview.dark { background: #161d27; }
.settings .theme-preview.dark i { background: #f2f4f7; opacity: 0.18; }
.settings .theme-preview.system { background: linear-gradient(90deg, #f7f8fa 50%, #161d27 50%); }
.settings .theme-preview.system i:first-child { background: linear-gradient(90deg, #111827 50%, #f2f4f7 50%); }
.settings .theme-label { display: flex; align-items: center; gap: 5px; color: var(--op-ink); font-size: 12px; font-weight: 500; }
.settings .passkey-row { padding: 14px 16px; }
.settings .enrolled { display: inline-flex; align-items: center; gap: 6px; color: var(--op-success); font-size: 12px; font-weight: 600; }

@media (max-width: 900px) {
  .settings .settings-shell { grid-template-columns: 1fr; gap: 16px; }
  .settings .settings-nav {
    position: static;
    grid-template-columns: repeat(6, minmax(110px, 1fr));
    grid-auto-flow: column;
    overflow-x: auto;
    padding: 6px;
  }
  .settings .settings-nav .nav-item { min-width: 110px; justify-content: center; }
}

@media (max-width: 640px) {
  .settings .pane-card.identity { align-items: flex-start; flex-wrap: wrap; padding: 16px; }
  .settings .identity-actions { width: 100%; margin-left: 0; }
  .settings .pwd-form,
  .settings .theme-options { grid-template-columns: 1fr; }
  .settings .password-field:first-child { grid-column: auto; }
  .settings .setting-row { align-items: flex-start; flex-direction: column; gap: 10px; }
}
</style>

<style scoped>
.webdav-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 16px;
  border: 1px solid var(--op-rule);
  border-radius: 10px;
  background: #fff;
}

.webdav-summary > div { display: grid; gap: 4px; min-width: 0; }
.webdav-summary strong { color: var(--op-ink); font-size: 14px; font-weight: 600; }
.webdav-summary p { margin: 0; color: var(--op-ink-soft); font-size: 12px; }

.webdav-status {
  justify-self: start;
  padding: 2px 7px;
  border-radius: 999px;
  color: var(--op-ink-soft);
  background: var(--op-surface-muted);
  font-size: 11px;
  font-weight: 600;
}

.webdav-status.enabled { color: var(--op-success); background: var(--op-success-soft); }

.webdav-url {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 11px 13px;
  border: 1px solid var(--op-rule);
  border-radius: 8px;
  background: #f9fafb;
}

.webdav-url code {
  min-width: 0;
  overflow: hidden;
  color: var(--op-ink);
  font-family: var(--op-font-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.webdav-form { margin-top: 0; }
.form-spacer { flex: 1; }

@media (max-width: 640px) {
  .webdav-summary { align-items: flex-start; flex-direction: column; }
  .webdav-url { align-items: flex-start; flex-direction: column; }
  .webdav-url .n-button { align-self: flex-end; }
}
</style>
