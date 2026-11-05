<script setup lang="ts">
import BrandLogo from "./BrandLogo.vue";
import Icon from "./Icon.vue";

defineProps<{ title: string; subtitle?: string }>();
</script>

<template>
  <div class="auth-page">
    <section class="auth-aside">
      <div class="aside-glow one" />
      <div class="aside-glow two" />
      <BrandLogo subtitle="Cloud Drive" />
      <div class="aside-body">
        <h1>把云盘<br />变成你的私有文件站</h1>
        <p>用 Go 打造的轻量文件管理器，Docker 一键部署，支持直链、分享与在线预览。</p>
        <ul>
          <li><span class="dot"><Icon name="files" :size="15" /></span>多级目录与批量操作</li>
          <li><span class="dot"><Icon name="link" :size="15" /></span>稳定直链与文件床</li>
          <li><span class="dot"><Icon name="shield" :size="15" /></span>令牌自动保活</li>
        </ul>
      </div>
      <div class="aside-foot">Go · Vue · Docker</div>
    </section>

    <section class="auth-main">
      <div class="auth-card">
        <div class="mobile-brand"><BrandLogo /></div>
        <h2>{{ title }}</h2>
        <p v-if="subtitle" class="sub">{{ subtitle }}</p>
        <div class="auth-body"><slot /></div>
        <div v-if="$slots.footer" class="auth-foot"><slot name="footer" /></div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: grid;
  grid-template-columns: minmax(380px, 44%) 1fr;
  background: var(--gh-bg);
}
.auth-aside {
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 40px 46px;
  color: #fff;
  background: linear-gradient(160deg, #8e9bff 0%, #6673f6 45%, #4f5de8 100%);
}
.aside-glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(20px);
  pointer-events: none;
}
.aside-glow.one {
  width: 560px;
  height: 560px;
  right: -180px;
  top: -160px;
  background: radial-gradient(closest-side, rgba(255, 255, 255, 0.35), transparent);
}
.aside-glow.two {
  width: 480px;
  height: 480px;
  left: -160px;
  bottom: -140px;
  background: radial-gradient(closest-side, rgba(255, 214, 236, 0.3), transparent);
}
.auth-aside :deep(.word) {
  color: #fff;
}
.auth-aside :deep(.sub) {
  color: rgba(255, 255, 255, 0.55);
}
.auth-aside :deep(.mark) {
  filter: drop-shadow(0 6px 18px rgba(31, 20, 60, 0.28));
}
.aside-body {
  position: relative;
  z-index: 1;
  margin: auto 0;
  max-width: 440px;
}
.aside-body h1 {
  margin: 0 0 18px;
  font-size: 36px;
  line-height: 1.22;
  font-weight: 750;
  letter-spacing: 0.01em;
}
.aside-body p {
  margin: 0 0 26px;
  color: rgba(255, 255, 255, 0.72);
  line-height: 1.8;
  font-size: 14px;
}
.aside-body ul {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 14px;
}
.aside-body li {
  display: flex;
  align-items: center;
  gap: 10px;
  color: rgba(255, 255, 255, 0.86);
  font-size: 14px;
}
.dot {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.14);
}
.aside-foot {
  position: relative;
  z-index: 1;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  letter-spacing: 0.08em;
}
.auth-main {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 24px;
}
.auth-card {
  width: min(420px, 100%);
  background: var(--gh-surface);
  border: 1px solid var(--gh-border);
  border-radius: 22px;
  box-shadow: var(--gh-shadow-lg);
  padding: 34px 32px 28px;
}
.mobile-brand {
  display: none;
  margin-bottom: 22px;
}
.auth-card h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 750;
  letter-spacing: 0.01em;
}
.auth-body {
  margin-top: 22px;
}
.auth-card .sub {
  margin: 8px 0 0;
  color: var(--gh-ink-soft);
  font-size: 13px;
  line-height: 1.7;
}
.auth-card .sub + .auth-body {
  margin-top: 24px;
}
.auth-foot {
  margin-top: 20px;
  color: var(--gh-ink-soft);
  font-size: 12px;
  text-align: center;
}
@media (max-width: 900px) {
  .auth-page {
    grid-template-columns: 1fr;
  }
  .auth-aside {
    display: none;
  }
  .mobile-brand {
    display: block;
  }
  .auth-main {
    padding: 24px 16px;
  }
  .auth-card {
    padding: 28px 22px 24px;
  }
}
</style>
