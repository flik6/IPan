<script setup lang="ts">
import { ref } from "vue";
import { NButton, NInput } from "naive-ui";
import AuthLayout from "./AuthLayout.vue";
import Icon from "./Icon.vue";
import { auth, gateLogin, refreshLoginStatus } from "../stores/auth";
import { setAuthToken } from "../api/client";
import { loginWithPasskey, passkeyEnrolled, passkeySupported, registerPasskey } from "../utils/passkey";

const emit = defineEmits<{ success: [] }>();

const password = ref("");
const error = ref("");
const busy = ref(false);

// Passkey 相关状态（支持性在 setup 顶层求值一次即可）
const hasPasskeySupport = passkeySupported();
const passkeyBusy = ref(false);
const showEnroll = ref(false);
const enrollBusy = ref(false);
const enrollNote = ref("");

async function submit(): Promise<void> {
  error.value = "";
  const value = password.value.trim();
  if (!value) {
    error.value = "请输入访问密码";
    return;
  }
  busy.value = true;
  auth.loginInProgress = true;
  let keepLoginStage = false;
  try {
    await gateLogin(value);
    await refreshLoginStatus();
    // 密码登录成功：若本机未注册 Passkey，先展示注册引导
    if (hasPasskeySupport && !passkeyEnrolled()) {
      showEnroll.value = true;
      keepLoginStage = true;
    } else {
      emit("success");
    }
  } catch (e) {
    error.value = (e as Error).message || "访问密码错误，请重试";
  } finally {
    if (!keepLoginStage) auth.loginInProgress = false;
    busy.value = false;
  }
}

/** Passkey 免密登录（不设置 auth.ready，由父组件处理） */
async function onPasskeyLogin(): Promise<void> {
  error.value = "";
  passkeyBusy.value = true;
  try {
    const token = await loginWithPasskey();
    setAuthToken(token);
    auth.loginInProgress = true;
    try {
      await refreshLoginStatus();
      emit("success");
    } finally {
      auth.loginInProgress = false;
    }
  } catch (e) {
    error.value = (e as Error).message || "Passkey 登录失败";
  } finally {
    passkeyBusy.value = false;
  }
}

/** 注册本设备的 Passkey；无论成败都进入应用 */
async function onEnroll(): Promise<void> {
  enrollBusy.value = true;
  try {
    await registerPasskey();
    enrollNote.value = "";
  } catch (e) {
    enrollNote.value = `注册未完成：${(e as Error).message || "请稍后在设置页重试"}（不影响本次进入）`;
  } finally {
    enrollBusy.value = false;
    auth.loginInProgress = false;
    emit("success");
  }
}

function finishLogin(): void {
  auth.loginInProgress = false;
  emit("success");
}
</script>

<template>
  <AuthLayout title="登录 iPan">
    <form v-if="!showEnroll" class="form" @submit.prevent="submit">
      <NInput
        v-model:value="password"
        type="password"
        show-password-on="click"
        size="large"
        placeholder="请输入访问密码"
        :disabled="busy"
        autofocus
        @keydown.enter.prevent="submit"
      >
        <template #prefix>
          <Icon name="shield" :size="17" />
        </template>
      </NInput>

      <div v-if="error" class="error" role="alert">
        <Icon name="warning" :size="15" />
        <span>{{ error }}</span>
      </div>

      <NButton type="primary" size="large" block :loading="busy" attr-type="submit" class="submit">
        登录
      </NButton>

      <button
        v-if="hasPasskeySupport"
        type="button"
        class="passkey-link"
        :disabled="busy || passkeyBusy"
        @click="onPasskeyLogin"
      >
        {{ passkeyBusy ? "正在验证…" : "使用 Passkey 登录" }}
      </button>
    </form>

    <div v-else class="enroll">
      <div class="enroll-title">登录成功</div>
      <p class="enroll-desc">注册 Passkey，下次可免密登录。</p>
      <NButton type="primary" size="large" block :loading="enrollBusy" class="submit" @click="onEnroll">
        注册
      </NButton>
      <NButton size="large" block quaternary class="passkey-btn" @click="finishLogin">
        暂不
      </NButton>
      <div v-if="enrollNote" class="enroll-note">{{ enrollNote }}</div>
    </div>
  </AuthLayout>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
}
.field-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}
.field-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--gh-ink);
}
.help-btn {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: none;
  color: var(--gh-ink-soft);
  cursor: help;
  transition: color 0.18s ease, background-color 0.18s ease;
}
.help-btn:hover,
.help-btn:focus-visible {
  color: var(--gh-primary);
  background: var(--gh-primary-soft);
  outline: none;
}
.help-tip {
  max-width: 240px;
  font-size: 12px;
  line-height: 1.65;
}
.help-tip p {
  margin: 0;
}
.help-tip p + p {
  margin-top: 6px;
}
.help-tip code {
  padding: 1px 4px;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.16);
  font-size: 11px;
}
.error {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 12px;
  padding: 9px 11px;
  border-radius: 10px;
  background: var(--gh-danger-bg);
  border: 1px solid var(--gh-danger-border);
  color: var(--gh-danger);
  font-size: 13px;
}
.submit {
  margin-top: 20px;
  height: 42px;
  border-radius: 12px;
  font-weight: 600;
}
.divider-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 16px;
}
.divider-line {
  flex: 1;
  height: 1px;
  background: var(--gh-border);
}
.divider-text {
  font-size: 12px;
  color: var(--gh-ink-soft);
}
.passkey-btn {
  margin-top: 12px;
  height: 42px;
  border-radius: 12px;
  font-weight: 600;
}
.enroll {
  display: flex;
  flex-direction: column;
}
.enroll-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--gh-ink);
}
.enroll-desc {
  margin: 8px 0 4px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--gh-ink-soft);
}
.enroll-note {
  margin-top: 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--gh-ink-soft);
}
</style>

<style scoped>
.form { display: grid; gap: 16px; margin: 0; }
.field-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 2px; }
.field-label { margin: 0; }

.help-btn {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  color: var(--op-ink-faint);
  background: transparent;
  cursor: help;
}

.help-btn:hover { color: var(--op-accent); }
.help-tip p { margin: 0; font-size: 12px; line-height: 1.65; }
.help-tip p + p { margin-top: 7px; }

.error {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-left: 2px solid var(--op-danger);
  color: var(--op-danger);
  background: var(--op-danger-soft);
  font-size: 12px;
  line-height: 1.55;
}

.submit {
  width: 100%;
  min-height: 46px;
  margin-top: 4px;
  border-radius: 0 !important;
  font-weight: 650;
}

.divider-row {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 10px;
  margin: 2px 0;
}

.divider-line { height: 1px; background: var(--op-rule); }
.divider-text {
  color: var(--op-ink-faint);
  font-family: var(--op-font-mono);
  font-size: 9px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.passkey-btn {
  width: 100%;
  min-height: 46px;
  border-radius: 0 !important;
  color: var(--op-ink) !important;
  background: transparent !important;
  border: 1px solid var(--op-rule-strong) !important;
}

.passkey-btn:hover { color: var(--op-accent) !important; border-color: var(--op-accent) !important; }
.enroll { display: grid; gap: 12px; }
.enroll-title {
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.045em;
  line-height: 1;
}

.enroll-desc { margin: 0; color: var(--op-ink-soft); font-size: 13px; line-height: 1.7; }
.enroll-note { margin: 0; padding: 10px 12px; border-left: 2px solid var(--op-warning); background: var(--op-warning-soft); font-size: 11px; }
</style>

<style scoped>
.form { display: grid; gap: 15px; margin: 0; }
.field-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-bottom: 2px; }
.field-label { margin: 0; }
.help-btn { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 6px; color: var(--op-ink-faint); background: transparent; cursor: help; }
.help-btn:hover { color: var(--db-blue); background: var(--db-blue-soft); }
.help-tip p { margin: 0; font-size: 12px; line-height: 1.6; }
.help-tip p + p { margin-top: 6px; }
.error { display: flex; align-items: flex-start; gap: 7px; padding: 10px 12px; border: 1px solid #fecdca; border-radius: 8px; color: var(--op-danger); background: var(--op-danger-soft); font-size: 12px; line-height: 1.5; }
.submit { width: 100%; min-height: 44px; margin-top: 2px; border-radius: 8px !important; }
.divider-row { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 10px; }
.divider-line { height: 1px; background: var(--op-rule); }
.divider-text { color: var(--op-ink-faint); font-size: 11px; }
.passkey-btn { width: 100%; min-height: 44px; border-radius: 8px !important; color: var(--op-ink) !important; background: #fff !important; border: 1px solid var(--op-rule-strong) !important; }
.passkey-btn:hover { color: var(--db-blue) !important; border-color: var(--db-blue) !important; }
.enroll { display: grid; gap: 12px; }
.enroll-title { color: var(--op-ink); font-size: 22px; font-weight: 700; letter-spacing: -0.025em; line-height: 1.2; }
.enroll-desc { margin: 0; color: var(--op-ink-soft); font-size: 13px; line-height: 1.6; }
.enroll-note { margin: 0; padding: 10px 12px; border: 1px solid #fedf89; border-radius: 8px; background: var(--op-warning-soft); font-size: 12px; }
</style>

<style scoped>
.form { display: grid; gap: 12px; }
.submit { margin-top: 0; }
.passkey-link {
  justify-self: center;
  padding: 4px 2px;
  border: 0;
  color: var(--op-ink-soft);
  background: transparent;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
}
.passkey-link:hover { color: var(--db-blue); }
.passkey-link:disabled { opacity: 0.55; cursor: not-allowed; }
.enroll { display: grid; gap: 10px; }
.enroll-desc { margin: 0 0 4px; color: var(--op-ink-soft); font-size: 13px; }
</style>
