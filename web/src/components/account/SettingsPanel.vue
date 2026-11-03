<script setup lang="ts">
import { reactive } from "vue";
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
} from "naive-ui";
import { api } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
import type { Settings } from "../../types";
const { settings: appSettings, loading, notice, run } = useAppContext();
const settings = reactive({ ...appSettings });
async function refresh() {
  const data = await api<Settings>("/settings");
  Object.assign(settings, data);
  Object.assign(appSettings, data);
}
async function saveSettings() {
  await run(async () => {
    await api("/settings", "PUT", settings);
    Object.assign(appSettings, settings);
    notice.value = "设置已保存，新会话使用新的有效期";
  });
}
defineExpose({ refresh });
</script>
<template>
  <section>
    <NAlert type="info" :bordered="false" class="banner"
      >连接地址、端口和协议用于生成客户端配置，不修改服务器监听配置。修改后，用户设备上的旧配置需要重新下载并导入。</NAlert
    ><NForm label-placement="top"
      ><div class="form-grid">
        <NFormItem label="VPN 连接地址"
          ><NInput
            v-model:value="settings.endpoint"
            placeholder="vpn.example.com" /></NFormItem
        ><NFormItem label="端口"
          ><NInputNumber
            v-model:value="settings.port"
            :min="1"
            :max="65535" /></NFormItem
        ><NFormItem label="协议"
          ><NSelect
            v-model:value="settings.protocol"
            :options="[
              { label: 'UDP', value: 'udp' },
              { label: 'TCP', value: 'tcp-client' },
            ]" /></NFormItem
        ><NFormItem label="客户端 DNS（可选）"
          ><NInput
            v-model:value="settings.dns"
            placeholder="IPv4 地址" /></NFormItem
        ><NFormItem label="状态刷新周期（秒）"
          ><NInputNumber
            v-model:value="settings.refresh"
            :min="5"
            :max="300" /></NFormItem
        ><NFormItem label="会话有效期（小时）"
          ><NInputNumber
            v-model:value="settings.session_hours"
            :min="1"
            :max="168" /></NFormItem
        ><NFormItem label="展示名称"
          ><NInput v-model:value="settings.name"
        /></NFormItem>
      </div>
      <NButton type="primary" :loading="loading" @click="saveSettings"
        >保存设置</NButton
      ></NForm
    >
  </section>
</template>
