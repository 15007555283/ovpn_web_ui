<script setup lang="ts">
import { computed, ref } from "vue";
import { NAlert, NDataTable, NInput } from "naive-ui";
import { api } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
import { fmt, bytes } from "../utils/format";
import type { OnlineStatus, OnlineClient } from "../types";
const { settings, loading } = useAppContext();
const online = ref<OnlineStatus>({ available: false, clients: [] });
const search = ref("");
const filteredOnline = computed(() =>
  online.value.clients.filter((r) =>
    (r.username + " " + r.remote).includes(search.value),
  ),
);
async function refresh() {
  online.value = await api<OnlineStatus>("/sessions/online");
}
defineExpose({ refresh });
const onlineColumns = [
  { title: "用户名", key: "username" },
  { title: "VPN IP", key: "vpn_ip" },
  { title: "来源 IP / 端口", key: "remote" },
  {
    title: "连接时间",
    key: "connected_at",
    render: (r: OnlineClient) => fmt(r.connected_at),
  },
  {
    title: "在线时长",
    key: "duration",
    render: (r: OnlineClient) =>
      Math.max(
        0,
        Math.floor((Date.now() - new Date(r.connected_at).getTime()) / 60000),
      ) + " 分钟",
  },
  {
    title: "接收",
    key: "bytes_received",
    render: (r: OnlineClient) => bytes(r.bytes_received),
  },
  {
    title: "发送",
    key: "bytes_sent",
    render: (r: OnlineClient) => bytes(r.bytes_sent),
  },
];
</script>
<template>
  <section class="panel">
    <div class="toolbar">
      <NInput
        v-model:value="search"
        clearable
        placeholder="搜索用户名或来源 IP"
      /><span class="muted"
        >每 {{ settings.refresh }} 秒刷新 · 更新于
        {{ fmt(online.updated_at) }}</span
      >
    </div>
    <NAlert v-if="!online.available" type="warning">{{
      online.message || "状态暂不可用"
    }}</NAlert
    ><NDataTable
      v-else
      :columns="onlineColumns"
      :data="filteredOnline"
      :loading="loading"
      :scroll-x="1000"
      :pagination="{ pageSize: 15 }"
    />
  </section>
</template>
