<script setup lang="ts">
import { ref } from "vue";
import { NButton, NInput } from "naive-ui";
import AuthLayout from "./AuthLayout.vue";
import Icon from "./Icon.vue";
import { completeSetup, markSetupDone } from "../stores/auth";

const emit = defineEmits<{ done: [] }>();

const password = ref("");
const confirm = ref("");
const error = ref("");
const busy = ref(false);

async function submit(): Promise<void> {
  error.value = "";
  const pwd = password.value.trim();
  if (pwd.length < 4) {
    error.value = "访问密码至少 4 位";
    return;
  }
  if (pwd !== confirm.value.trim()) {
    error.value = "两次输入的密码不一致";
    return;
  }
  busy.value = true;
  try {
    await completeSetup({ password: pwd });
    // 先把 done 交给父组件切换界面，再关闭初始化引导：
    // 顺序反了会让本组件先被卸载，emit 就再也到达不了父组件。
    emit("done");
    markSetupDone();
  } catch (e) {
    error.value = (e as Error).message || "初始化失败";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <AuthLayout title="设置访问密码" subtitle="首次使用需要设置访问密码，之后可用它登录 iPan。">
    <form class="form" @submit.prevent="submit">
      <div class="field">
        <label class="field-label" for="setup-password">访问密码</label>
        <NInput
          id="setup-password"
          v-model:value="password"
          type="password"
          show-password-on="click"
          size="large"
          placeholder="至少 4 位"
          :disabled="busy"
          autofocus
        >
          <template #prefix><Icon name="shield" :size="17" /></template>
        </NInput>
      </div>

      <div class="field">
        <label class="field-label" for="setup-confirm">确认密码</label>
        <NInput
          id="setup-confirm"
          v-model:value="confirm"
          type="password"
          show-password-on="click"
          size="large"
          placeholder="再输入一次"
          :disabled="busy"
          @keydown.enter.prevent="submit"
        >
          <template #prefix><Icon name="check" :size="17" /></template>
        </NInput>
      </div>

      <p class="hint">
        文件床签名密钥（SIGN_PARAM）会自动随机生成并保存，无需手动配置；云盘账号可稍后在设置页绑定。
      </p>

      <div v-if="error" class="error" role="alert">
        <Icon name="warning" :size="15" />
        <span>{{ error }}</span>
      </div>

      <NButton type="primary" size="large" block :loading="busy" attr-type="submit" class="submit">
        完成初始化
      </NButton>
    </form>
  </AuthLayout>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.field-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--gh-ink);
}
.hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--gh-ink-soft);
}
.error {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 9px 11px;
  border-radius: 10px;
  background: var(--gh-danger-bg);
  border: 1px solid var(--gh-danger-border);
  color: var(--gh-danger);
  font-size: 13px;
}
.submit {
  margin-top: 6px;
  height: 42px;
  border-radius: 12px;
  font-weight: 600;
}
</style>

<style scoped>
.form { display: grid; gap: 17px; }
.field { display: grid; gap: 7px; }
.field-label { margin: 0; }

.hint {
  margin: 0;
  padding: 11px 13px;
  border-left: 2px solid var(--op-accent);
  color: var(--op-ink-soft);
  background: var(--op-accent-soft);
  font-size: 11px;
  line-height: 1.7;
}

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
  height: 46px;
  margin-top: 4px;
  border-radius: 0 !important;
  font-weight: 650;
}
</style>

<style scoped>
.form { display: grid; gap: 15px; }
.field { display: grid; gap: 7px; }
.field-label { margin: 0; }
.hint { margin: 0; padding: 10px 12px; border: 1px solid #c7dbff; border-radius: 8px; color: var(--op-ink-soft); background: var(--db-blue-soft); font-size: 12px; line-height: 1.6; }
.error { display: flex; align-items: flex-start; gap: 7px; padding: 10px 12px; border: 1px solid #fecdca; border-radius: 8px; color: var(--op-danger); background: var(--op-danger-soft); font-size: 12px; line-height: 1.5; }
.submit { width: 100%; height: 44px; margin-top: 2px; border-radius: 8px !important; }
</style>
