<template>
  <footer class="beian-footer" role="contentinfo" aria-label="网站备案信息">
    <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
      苏ICP备2026026995号-1
    </a>
    <a
      class="beian-psb"
      href="https://www.beian.gov.cn/portal/registerSystemInfo?recordcode=32032202000643"
      target="_blank"
      rel="noopener noreferrer"
    >
      <img src="https://www.beian.gov.cn/img/ghs.png" alt="" width="16" height="16" />
      <span>苏公网安备32032202000643号</span>
    </a>
  </footer>
</template>

<style scoped>
.beian-footer {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 20px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 12px 20px;
  padding: 0 16px;
  font-size: 12px;
  line-height: 1.5;
}

.beian-footer a {
  color: rgba(255, 255, 255, 0.85);
  text-decoration: none;
}

.beian-footer a:hover {
  color: #fff;
  text-decoration: underline;
}

.beian-psb {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.beian-psb img {
  display: block;
}
</style>
