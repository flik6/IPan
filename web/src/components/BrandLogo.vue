<script setup lang="ts">
withDefaults(defineProps<{ subtitle?: string; size?: "sm" | "md" }>(), { subtitle: "", size: "md" });
</script>

<template>
  <div class="brand" :class="size">
    <img class="mark" src="/logo.svg" alt="iPan" draggable="false" />
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
  display: block;
  width: 40px;
  height: 40px;
  object-fit: contain;
  flex: 0 0 auto;
  -webkit-user-drag: none;
  user-select: none;
}
.sm .mark {
  width: 32px;
  height: 32px;
}
.text {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.2;
}
.word {
  font-size: 18px;
  font-weight: 750;
  letter-spacing: 0.01em;
  color: var(--gh-ink);
}
.sm .word {
  font-size: 16px;
}
.sub {
  margin-top: 4px;
  font-size: 11px;
  color: var(--gh-ink-soft);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
</style>
