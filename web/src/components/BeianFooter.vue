<template>
  <footer v-if="icp || gongan" class="beian-footer" role="contentinfo" aria-label="网站备案信息">
    <a v-if="icp" href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
      {{ icp }}
    </a>
    <a
      v-if="gongan"
      class="beian-psb"
      :href="gonganUrl || 'https://www.beian.gov.cn/'"
      target="_blank"
      rel="noopener noreferrer"
    >
      <img src="https://www.beian.gov.cn/img/ghs.png" alt="" width="16" height="16" />
      <span>{{ gongan }}</span>
    </a>
  </footer>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import request from '@/api/request'

const icp = ref('')
const gongan = ref('')
const gonganUrl = ref('')

onMounted(async () => {
  try {
    const res = await request.get('/api/site')
    icp.value = res.data?.beian_icp || ''
    gongan.value = res.data?.beian_gongan || ''
    gonganUrl.value = res.data?.beian_gongan_url || ''
  } catch {
    // ignore
  }
})
</script>

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
