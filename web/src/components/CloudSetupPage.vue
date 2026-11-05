<script setup lang="ts">
import AuthLayout from "./AuthLayout.vue";
import CloudAccountForm from "./CloudAccountForm.vue";

const emit = defineEmits<{ done: []; skip: [] }>();
</script>

<template>
  <AuthLayout
    title="绑定云盘账号"
    subtitle="还差一步：绑定手机号后即可在网页里管理文件。"
  >
    <CloudAccountForm mode="setup" @configured="emit('done')" />

    <template #footer>
      <button type="button" class="skip" @click="emit('skip')">稍后配置，先进入文件页</button>
    </template>
  </AuthLayout>
</template>

<style scoped>
.skip {
  border: 0;
  background: none;
  color: var(--gh-ink-soft);
  font-size: 12px;
  cursor: pointer;
  padding: 4px;
}
.skip:hover {
  color: var(--gh-primary);
}
</style>
