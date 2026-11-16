<template>
  <div class="loading-logo" role="status">
    <span class="mark" aria-hidden="true">iP<span class="dot" /></span>
    <span>正在打开档案库</span>
    <span class="track" aria-hidden="true" />
  </div>
</template>

<style scoped>
.loading-logo {
  display: grid;
  justify-items: center;
  gap: 15px;
  color: var(--op-ink-soft);
  font-family: var(--op-font-mono);
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.mark {
  position: relative;
  display: grid;
  place-items: center;
  width: 62px;
  height: 62px;
  color: var(--op-feature-ink);
  background: var(--op-feature-bg);
  font-family: var(--op-font-display);
  font-size: 27px;
  letter-spacing: -0.08em;
}
.dot {
  position: absolute;
  right: 7px;
  bottom: 7px;
  width: 7px;
  height: 7px;
  background: var(--op-accent);
}
.track {
  position: relative;
  width: 92px;
  height: 2px;
  overflow: hidden;
  background: var(--op-rule);
}
.track::after {
  content: "";
  position: absolute;
  inset: 0 55% 0 0;
  background: var(--op-accent);
  animation: progress 1.4s ease-in-out infinite alternate;
}
@keyframes progress {
  from { transform: translateX(0); }
  to { transform: translateX(122%); }
}
</style>

<style scoped>
.loading-logo { display: grid; justify-items: center; gap: 12px; color: var(--op-ink-soft); font-family: var(--op-font-sans); font-size: 12px; font-weight: 500; letter-spacing: 0; text-transform: none; }
.mark {
  position: relative;
  display: grid;
  place-items: center;
  width: 52px;
  height: 52px;
  border-radius: 12px;
  color: #fff;
  background: var(--db-blue);
  font-family: var(--op-font-sans);
  font-size: 21px;
  font-weight: 700;
  letter-spacing: -0.04em;
}
.dot { position: absolute; right: 6px; bottom: 6px; width: 6px; height: 6px; border-radius: 50%; background: #fff; opacity: 0.65; }
.track { position: relative; width: 84px; height: 3px; overflow: hidden; border-radius: 999px; background: var(--op-rule); }
.track::after { content: ""; position: absolute; inset: 0 55% 0 0; border-radius: inherit; background: var(--db-blue); animation: progress 1.2s ease-in-out infinite alternate; }
</style>
