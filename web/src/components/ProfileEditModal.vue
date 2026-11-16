<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NAvatar, NButton, NDatePicker, NInput, NModal, NRadio, NRadioGroup } from "naive-ui";
import Icon from "./Icon.vue";
import { api, toBirthday, type UpdateProfilePayload, type UserProfile } from "../api/client";
import { applyProfile, loadWopanProfile } from "../composables/useWopanUser";
import { auth } from "../stores/auth";
import { showToast } from "../stores/toast";
import { maskPhone } from "../utils/format";

const props = defineProps<{ show: boolean }>();
const emit = defineEmits<{ "update:show": [boolean]; saved: [] }>();

const loading = ref(false);
const saving = ref(false);
const uploading = ref(false);
const phone = ref("");
const userName = ref("");
const sex = ref("");
const headUrl = ref("");
const birthday = ref<number | null>(null);
/** 已上传但尚未随表单提交的新头像 URL */
const pendingHeadUrl = ref("");
const fileInput = ref<HTMLInputElement | null>(null);
/** 保存失败时就地展示的错误，弹窗内可见 */
const errorMessage = ref("");

const avatarPreview = computed(() => pendingHeadUrl.value || headUrl.value || undefined);
const editable = computed(() => auth.cloudConfigured);

/**
 * 联通云盘要求昵称、性别、生日三项必须齐全，缺任意一项整次提交都会被拒。
 * 这里按必填项处理：未填满就禁用「保存」，并在底部提示还缺什么。
 */
const missingFields = computed(() => {
  const missing: string[] = [];
  if (!userName.value.trim()) missing.push("昵称");
  if (!sex.value) missing.push("性别");
  if (birthday.value === null) missing.push("生日");
  return missing;
});
const missingHint = computed(() =>
  missingFields.value.length ? `请先填写：${missingFields.value.join("、")}` : "",
);
const canSave = computed(
  () => editable.value && !saving.value && !uploading.value && !loading.value && missingFields.value.length === 0,
);

async function loadProfile(): Promise<void> {
  loading.value = true;
  try {
    const p = await loadWopanProfile();
    fillFromProfile(p);
  } catch (e) {
    showToast((e as Error).message || "读取账号资料失败", "error");
  } finally {
    loading.value = false;
  }
}

function fillFromProfile(p: UserProfile): void {
  phone.value = p.phone || "";
  userName.value = p.userName || "";
  sex.value = p.sex || "";
  headUrl.value = p.headUrl || "";
  birthday.value = toBirthday(p.birthday);
  pendingHeadUrl.value = "";
  errorMessage.value = "";
}

watch(
  () => props.show,
  (show) => {
    if (show) void loadProfile();
  },
);

function close(): void {
  if (saving.value) return;
  emit("update:show", false);
}

function pickAvatar(): void {
  if (!editable.value || uploading.value) return;
  fileInput.value?.click();
}

async function onFileChange(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  if (!/^image\//.test(file.type)) {
    showToast("请选择图片文件（JPG / PNG / GIF / BMP）", "error");
    return;
  }
  if (file.size > 10 * 1024 * 1024) {
    showToast("头像文件不能超过 10MB", "error");
    return;
  }

  uploading.value = true;
  errorMessage.value = "";
  try {
    const form = new FormData();
    form.set("file", file);
    const data = await api<{ headUrl: string }>("/api/user/avatar", { method: "POST", body: form });
    if (!data?.headUrl) throw new Error("上传成功但未返回头像地址");
    pendingHeadUrl.value = data.headUrl;
    showToast("头像已上传，点击「保存」后生效", "success");
  } catch (e) {
    errorMessage.value = (e as Error).message || "头像上传失败";
  } finally {
    uploading.value = false;
  }
}

async function save(): Promise<void> {
  if (!editable.value) return;
  errorMessage.value = "";
  const name = userName.value.trim();
  if (!name || !sex.value || birthday.value === null) {
    errorMessage.value = missingHint.value || "请填写完整后再保存";
    return;
  }

  const payload: UpdateProfilePayload = { userName: name, sex: sex.value, birthday: birthday.value };
  if (pendingHeadUrl.value) payload.headUrl = pendingHeadUrl.value;

  saving.value = true;
  try {
    await api("/api/user/profile", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    applyProfile({
      userName: name,
      sex: sex.value,
      headUrl: pendingHeadUrl.value || headUrl.value,
      birthday: birthday.value,
    });
    showToast("账号信息已更新", "success");
    emit("saved");
    emit("update:show", false);
  } catch (e) {
    // 弹窗内的操作失败就地提示，避免提示条被弹窗遮住看不到
    errorMessage.value = (e as Error).message || "保存失败";
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NModal :show="show" @update:show="(v: boolean) => !v && close()">
    <div class="profile-box glass-panel">
      <div class="box-head">
        <span>账号信息</span>
        <button type="button" class="icon-btn" title="关闭" @click="close">
          <Icon name="close" :size="17" />
        </button>
      </div>

      <div class="box-body">
        <div class="avatar-col">
          <NAvatar :src="avatarPreview" round :size="80" class="profile-avatar">
            <Icon name="user" :size="36" />
          </NAvatar>
          <NButton size="tiny" quaternary :loading="uploading" :disabled="!editable" @click="pickAvatar">
            更换头像
          </NButton>
          <input ref="fileInput" type="file" accept="image/*" hidden @change="onFileChange" />
          <p v-if="pendingHeadUrl" class="muted tiny">新头像待保存</p>
        </div>

        <div class="fields">
          <label class="field">
            <span class="label">手机号码</span>
            <span class="static">{{ maskPhone(phone) || "—" }}</span>
          </label>

          <label class="field">
            <span class="label"><span class="required">*</span>昵称</span>
            <NInput v-model:value="userName" placeholder="请输入昵称" :maxlength="30" :disabled="!editable" />
          </label>

          <div class="field">
            <span class="label"><span class="required">*</span>性别</span>
            <NRadioGroup v-model:value="sex" :disabled="!editable">
              <NRadio value="男">男</NRadio>
              <NRadio value="女">女</NRadio>
            </NRadioGroup>
          </div>

          <div class="field">
            <span class="label"><span class="required">*</span>生日</span>
            <NDatePicker
              v-model:value="birthday"
              type="date"
              clearable
              size="small"
              placeholder="请选择日期"
              :disabled="!editable"
            />
          </div>
        </div>
      </div>

      <div class="box-foot">
        <p v-if="errorMessage" class="inline-error" role="alert">{{ errorMessage }}</p>
        <p v-else-if="missingHint" class="inline-hint">{{ missingHint }}</p>
        <p v-else class="muted tiny">
          {{ editable ? "带 * 的项为必填，修改会同步到联通云盘官方账号。" : "请先绑定云盘账号后再修改资料。" }}
        </p>
        <div class="actions">
          <NButton :disabled="saving" @click="close">取消</NButton>
          <NButton type="primary" :loading="saving" :disabled="!canSave" @click="save">保存</NButton>
        </div>
      </div>

      <div v-if="loading" class="loading-mask">
        <span>正在读取账号资料…</span>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.profile-avatar { background: var(--gh-primary-soft); color: var(--gh-primary); }
.profile-box {
  position: relative;
  border-radius: var(--gh-radius-card);
  box-shadow: var(--gh-shadow-lg);
  width: min(560px, calc(100vw - 40px));
  overflow: hidden;
}
.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--gh-border);
  font-size: 14px;
  font-weight: 700;
}
.icon-btn {
  border: 0;
  background: none;
  padding: 4px;
  border-radius: 8px;
  color: var(--gh-ink-soft);
  cursor: pointer;
  display: grid;
  place-items: center;
}
.icon-btn:hover {
  background: var(--gh-primary-soft);
  color: var(--gh-primary);
}
.box-body {
  display: flex;
  gap: 22px;
  padding: 20px 18px;
}
.avatar-col {
  width: 96px;
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.fields {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.field {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
}
.field .label {
  font-size: 12px;
  color: var(--gh-ink-soft);
  text-align: right;
}
.required {
  color: var(--gh-danger);
  margin-right: 2px;
}
.static {
  font-size: 13px;
}
.muted {
  color: var(--gh-ink-soft);
}
.tiny {
  margin: 0;
  font-size: 11px;
  line-height: 1.5;
  grid-column: 2;
}
.inline-error {
  margin: 0;
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--gh-danger);
}
.inline-hint {
  margin: 0;
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--gh-warning);
}
.box-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 18px 16px;
  border-top: 1px solid var(--gh-border);
}
.actions {
  display: flex;
  gap: 8px;
  flex: 0 0 auto;
}
.loading-mask {
  position: absolute;
  inset: 0;
  background: rgba(255, 255, 255, 0.72);
  display: grid;
  place-items: center;
  font-size: 13px;
  color: var(--gh-ink-soft);
}
@media (max-width: 560px) {
  .box-body {
    flex-direction: column;
    align-items: center;
  }
  .fields {
    width: 100%;
  }
  .field {
    grid-template-columns: 56px minmax(0, 1fr);
  }
  .box-foot {
    flex-direction: column;
    align-items: stretch;
  }
  .actions {
    justify-content: flex-end;
  }
}
</style>

<style scoped>
.profile-box { position: relative; width: min(620px, calc(100vw - 28px)); max-height: min(86vh, 740px); overflow: auto; border: 1px solid var(--op-rule); border-radius: 12px; background: #fff; box-shadow: var(--op-shadow-lg); }
.box-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 18px; border-bottom: 1px solid var(--op-rule); color: var(--op-ink); font-size: 18px; font-weight: 600; }
.icon-btn { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 8px; color: var(--op-ink-soft); background: transparent; cursor: pointer; }
.icon-btn:hover { color: var(--db-blue); background: var(--db-blue-soft); }
.box-body { display: grid; grid-template-columns: 132px minmax(0, 1fr); gap: 22px; padding: 20px 18px; }
.avatar-col { display: grid; justify-items: start; align-content: start; gap: 9px; padding-right: 18px; border-right: 1px solid var(--op-rule); }
.profile-avatar { border-radius: 50% !important; background: var(--db-blue-soft); color: var(--db-blue); }
.fields { display: grid; gap: 14px; }
.field { display: grid; gap: 6px; }
.label { color: var(--op-ink); font-size: 12px; font-weight: 600; }
.required { color: var(--op-danger); }
.static { color: var(--op-ink-soft); font-size: 13px; }
.box-foot { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 12px 18px; border-top: 1px solid var(--op-rule); }
.inline-error { margin: 0; color: var(--op-danger); font-size: 12px; }
.inline-hint { margin: 0; color: var(--op-warning); font-size: 12px; }
.actions { display: flex; gap: 8px; margin-left: auto; }
.loading-mask { position: absolute; inset: 0; display: grid; place-items: center; background: rgba(255,255,255,.82); color: var(--op-ink-soft); font-size: 12px; }
@media (max-width: 600px) { .box-body { grid-template-columns: 1fr; } .avatar-col { grid-template-columns: auto 1fr; align-items: center; padding-right: 0; padding-bottom: 14px; border-right: 0; border-bottom: 1px solid var(--op-rule); } .box-foot { align-items: flex-start; flex-direction: column; } .actions { width: 100%; justify-content: flex-end; } }
</style>

<style scoped>
.profile-box {
  position: relative;
  width: min(650px, calc(100vw - 28px));
  max-height: min(86vh, 760px);
  overflow: auto;
  border: 1px solid var(--op-rule-strong);
  border-top: 3px solid var(--op-accent);
  border-radius: 0;
  background: var(--op-surface);
  box-shadow: var(--op-shadow-lg);
}

.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 15px 19px;
  border-bottom: 1px solid var(--op-rule-strong);
  color: var(--op-ink);
  font-family: var(--op-font-display);
  font-size: 27px;
  font-weight: 500;
  letter-spacing: -0.04em;
}

.icon-btn {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  padding: 0;
  border: 1px solid var(--op-rule);
  border-radius: 0;
  color: var(--op-ink-soft);
  background: transparent;
  cursor: pointer;
}

.icon-btn:hover { color: var(--op-accent); border-color: var(--op-accent); }

.box-body {
  display: grid;
  grid-template-columns: 150px minmax(0, 1fr);
  gap: 24px;
  padding: 22px 19px;
}

.avatar-col { display: grid; justify-items: start; align-content: start; gap: 10px; padding-right: 20px; border-right: 1px solid var(--op-rule); }
.profile-avatar { border-radius: 0 !important; background: var(--op-accent-soft); color: var(--op-accent); }
.avatar-col .n-button { --n-color: transparent !important; --n-text-color: var(--op-accent) !important; }
.fields { display: grid; gap: 17px; }
.field { display: grid; gap: 7px; }
.label { color: var(--op-ink-soft); font-family: var(--op-font-mono); font-size: 9px; font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; }
.required { color: var(--op-accent); }
.static { color: var(--op-ink); font-size: 13px; }

.box-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 19px;
  border-top: 1px solid var(--op-rule-strong);
}

.inline-error { margin: 0; color: var(--op-danger); font-size: 11px; }
.inline-hint { margin: 0; color: var(--op-warning); font-size: 11px; }
.actions { display: flex; gap: 8px; margin-left: auto; }

.loading-mask {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: color-mix(in srgb, var(--op-surface) 80%, transparent);
  color: var(--op-ink-soft);
  font-family: var(--op-font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

@media (max-width: 600px) {
  .box-body { grid-template-columns: 1fr; }
  .avatar-col { grid-template-columns: auto 1fr; align-items: center; padding-right: 0; padding-bottom: 16px; border-right: 0; border-bottom: 1px solid var(--op-rule); }
  .box-foot { align-items: flex-start; flex-direction: column; }
  .actions { width: 100%; justify-content: flex-end; }
}
</style>
