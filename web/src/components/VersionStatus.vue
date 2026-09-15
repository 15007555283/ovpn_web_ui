<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NAlert, NButton, NModal, NSpace, NTag } from "naive-ui";
import { api, ApiError } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
import type { VersionStatus } from "../types";
const props = defineProps<{ version: string }>();
const { logged } = useAppContext();
const result = ref<VersionStatus>();
const checking = ref(false), show = ref(false), error = ref("");
const message = computed(() => {
  if (!result.value) return "";
  if (props.version === "dev") return `当前为开发构建，可安装正式版 ${result.value.latest}`;
  if (result.value.update_available) return `发现新版本 ${result.value.latest}`;
  return result.value.current === result.value.latest ? "已是最新版本" : `当前版本高于最新正式版 ${result.value.latest}`;
});
async function check() {
  if (checking.value) return;
  checking.value = true;
  error.value = "";
  try {
    result.value = await api<VersionStatus>("/system/version");
  } catch (e) {
    result.value = undefined;
    if (e instanceof ApiError && e.status === 401) logged.value = false;
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    checking.value = false;
  }
}
onMounted(check);
</script>
<template>
  <section class="panel version-panel">
    <div class="panel-title">
      <h3>ovpn-cli 版本</h3>
      <NSpace :size="8">
        <NButton size="small" :loading="checking" @click="check">检查更新</NButton>
        <NButton size="small" @click="show = true">更新方法</NButton>
      </NSpace>
    </div>
    <NSpace align="center" :size="12">
      <NTag :bordered="false" type="info">{{ version }}</NTag>
      <span v-if="error" class="version-error">检查失败，请重试</span>
      <span v-else class="version-message">{{ checking ? "正在检查新版本…" : message }}</span>
    </NSpace>
  </section>
  <NModal v-model:show="show" preset="card" title="更新 ovpn-cli" :style="{ width: 'min(580px, calc(100vw - 32px))' }">
    <p>当前运行版本：{{ version }}</p>
    <NAlert v-if="error" type="warning">{{ error }}</NAlert>
    <p v-else-if="message">{{ message }}</p>
    <p>登录服务器，备份数据和 PKI 后执行：</p>
    <pre class="update-commands">ovpn-cli update --check
sudo ovpn-cli update
ovpn-cli version</pre>
    <p>更新会短暂重启管理后台，保留配置、数据和 PKI，OpenVPN 服务不会重启。完成后刷新页面查看新版本。</p>
    <a v-if="result" :href="result.release_url" target="_blank" rel="noopener noreferrer">查看版本发布说明 ↗</a>
  </NModal>
</template>
<style scoped>
.version-panel .panel-title { flex-wrap: wrap; gap: 12px; }
.version-message { font-size: 13px; color: #657388; }
.version-error { color: #b45309; }
.update-commands { padding: 12px; background: #f5f7fa; border-radius: 8px; overflow-x: auto; }
</style>
