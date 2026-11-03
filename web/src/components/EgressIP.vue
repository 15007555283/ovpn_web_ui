<script setup lang="ts">
import { onMounted, shallowRef } from "vue";
import { NButton, NSpace } from "naive-ui";
import { api, ApiError } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
const { logged } = useAppContext();
const ip = shallowRef("");
const message = shallowRef("");
const loading = shallowRef(false);
async function refresh() {
  if (loading.value) return;
  loading.value = true;
  try {
    const result = await api<{ ip: string; message: string }>("/system/egress-ip");
    ip.value = result.ip;
    message.value = result.message;
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) logged.value = false;
    ip.value = "";
    message.value = "暂时无法获取出口 IP";
  } finally {
    loading.value = false;
  }
}
onMounted(refresh);
</script>
<template>
  <div class="health-row">
    <span>VPN 服务器出口 IP（IPv4）</span>
    <NSpace align="center" :size="8">
      <span :title="message || '服务器默认公网出口，缓存 5 分钟'">{{ loading ? "查询中…" : ip || message || "—" }}</span>
      <NButton text size="tiny" :loading="loading" @click="refresh">刷新</NButton>
    </NSpace>
  </div>
</template>
