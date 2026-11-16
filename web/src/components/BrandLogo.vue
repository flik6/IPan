<script setup lang="ts">
withDefaults(defineProps<{ subtitle?: string; size?: "sm" | "md" }>(), { subtitle: "", size: "md" });
</script>

<template>
  <div class="brand" :class="size">
    <span class="mark" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none">
        <path d="M4.5 7.5A2.5 2.5 0 0 1 7 5h3.2l1.8 2H17a2.5 2.5 0 0 1 2.5 2.5v7A2.5 2.5 0 0 1 17 19H7a2.5 2.5 0 0 1-2.5-2.5v-9Z" fill="currentColor"/>
        <path d="M8 11.5h8" stroke="#0061FF" stroke-width="1.5" stroke-linecap="round" opacity=".58"/>
      </svg>
    </span>
    <span class="text">
      <span class="word">iPan</span>
      <span v-if="subtitle" class="sub">{{ subtitle }}</span>
    </span>
  </div>
</template>

<style scoped>
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.mark {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  flex: 0 0 auto;
  border-radius: 9px;
  color: #fff;
  background: var(--db-blue);
}

.mark svg { width: 21px; height: 21px; }

.text {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.12;
}

.word {
  color: var(--op-ink);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.035em;
}

.sub {
  margin-top: 2px;
  color: var(--op-ink-faint);
  font-size: 11px;
  font-weight: 500;
  line-height: 1.25;
}

.sm .mark { width: 30px; height: 30px; border-radius: 8px; }
.sm .mark svg { width: 18px; height: 18px; }
.sm .word { font-size: 19px; }
.sm .sub { font-size: 10px; }
</style>
