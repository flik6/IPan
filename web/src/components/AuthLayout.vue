<script setup lang="ts">
import BrandLogo from "./BrandLogo.vue";

withDefaults(defineProps<{ title: string; subtitle?: string; variant?: "workspace" | "share" }>(), {
  variant: "workspace",
});
</script>

<template>
  <div class="auth-page" :class="{ 'share-page': variant === 'share' }">
    <header class="auth-header">
      <BrandLogo subtitle="个人云盘" />
    </header>

    <main class="auth-main">
      <section v-if="variant === 'workspace'" class="auth-promo">
        <div class="auth-promo-copy">
          <h2>所有文件，<br />一个地方。</h2>
        </div>

        <div class="auth-art" aria-hidden="true">
          <svg viewBox="0 0 430 300" fill="none">
            <rect x="76" y="30" width="138" height="174" rx="10" fill="#FFFFFF" stroke="#D0D5DD"/>
            <rect x="104" y="53" width="82" height="8" rx="4" fill="#E4E7EC"/>
            <rect x="104" y="70" width="58" height="7" rx="3.5" fill="#EEF0F3"/>
            <rect x="104" y="86" width="70" height="7" rx="3.5" fill="#EEF0F3"/>
            <rect x="178" y="74" width="142" height="180" rx="10" fill="#FFFFFF" stroke="#D0D5DD"/>
            <rect x="203" y="98" width="92" height="8" rx="4" fill="#E4E7EC"/>
            <rect x="203" y="115" width="64" height="7" rx="3.5" fill="#EEF0F3"/>
            <rect x="203" y="131" width="78" height="7" rx="3.5" fill="#EEF0F3"/>
            <rect x="203" y="147" width="52" height="7" rx="3.5" fill="#EEF0F3"/>
            <path d="M39 150.5c0-10.2 8.3-18.5 18.5-18.5h47.2c5.4 0 10.6 2.4 14.1 6.5l9 10.5h183.7c10.2 0 18.5 8.3 18.5 18.5v76.9c0 10.2-8.3 18.5-18.5 18.5H57.5c-10.2 0-18.5-8.3-18.5-18.5v-93.9Z" fill="#0061FF"/>
            <path d="M39 171.5c0-6.4 5.2-11.5 11.5-11.5h329c6.4 0 11.5 5.2 11.5 11.5v76.9c0 10.2-8.3 18.5-18.5 18.5H57.5c-10.2 0-18.5-8.3-18.5-18.5v-76.9Z" fill="#0061FF"/>
            <rect x="100" y="197" width="118" height="9" rx="4.5" fill="#FFFFFF" opacity=".9"/>
            <rect x="100" y="216" width="76" height="7" rx="3.5" fill="#FFFFFF" opacity=".5"/>
            <circle cx="326" cy="205" r="16" fill="#FFFFFF" opacity=".18"/>
            <circle cx="326" cy="205" r="7" fill="#FFFFFF" opacity=".9"/>
          </svg>
        </div>
      </section>

      <section class="auth-card">
        <span v-if="variant === 'share'" class="auth-kicker">iPan 文件分享</span>
        <h1 class="auth-title">{{ title }}</h1>
        <p v-if="subtitle" class="auth-subtitle">{{ subtitle }}</p>
        <div class="auth-body"><slot /></div>
        <div v-if="$slots.footer" class="auth-foot"><slot name="footer" /></div>
      </section>
    </main>
  </div>
</template>

<style scoped>
.auth-page {
  display: grid;
  grid-template-rows: auto 1fr;
  min-height: 100dvh;
  padding: 20px var(--op-gutter) 36px;
  background: var(--db-bg);
}

.auth-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  width: 100%;
  max-width: 1180px;
  margin-inline: auto;
  padding-bottom: 16px;
}

.auth-header-note { color: var(--op-ink-faint); font-size: 12px; }

.auth-main {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(360px, 430px);
  align-items: center;
  gap: clamp(40px, 7vw, 100px);
  width: 100%;
  max-width: 1080px;
  margin-inline: auto;
  padding-block: clamp(24px, 5vh, 54px);
}

.auth-promo { min-width: 0; }
.auth-promo-copy { max-width: 480px; }

.auth-badge {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 9px;
  border-radius: 999px;
  color: var(--db-blue);
  background: var(--db-blue-soft);
  font-size: 12px;
  font-weight: 600;
}

.auth-promo h2 {
  margin: 0;
  color: var(--op-ink);
  font-size: clamp(40px, 4.8vw, 58px);
  font-weight: 700;
  letter-spacing: -0.045em;
  line-height: 1.08;
}

.auth-promo p {
  max-width: 46ch;
  margin: 14px 0 0;
  color: var(--op-ink-soft);
  font-size: 15px;
  line-height: 1.65;
}

.auth-points {
  display: grid;
  gap: 8px;
  margin: 22px 0 0;
  padding: 0;
  list-style: none;
  color: var(--op-ink-soft);
  font-size: 13px;
}

.auth-points li { display: flex; align-items: center; gap: 9px; }

.point-check {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  color: var(--db-blue);
  background: var(--db-blue-soft);
  font-size: 11px;
  font-weight: 700;
}

.auth-art {
  width: min(100%, 390px);
  margin-top: 24px;
}

.auth-art svg { width: 100%; height: auto; }

.auth-card {
  width: 100%;
  min-width: 0;
  padding: clamp(26px, 3.4vw, 38px);
  border: 1px solid var(--op-rule);
  border-radius: 12px;
  background: #ffffff;
  box-shadow: var(--op-shadow-sm);
}

.auth-kicker {
  display: block;
  color: var(--db-blue);
  font-size: 12px;
  font-weight: 600;
}

.auth-title {
  margin: 0;
  color: var(--op-ink);
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.2;
}

.auth-subtitle {
  margin: 9px 0 0;
  color: var(--op-ink-soft);
  font-size: 13px;
  line-height: 1.6;
}

.auth-body { margin-top: 24px; }
.auth-body :deep(.n-input) { --n-height: 44px !important; }
.auth-body :deep(.n-button) { min-height: 44px; }

.auth-foot {
  margin-top: 18px;
  color: var(--op-ink-faint);
  font-size: 12px;
  text-align: center;
}

.share-page .auth-main {
  display: block;
  max-width: 720px;
  padding-top: clamp(32px, 6vh, 66px);
}

.share-page .auth-card { width: min(660px, 100%); margin-inline: auto; }

@media (max-width: 900px) {
  .auth-main { grid-template-columns: 1fr; max-width: 480px; gap: 24px; }
  .auth-promo { display: none; }
  .auth-header-note { display: none; }
  .auth-card { justify-self: stretch; }
}

@media (max-width: 520px) {
  .auth-page { padding: 16px 16px 26px; }
  .auth-card { padding: 22px 18px; }
  .auth-title { font-size: 25px; }
}
</style>
