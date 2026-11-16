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
  return ts ? new Date(ts).toLocaleString("zh-CN") : "-";
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
          <span class="status-icon" :class="{ warn: paused || lastError }">
            <Icon :name="paused || lastError ? 'warning' : 'check'" :size="18" />
          </span>
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
            <span class="capacity-value">{{ wopanUser.loaded ? `${formatSize(wopanUser.usedSize)} / ${formatSize(wopanUser.totalSize)}` : "加载中…" }}</span>
          </div>
          <NProgress
            type="line"
            :percentage="usagePercent"
            :show-indicator="false"
            :height="7"
            aria-label="联通云盘已用容量"
            border-radius="999px"
            color="var(--gh-primary)"
            rail-color="var(--gh-border)"
          />
        </div>

        <dl class="meta-grid">
          <div class="meta">
            <dt>手机号</dt>
            <dd>{{ wopanUser.phone ? maskPhone(wopanUser.phone) : "—" }}</dd>
          </div>
          <div class="meta">
            <dt>自动刷新</dt>
            <dd>{{ auto?.enabled ? `每 ${Math.max(1, Math.round((auto.intervalHours || 48) / 24))} 天` : "未启用" }}</dd>
          </div>
          <div class="meta">
            <dt>上次刷新</dt>
            <dd>{{ formatTs(auto?.lastSuccessAt) }}</dd>
          </div>
          <div class="meta">
            <dt>下次刷新</dt>
            <dd>{{ formatTs(auto?.nextRefreshAt) }}</dd>
          </div>
        </dl>

        <NAlert v-if="paused || lastError" type="warning" :bordered="false" class="alert">
          {{ lastError || "检测到账号状态异常，自动刷新已暂停。" }}
          <template v-if="paused"> 请先在官方客户端完成验证，再重新登录。</template>
        </NAlert>

        <div class="status-actions">
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
        <div class="form-field">
          <label for="cloud-phone">手机号</label>
          <NInput v-model:value="phone" placeholder="请输入 11 位手机号" size="large" :disabled="sending || verifying" maxlength="11" :input-props="{ id: 'cloud-phone', autocomplete: 'tel', inputmode: 'tel' }">
            <template #prefix><Icon name="user" :size="16" /></template>
          </NInput>
        </div>

        <div class="form-field">
          <label for="cloud-code">短信验证码</label>
          <div class="form-row">
          <NInput
            v-model:value="code"
            placeholder="请输入验证码"
            :input-props="{ id: 'cloud-code', autocomplete: 'one-time-code', inputmode: 'numeric' }"
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
        </div>

        <NButton type="primary" size="large" block :loading="verifying" class="submit-btn" @click="verify">
          登录
        </NButton>
      </div>

      <div v-if="error" class="form-error" role="alert">{{ error }}</div>
      <div v-else-if="info" class="form-info" role="status">{{ info }}</div>

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
  border-radius: 16px;
  background: var(--gh-surface-2);
  padding: clamp(13px, 2vw, 17px);
}
.status-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.status-icon {
  display: grid;
  place-items: center;
  flex: 0 0 37px;
  height: 37px;
  border-radius: 12px;
  color: var(--gh-success);
  background: var(--ac-green-soft);
}
.status-icon.warn { color: var(--gh-warning); background: var(--ac-orange-soft); }
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
  margin-top: 14px;
  padding: 12px;
  border-radius: 12px;
  background: var(--gh-surface);
}
.usage-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 2px 8px;
  font-size: 12px;
  font-weight: 650;
  margin-bottom: 6px;
}
.capacity-value { color: var(--gh-ink-soft); font-variant-numeric: tabular-nums; font-weight: 500; }
.meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 18px;
  margin: 13px 0 0;
  padding: 0;
}
.meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
}
.meta dt { color: var(--gh-ink-soft); font-size: 11px; }
.meta dd { margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.alert {
  margin-top: 14px;
  font-size: 12px;
}
.status-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
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
.form-field { display: grid; gap: 5px; min-width: 0; }
.form-field label { font-size: 12px; font-weight: 650; }
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
  .meta-grid { gap: 10px; }
}
</style>

<style scoped>
.cloud-account { display: grid; gap: 16px; }
.status-card { padding: 18px; border: 1px solid var(--op-rule); border-radius: 12px; color: var(--op-ink); background: #fff; }
.status-head { display: flex; align-items: center; gap: 10px; }
.status-icon { display: grid; place-items: center; flex: 0 0 36px; height: 36px; border: 0; border-radius: 8px; color: var(--db-blue); background: var(--db-blue-soft); }
.status-icon.warn { color: var(--op-warning); background: var(--op-warning-soft); }
.status-title { display: grid; gap: 2px; flex: 1; min-width: 0; }
.status-title strong { color: var(--op-ink); font-size: 14px; font-weight: 600; }
.status-title .muted { color: var(--op-ink-soft); font-size: 12px; }
.usage { margin-top: 18px; }
.usage-line { display: flex; justify-content: space-between; gap: 12px; color: var(--op-ink-soft); font-size: 12px; }
.capacity-value { color: var(--op-ink); font-size: 12px; font-weight: 500; }
.usage :deep(.n-progress) { margin-top: 7px; }
.meta-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0; margin: 18px 0 0; border-top: 1px solid var(--op-rule); }
.meta { display: grid; gap: 4px; min-width: 0; padding: 11px 10px 0 0; border-right: 1px solid var(--op-rule); }
.meta + .meta { padding-left: 10px; }
.meta:last-child { border-right: 0; }
.meta dt { color: var(--op-ink-faint); font-size: 11px; }
.meta dd { margin: 0; color: var(--op-ink); font-size: 12px; overflow-wrap: anywhere; }
.alert { margin-top: 14px; }
.status-actions { display: flex; justify-content: flex-end; margin-top: 12px; }
.login-head { display: flex; align-items: flex-start; gap: 11px; padding-bottom: 14px; border-bottom: 1px solid var(--op-rule); }
.login-icon { display: grid; place-items: center; flex: 0 0 38px; height: 38px; border: 0; border-radius: 8px; color: var(--db-blue); background: var(--db-blue-soft); }
.login-head strong { color: var(--op-ink); font-size: 16px; font-weight: 600; }
.login-head p { margin: 3px 0 0; color: var(--op-ink-soft); font-size: 12px; }
.form { display: grid; gap: 14px; }
.form-field { display: grid; gap: 6px; }
.form-field label { color: var(--op-ink); font-size: 12px; font-weight: 600; }
.form-row { display: flex; gap: 8px; }
.form-row .n-input { flex: 1; }
.send-btn, .submit-btn { border-radius: 8px !important; }
.form-error, .form-info { padding: 9px 11px; border: 1px solid #fecdca; border-radius: 8px; color: var(--op-danger); background: var(--op-danger-soft); font-size: 12px; }
.form-info { border-color: #abefc6; color: var(--op-success); background: var(--op-success-soft); }
.login-foot { display: flex; justify-content: space-between; gap: 10px; color: var(--op-ink-faint); font-size: 11px; }
.link-btn { padding: 0; border: 0; color: var(--db-blue); background: transparent; font-size: 12px; cursor: pointer; }
@media (max-width: 720px) { .meta-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .meta:nth-child(2) { border-right: 0; } }
@media (max-width: 520px) { .status-card { padding: 15px; } .meta-grid { grid-template-columns: 1fr; } .meta, .meta + .meta { padding: 8px 0; border-right: 0; border-top: 1px solid var(--op-rule); } .meta:first-child { border-top: 0; } .form-row { flex-direction: column; } .send-btn { width: 100%; } }
</style>

<style scoped>
.cloud-account { display: grid; gap: 20px; }

.status-card {
  padding: 22px;
  border: 0;
  border-radius: 0;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
}

.status-head { display: flex; align-items: center; gap: 11px; }

.status-icon {
  display: grid;
  place-items: center;
  flex: 0 0 38px;
  height: 38px;
  border: 1px solid color-mix(in srgb, var(--op-feature-ink) 28%, transparent);
  border-radius: 0;
  color: var(--op-feature-ink);
  background: transparent;
}

.status-icon.warn { color: var(--op-warning); }
.status-title { display: grid; gap: 3px; flex: 1; min-width: 0; }
.status-title strong { color: var(--op-feature-ink); font-size: 14px; }
.status-title .muted { color: color-mix(in srgb, var(--op-feature-ink) 58%, transparent); font-size: 11px; }

.usage { margin-top: 24px; }
.usage-line { display: flex; justify-content: space-between; gap: 12px; color: color-mix(in srgb, var(--op-feature-ink) 62%, transparent); font-size: 11px; }
.capacity-value { color: var(--op-feature-ink); font-family: var(--op-font-mono); font-size: 10px; }
.usage :deep(.n-progress) { margin-top: 8px; }
.usage :deep(.n-progress-graph-line-rail) { background: color-mix(in srgb, var(--op-feature-ink) 17%, transparent) !important; }
.usage :deep(.n-progress-graph-line-fill) { background: var(--op-accent) !important; }

.meta-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0;
  margin: 22px 0 0;
  border-top: 1px solid color-mix(in srgb, var(--op-feature-ink) 20%, transparent);
}

.meta {
  display: grid;
  gap: 5px;
  min-width: 0;
  padding: 12px 12px 0 0;
  border-right: 1px solid color-mix(in srgb, var(--op-feature-ink) 16%, transparent);
}

.meta + .meta { padding-left: 12px; }
.meta:last-child { border-right: 0; }
.meta dt { color: color-mix(in srgb, var(--op-feature-ink) 48%, transparent); font-family: var(--op-font-mono); font-size: 8px; letter-spacing: 0.08em; text-transform: uppercase; }
.meta dd { margin: 0; color: var(--op-feature-ink); font-size: 11px; overflow-wrap: anywhere; }

.alert { margin-top: 18px; }
.status-actions { display: flex; justify-content: flex-end; margin-top: 16px; }
.status-actions .n-button { --n-color: transparent !important; --n-text-color: var(--op-feature-ink) !important; }

.login-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding-bottom: 18px;
  border-bottom: 1px solid var(--op-rule-strong);
}

.login-icon {
  display: grid;
  place-items: center;
  flex: 0 0 42px;
  height: 42px;
  border: 1px solid var(--op-rule);
  color: var(--op-accent);
  background: var(--op-accent-soft);
}

.login-head strong { color: var(--op-ink); font-family: var(--op-font-display); font-size: 24px; font-weight: 500; letter-spacing: -0.03em; }
.login-head p { margin: 4px 0 0; color: var(--op-ink-soft); font-size: 11px; }

.form { display: grid; gap: 15px; }
.form-field { display: grid; gap: 7px; }
.form-field label { color: var(--op-ink-soft); font-family: var(--op-font-mono); font-size: 9px; font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; }
.form-row { display: flex; gap: 8px; }
.form-row .n-input { flex: 1; }
.send-btn { min-width: 112px; border-radius: 0 !important; }
.submit-btn { border-radius: 0 !important; }

.form-error,
.form-info {
  padding: 10px 12px;
  border-left: 2px solid var(--op-danger);
  color: var(--op-danger);
  background: var(--op-danger-soft);
  font-size: 11px;
}

.form-info { border-left-color: var(--op-success); color: var(--op-success); background: var(--op-success-soft); }
.login-foot { display: flex; justify-content: space-between; gap: 10px; padding-top: 4px; color: var(--op-ink-faint); font-size: 10px; }
.link-btn { padding: 0; border: 0; color: var(--op-accent); background: transparent; font-size: 11px; cursor: pointer; }

@media (max-width: 720px) {
  .meta-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .meta:nth-child(2) { border-right: 0; }
  .meta:nth-child(n + 3) { margin-top: 10px; padding-top: 12px; border-top: 1px solid color-mix(in srgb, var(--op-feature-ink) 16%, transparent); }
}

@media (max-width: 520px) {
  .status-card { padding: 17px; }
  .meta-grid { grid-template-columns: 1fr; }
  .meta,
  .meta + .meta { padding: 9px 0; border-right: 0; border-top: 1px solid color-mix(in srgb, var(--op-feature-ink) 16%, transparent); }
  .meta:first-child { border-top: 0; }
  .form-row { flex-direction: column; }
  .send-btn { width: 100%; }
}
</style>
