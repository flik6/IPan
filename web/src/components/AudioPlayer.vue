<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

const props = defineProps<{ src: string }>();
/** ready：拿到元数据可播放；error：加载失败，供外层收起加载动画 */
const emit = defineEmits<{ ready: []; error: [] }>();

const audio = ref<HTMLAudioElement | null>(null);
const playing = ref(false);
const current = ref(0);
const duration = ref(0);

function toggle(): void {
  const el = audio.value;
  if (!el) return;
  if (el.paused) void el.play();
  else el.pause();
}

function fmt(t: number): string {
  if (!Number.isFinite(t)) return "00:00";
  const m = Math.floor(t / 60);
  const s = Math.floor(t % 60);
  return String(m).padStart(2, "0") + ":" + String(s).padStart(2, "0");
}

function onTimeUpdate(): void {
  current.value = audio.value?.currentTime || 0;
}

function onMeta(): void {
  duration.value = audio.value?.duration || 0;
  emit("ready");
}

function onError(): void {
  emit("error");
}

function onPlay(): void {
  playing.value = true;
}

function onPause(): void {
  playing.value = false;
}

function seek(event: MouseEvent): void {
  const track = event.currentTarget as HTMLElement;
  const rect = track.getBoundingClientRect();
  const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width));
  if (audio.value && Number.isFinite(duration.value)) audio.value.currentTime = ratio * duration.value;
}

onMounted(() => {
  const el = audio.value;
  if (!el) return;
  el.addEventListener("play", onPlay);
  el.addEventListener("pause", onPause);
  el.addEventListener("ended", onPause);
  el.addEventListener("timeupdate", onTimeUpdate);
  el.addEventListener("loadedmetadata", onMeta);
  el.addEventListener("error", onError);
});

onUnmounted(() => {
  audio.value?.pause();
});

const fillPercent = computed(() => (duration.value ? (current.value / duration.value) * 100 : 0));

// props.src 由模板直接引用
void props;
</script>

<template>
  <div class="audio-player">
    <audio ref="audio" :src="src" preload="auto" style="display: none"></audio>
    <button class="ap-toggle" type="button" aria-label="播放" @click="toggle">{{ playing ? "⏸" : "▶" }}</button>
    <div class="ap-track" role="progressbar" @click="seek">
      <div class="ap-fill" :style="{ width: fillPercent + '%' }"></div>
      <div class="ap-knob" :style="{ left: fillPercent + '%' }"></div>
    </div>
    <span class="ap-time">{{ fmt(current) }} / {{ fmt(duration) }}</span>
  </div>
</template>

<style scoped>
.audio-player {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  margin: 4px 0;
  padding: 10px 14px;
  background: var(--gh-surface-2);
  border: 1px solid var(--gh-border);
  border-radius: 16px;
  box-sizing: border-box;
}
.ap-toggle {
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  border: 0;
  border-radius: 50%;
  background: var(--gh-action);
  color: var(--gh-on-action);
  font-size: 14px;
  cursor: pointer;
}
.ap-track {
  position: relative;
  flex: 1 1 auto;
  height: 8px;
  background: var(--gh-track);
  border-radius: 6px;
  cursor: pointer;
}
.ap-fill {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 0%;
  background: var(--gh-primary);
  border-radius: 6px;
}
.ap-knob {
  position: absolute;
  top: 50%;
  left: 0%;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--gh-primary-deep);
  border: 2px solid #fff;
  transform: translate(-50%, -50%);
  pointer-events: none;
}
.ap-time {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--gh-ink-soft);
  font-variant-numeric: tabular-nums;
}
</style>

<style scoped>
.audio-player { width: min(520px, 100%); padding: 12px 14px; border: 1px solid #334155; border-radius: 10px; background: #1f2937; }
.ap-toggle { width: 36px; height: 36px; border-radius: 8px; color: #fff; background: var(--db-blue); font-size: 12px; }
.ap-toggle:hover { background: var(--db-blue-hover); }
.ap-track { height: 4px; border-radius: 999px; background: #475467; }
.ap-fill { border-radius: 999px; background: var(--db-blue); }
.ap-knob { width: 10px; height: 10px; border: 2px solid #fff; border-radius: 50%; background: var(--db-blue); }
.ap-time { color: #cbd5e1; font-size: 11px; }
</style>

<style scoped>
.audio-player {
  width: min(520px, 100%);
  padding: 14px 16px;
  border: 1px solid #37332b;
  border-radius: 0;
  background: #171510;
}

.ap-toggle {
  width: 38px;
  height: 38px;
  border-radius: 0;
  color: var(--op-feature-ink);
  background: #2b2721;
}

.ap-toggle:hover { background: var(--op-accent); }
.ap-track { height: 3px; border-radius: 0; background: #3d372e; }
.ap-fill { border-radius: 0; background: var(--op-accent); }
.ap-knob { width: 10px; height: 10px; border: 0; border-radius: 0; background: var(--op-accent); }
.ap-time { color: #a59d90; font-family: var(--op-font-mono); font-size: 9px; }
</style>
