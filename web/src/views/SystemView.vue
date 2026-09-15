<script setup lang="ts">
import { ref, computed } from "vue";
import {
  NAlert,
  NButton,
  NDescriptions,
  NDescriptionsItem,
  NModal,
} from "naive-ui";
import { api, downloadFile } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
import { healthNames } from "../utils/health";
import type { Health } from "../types";
const { loading, error, notice, run } = useAppContext();
const health = ref<Health>({}),
  restarting = ref(false);
const isEditing = computed(() => restarting.value);
async function refresh() {
  health.value = await api<Health>("/system/health");
}
const download = (path: string, name: string) =>
  run(() => downloadFile(path, name));
async function restart() {
  await run(async () => {
    await api("/system/openvpn/restart", "POST", { confirm: true });
    notice.value = "服务已重启";
    restarting.value = false;
    await refresh();
  });
}
defineExpose({ refresh, isEditing });
</script>
<template>
  <section class="panel">
    <div class="panel-title">
      <h3>服务与运行环境</h3>
      <div class="actions">
        <NButton
          @click="download('/system/diagnostics', 'vpn-diagnostics.json')"
          >下载脱敏诊断</NButton
        ><NButton type="error" secondary @click="restarting = true"
          >重启 OpenVPN</NButton
        >
      </div>
    </div>
    <NDescriptions bordered :column="1" label-placement="left"
      ><NDescriptionsItem
        v-for="(value, key) in health"
        :key="key"
        :label="healthNames[String(key)] || String(key)"
        >{{
          typeof value === "boolean" ? (value ? "是" : "否") : value
        }}</NDescriptionsItem
      ></NDescriptions
    >
    <p class="muted">健康检查不替代真实客户端的 VPN 连通性、NAT 和分流验证。</p>
  </section>
  <NModal
    v-model:show="restarting"
    preset="card"
    title="重启 OpenVPN 服务"
    :style="{ width: 'min(580px, calc(100vw - 32px))' }"
    :mask-closable="!loading"
    :closable="!loading"
  >
    <NAlert v-if="error" type="error" class="banner">{{ error }}</NAlert>
    <NAlert type="warning" :bordered="false"
      >重启 OpenVPN 会中断当前 VPN 连接，请确认操作。</NAlert
    >
    <template #footer
      ><div class="modal-footer">
        <NButton :disabled="loading" @click="restarting = false">取消</NButton
        ><NButton type="error" :loading="loading" @click="restart"
          >确认</NButton
        >
      </div></template
    >
  </NModal>
</template>
