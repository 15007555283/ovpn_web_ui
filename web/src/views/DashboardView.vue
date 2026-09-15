<script setup lang="ts">
import { computed, ref } from "vue";
import { NAlert, NButton, NTag } from "naive-ui";
import VersionStatus from "../components/VersionStatus.vue";
import AuditTable from "../components/AuditTable.vue";
import { api } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
import { healthNames } from "../utils/health";
import type { Dashboard, PageKey } from "../types";
const emit = defineEmits<{ navigate: [page: PageKey] }>();
const navigate = (page: PageKey) => emit("navigate", page);
const { settings, fake, loading } = useAppContext();
const dashboard = ref<Dashboard>();
const online = computed(
  () => dashboard.value?.online ?? { available: false, clients: [] },
);
const health = computed(() => dashboard.value?.health ?? {});
async function refresh() {
  dashboard.value = await api<Dashboard>("/dashboard");
}
defineExpose({ refresh });
</script>
<template>
  <template v-if="dashboard">
    <div class="stats">
      <div class="stat">
        <span>服务状态</span
        ><strong class="small-stat">{{
          health.service === "active"
            ? "运行中"
            : health.service === "active (fake)"
              ? "运行中（模拟）"
              : health.service === "unknown"
                ? "暂不可用"
                : health.service || "暂不可用"
        }}</strong
        ><small
          >{{ settings.protocol.toUpperCase() }} · {{ settings.port }} /
          {{ health.vpn_network }}</small
        >
      </div>
      <div class="stat">
        <span>当前在线</span
        ><strong
          >{{ online.available ? online.clients.length : "—"
          }}<em>位用户</em></strong
        ><small>{{
          online.available
            ? "每 " + settings.refresh + " 秒自动刷新"
            : "状态暂不可用"
        }}</small>
      </div>
      <div class="stat">
        <span>{{ fake ? "演示 VPN 用户" : "PKI 客户端证书" }}</span
        ><strong>{{ dashboard.total_users ?? "—" }}<em>个</em></strong
        ><small
          >有效 {{ dashboard.active_certificates ?? "—" }} · 已撤销
          {{ dashboard.revoked_certificates ?? "—" }}</small
        >
      </div>
      <div class="stat">
        <span>{{ fake ? "分流规则" : "服务器分流规则" }}</span
        ><strong>{{ dashboard.routes ?? "—" }}<em>条</em></strong
        ><small>{{ fake ? "演示路由" : "当前路由文件中的规则" }}</small>
      </div>
    </div>
    <VersionStatus :version="dashboard.version" />
    <NAlert
      v-if="health.error || health.certificate_error || health.routes_error"
      type="warning"
      >{{
        health.error || health.certificate_error || health.routes_error
      }}</NAlert
    >
    <div class="dashboard-grid">
      <section class="panel">
        <div class="panel-title">
          <h3>网络健康</h3>
          <NTag size="small" :bordered="false">实时概况</NTag>
        </div>
        <div class="health-row">
          <span>服务启动时间</span><span>{{ health.started_at || "—" }}</span>
        </div>
        <div
          v-for="k in ['server_config', 'routes_config', 'crl', 'ipv4_forward']"
          :key="k"
          class="health-row"
        >
          <span>{{ healthNames[k] }}</span
          ><NTag
            :type="health[k] ? 'success' : 'warning'"
            :bordered="false"
            size="small"
            >{{ health[k] ? "正常" : "待检查" }}</NTag
          >
        </div>
        <NAlert v-if="!online.available" type="warning" :bordered="false">{{
          online.message
        }}</NAlert>
      </section>
      <section class="panel quick-start">
        <div class="panel-title">
          <h3>管理网络</h3>
          <span>↗</span>
        </div>
        <h2>一份配置，即可连接。</h2>
        <p>为设备创建独立证书，按需配置分流目标。</p>
        <NButton type="primary" @click="navigate('users')"
          >管理 VPN 用户</NButton
        ><NButton @click="navigate('routes')">管理分流路由</NButton>
      </section>
    </div>
    <section class="panel">
      <div class="panel-title">
        <h3>最近操作</h3>
        <NButton text @click="navigate('audit')">查看全部 →</NButton>
      </div>
      <AuditTable :data="dashboard.recent" :loading="loading" />
    </section>
  </template>
</template>
