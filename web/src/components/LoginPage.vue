<script setup lang="ts">
import { ref } from "vue";
import { NButton, NInput, NTooltip } from "naive-ui";
import AuthLayout from "./AuthLayout.vue";
import Icon from "./Icon.vue";
import { auth, gateLogin, refreshLoginStatus } from "../stores/auth";
import { setAuthToken } from "../api/client";
import { loginWithPasskey, passkeyEnrolled, passkeySupported, registerPasskey } from "../utils/passkey";

const emit = defineEmits<{ success: [] }>();

const password = ref("");
const error = ref("");
const busy = ref(false);

// 说明提示：桌面端 hover 显示；触屏没有 hover，改为点击切换
const helpOpen = ref(false);
const canHover = window.matchMedia("(hover: hover)").matches;

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
  try {
    await gateLogin(value);
    await refreshLoginStatus();
    // 密码登录成功：若本机未注册 Passkey，先展示注册引导
    if (hasPasskeySupport && !passkeyEnrolled()) {
      showEnroll.value = true;
    } else {
      emit("success");
    }
  } catch (e) {
    error.value = (e as Error).message || "访问密码错误，请重试";
  } finally {
    auth.loginInProgress = false;
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
    emit("success");
  }
}

function finishLogin(): void {
  emit("success");
}
</script>

<template>
  <AuthLayout title="登录 iPan">
    <form v-if="!showEnroll" class="form" @submit.prevent="submit">
      <div class="field-head">
        <label class="field-label" for="access-password">访问密码</label>
        <NTooltip :show="helpOpen" placement="top" :delay="0" @update:show="(v: boolean) => (helpOpen = v)">
          <template #trigger>
            <button
              type="button"
              class="help-btn"
              aria-label="登录说明"
              @mouseenter="canHover && (helpOpen = true)"
              @mouseleave="canHover && (helpOpen = false)"
              @click="helpOpen = !helpOpen"
              @blur="helpOpen = false"
            >
              <Icon name="help" :size="15" />
            </button>
          </template>
          <div class="help-tip">
            <p>输入访问密码，进入你的文件空间。</p>
            <p>访问密码在首次部署时设置，之后可在「设置 → 访问与安全」中修改。</p>
          </div>
        </NTooltip>
      </div>
      <NInput
        id="access-password"
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

      <template v-if="hasPasskeySupport">
        <div class="divider-row">
          <span class="divider-line"></span>
          <span class="divider-text">或</span>
          <span class="divider-line"></span>
        </div>
        <NButton
          size="large"
          block
          quaternary
          class="passkey-btn"
          :loading="passkeyBusy"
          :disabled="busy"
          @click="onPasskeyLogin"
        >
          使用 Passkey 登录
        </NButton>
      </template>
    </form>

    <div v-else class="enroll">
      <div class="enroll-title">登录成功</div>
      <p class="enroll-desc">注册本设备的 Passkey 后，下次打开无需输入密码，指纹/刷脸一键登录。</p>
      <NButton type="primary" size="large" block :loading="enrollBusy" class="submit" @click="onEnroll">
        注册并启用
      </NButton>
      <NButton size="large" block quaternary class="passkey-btn" @click="finishLogin">
        暂不，直接进入
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
