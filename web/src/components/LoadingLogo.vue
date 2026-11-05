<script setup lang="ts">
// 加载动画：底层是压暗的 logo 轮廓，上层是一条被 logo 墨迹裁切的光带。
// 两层分开，底层单独压暗，光带保持原有亮度，扫过时高光才看得见。
</script>

<template>
  <div class="loading-logo" role="status" aria-label="加载中" />
</template>

<style scoped>
.loading-logo {
  position: relative;
  width: 104px;
  height: 104px;
}
/* 底层：淡化的 logo 轮廓 */
.loading-logo::before {
  content: "";
  position: absolute;
  inset: 0;
  background: url("/logo.svg") center / contain no-repeat;
  opacity: 0.2;
}
/* 上层：光带裁成 logo 形状，沿线条流过 */
.loading-logo::after {
  content: "";
  position: absolute;
  inset: 0;
  background-image: linear-gradient(
    100deg,
    rgba(255, 110, 96, 0) 0%,
    rgba(255, 110, 96, 0) 43%,
    rgba(255, 118, 104, 1) 50%,
    rgba(255, 110, 96, 0) 57%,
    rgba(255, 110, 96, 0) 100%
  );
  background-size: 260% 100%;
  background-repeat: no-repeat;
  background-position: 100% 0;
  -webkit-mask: url("/logo.svg") center / contain no-repeat;
  mask: url("/logo.svg") center / contain no-repeat;
  animation: sweep 1.8s linear infinite;
}
@keyframes sweep {
  from {
    background-position: 96% 0;
  }
  to {
    background-position: 4% 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .loading-logo::after {
    animation-duration: 4.2s;
  }
}
</style>
