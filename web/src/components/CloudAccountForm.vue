<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { NAlert, NButton, NInput, NProgress, NTag } from "naive-ui";
import Icon from "./Icon.vue";
import { api } from "../api/client";
import { auth, refreshLoginStatus } from "../stores/auth";
import { wopanUser, refreshWopanUser } from "../composables/useWopanUser";
import { showToast } from "../stores/toast";
import { formatSize, maskPhone } from "../utils/format";

const props = withDefaults(defineProps<{ mode?: "setup" | "settings" }>(), { mode: "settings" });
const emit = defineEmits<{ configured: [] }>();

const phone = ref("");
const code = ref("");
const error = ref("");
const info = ref("");
const sending = ref(false);
const verifying = ref(false);
const refreshing = ref(false);
const showLogin = ref(props.mode === "setup" || !auth.cloudConfigured);
const countdown = ref(0);
let timer: ReturnType<typeof setInterval> | null = null;

const configured = computed(() => auth.cloudConfigured);
const auto = computed(() => auth.cloud);
const paused = computed(() => !!auto.value?.paused);
const lastError = computed(() => auto.value?.lastError || "");
const accountName = computed(() => wopanUser.nickName || maskPhone(wopanUser.phone) || "已绑定账号");
const usagePercent = computed(() =>
  wopanUser.totalSize ? Math.min(100, (wopanUser.usedSize / wopanUser.totalSize) * 100) : 0,
);

function formatTs(ts?: number | null): string {
  return ts ? new Date(ts).toLocaleString() : "-";
}

function stopCountdown(): void {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
}

async function load(): Promise<void> {
  await refreshLoginStatus();
  // 账号信息有本地缓存，refreshWopanUser 默认命中缓存就不再回源
  if (auth.cloudConfigured) await refreshWopanUser();
  if (!auth.cloudConfigured && props.mode === "settings") showLogin.value = true;
}

async function sendCode(): Promise<void> {
  error.value = "";
  info.value = "";
  const p = phone.value.trim();
  if (!/^1\d{10}$/.test(p)) {
    error.value = "请输入正确的 11 位手机号";
    return;
  }
  sending.value = true;
  try {
    await api("/api/login/send-code", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ phone: p }),
    });
    info.value = "验证码已发送，请查看短信";
    let left = 60;
    countdown.value = left;
    stopCountdown();
    timer = setInterval(() => {
      left -= 1;
      countdown.value = left;
      if (left <= 0) stopCountdown();
    }, 1000);
  } catch (e) {
    error.value = (e as Error).message || "验证码发送失败";
  } finally {
    sending.value = false;
  }
}

async function verify(): Promise<void> {
  error.value = "";
  info.value = "";
  const p = phone.value.trim();
  const c = code.value.trim();
  if (!/^1\d{10}$/.test(p)) {
    error.value = "请输入正确的 11 位手机号";
    return;
  }
  if (!c) {
    error.value = "请输入短信验证码";
    return;
  }
  verifying.value = true;
  try {
    await api("/api/login/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ phone: p, code: c }),
    });
    await refreshLoginStatus();
    await refreshWopanUser();
    code.value = "";
    showLogin.value = false;
    showToast("云盘账号绑定成功", "success");
    emit("configured");
  } catch (e) {
    error.value = (e as Error).message || "登录失败";
  } finally {
    verifying.value = false;
  }
}

async function manualRefresh(): Promise<void> {
  refreshing.value = true;
  try {
    await api("/api/refresh", { method: "POST" });
    await refreshLoginStatus();
    // 令牌换新后账号信息也应回源一次，避免展示旧数据
    await refreshWopanUser({ force: true });
    showToast("云盘令牌已刷新", "success");
  } catch (e) {
    showToast((e as Error).message, "error");
    await load();
  } finally {
    refreshing.value = false;
  }
}

function replaceAccount(): void {
  showLogin.value = true;
  error.value = "";
  info.value = "";
  if (!phone.value && wopanUser.phone) phone.value = wopanUser.phone;
}

onMounted(() => void load());
onUnmounted(stopCountdown);
</script>

<template>
  <div class="cloud-account">
    <template v-if="configured && !showLogin">
      <div class="status-card">
        <div class="status-head">
          <span class="status-dot" :class="{ warn: paused || lastError }" />
          <div class="status-title">
            <strong>{{ paused ? "自动保活已暂停" : "云盘已绑定" }}</strong>
            <span class="muted">{{ accountName }}</span>
          </div>
          <NTag v-if="wopanUser.vipLevel" size="small" type="warning" :bordered="false">
            VIP{{ wopanUser.vipLevel }}
          </NTag>
        </div>

        <div class="usage">
          <div class="usage-line">
            <span>云盘容量</span>
            <span class="muted">{{ wopanUser.loaded ? `${formatSize(wopanUser.usedSize)} / ${formatSize(wopanUser.totalSize)}` : "加载中…" }}</span>
          </div>
          <NProgress
            type="line"
            :percentage="usagePercent"
            :show-indicator="false"
            :height="8"
            border-radius="999px"
            color="var(--gh-primary)"
            rail-color="var(--gh-border)"
          />
        </div>

        <div class="meta-grid">
          <div class="meta">
            <span class="meta-label">手机号</span>
            <span>{{ wopanUser.phone ? maskPhone(wopanUser.phone) : "-" }}</span>
          </div>
          <div class="meta">
            <span class="meta-label">自动刷新</span>
            <span>{{ auto?.enabled ? `每 ${Math.max(1, Math.round((auto.intervalHours || 48) / 24))} 天` : "未启用" }}</span>
          </div>
          <div class="meta">
            <span class="meta-label">上次刷新</span>
            <span>{{ formatTs(auto?.lastSuccessAt) }}</span>
          </div>
          <div class="meta">
            <span class="meta-label">下次刷新</span>
            <span>{{ formatTs(auto?.nextRefreshAt) }}</span>
          </div>
        </div>

        <NAlert v-if="paused || lastError" type="warning" :bordered="false" class="alert">
          {{ lastError || "检测到账号状态异常，自动刷新已暂停。" }}
          <template v-if="paused"> 请先在官方客户端完成验证，再重新登录。</template>
        </NAlert>

        <div class="status-actions">
          <NButton size="small" :loading="refreshing" @click="manualRefresh">
            <template #icon><Icon name="refresh" :size="15" /></template>
            手动刷新令牌
          </NButton>
          <NButton size="small" quaternary @click="replaceAccount">更换账号</NButton>
        </div>
      </div>
    </template>

    <template v-else>
      <div v-if="mode === 'settings'" class="login-head">
        <div class="login-icon"><Icon name="cloud" :size="22" /></div>
        <div>
          <strong>{{ configured ? "重新绑定云盘账号" : "绑定云盘账号" }}</strong>
          <p class="muted">使用手机号 + 短信验证码登录，令牌自动保活。</p>
        </div>
      </div>

      <div class="form">
        <NInput v-model:value="phone" placeholder="手机号" size="large" :disabled="sending || verifying" maxlength="11">
          <template #prefix><Icon name="user" :size="16" /></template>
        </NInput>

        <div class="form-row">
          <NInput
            v-model:value="code"
            placeholder="短信验证码"
            size="large"
            :disabled="verifying"
            @keydown.enter.prevent="verify"
          />
          <NButton
            size="large"
            :loading="sending"
            :disabled="countdown > 0 || verifying"
            class="send-btn"
            @click="sendCode"
          >
            {{ countdown > 0 ? `${countdown}s` : "获取验证码" }}
          </NButton>
        </div>

        <NButton type="primary" size="large" block :loading="verifying" class="submit-btn" @click="verify">
          登录
        </NButton>
      </div>

      <div v-if="error" class="form-error">{{ error }}</div>
      <div v-else-if="info" class="form-info">{{ info }}</div>

      <div class="login-foot">
        <button v-if="configured" type="button" class="link-btn" @click="showLogin = false">取消</button>
        <span v-else class="muted">验证码仅用于云盘登录绑定。</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.cloud-account {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.status-card {
  border: 1px solid var(--gh-border);
  border-radius: 12px;
  background: var(--gh-surface-2);
  padding: 16px;
}
.status-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.status-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: var(--gh-success);
  box-shadow: 0 0 0 4px rgba(22, 163, 74, 0.12);
  flex: 0 0 auto;
}
.status-dot.warn {
  background: var(--gh-warning);
  box-shadow: 0 0 0 4px rgba(255, 169, 64, 0.16);
}
.status-title {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.status-title strong {
  font-size: 14px;
}
.status-title .muted {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.usage {
  margin-top: 16px;
}
.usage-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  margin-bottom: 6px;
}
.usage-line .muted {
  font-size: 12px;
}
.meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 18px;
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px dashed var(--gh-border);
}
.meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
}
.meta-label {
  font-size: 11px;
  color: var(--gh-ink-soft);
}
.alert {
  margin-top: 14px;
  font-size: 12px;
}
.status-actions {
  display: flex;
  gap: 8px;
  margin-top: 14px;
}
.login-head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.login-icon {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  border-radius: 12px;
  color: var(--gh-primary);
  background: var(--gh-primary-soft);
  flex: 0 0 auto;
}
.login-head strong {
  font-size: 14px;
}
.login-head p {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.7;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.form-row {
  display: flex;
  gap: 10px;
}
.form-row > :first-child {
  flex: 1 1 auto;
  min-width: 0;
}
.send-btn {
  flex: 0 0 auto;
  min-width: 112px;
}
.submit-btn {
  font-weight: 600;
}
.form-error {
  padding: 8px 11px;
  border-radius: 10px;
  background: var(--gh-danger-bg);
  border: 1px solid var(--gh-danger-border);
  color: var(--gh-danger);
  font-size: 12px;
}
.form-info {
  font-size: 12px;
  color: var(--gh-success);
}
.login-foot {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  min-height: 20px;
}
.link-btn {
  border: 0;
  background: none;
  color: var(--gh-ink-soft);
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}
.link-btn:hover {
  color: var(--gh-primary);
}
@media (max-width: 560px) {
  .send-btn {
    min-width: 96px;
  }
  .meta-grid {
    grid-template-columns: 1fr;
  }
}
</style>
