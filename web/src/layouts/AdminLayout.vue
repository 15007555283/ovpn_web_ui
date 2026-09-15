<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { NAlert, NButton } from "naive-ui";
import { api } from "../api/client";
import { useAppContext } from "../composables/useAppContext";
import type { PageKey, PageView } from "../types";
import DashboardView from "../views/DashboardView.vue";
import UsersView from "../views/users/UsersView.vue";
import RoutesView from "../views/routes/RoutesView.vue";
import OnlineView from "../views/OnlineView.vue";
import SystemView from "../views/SystemView.vue";
import AuditView from "../views/AuditView.vue";
import SettingsView from "../views/SettingsView.vue";
const { settings, fake, logged, loading, error, notice, run } = useAppContext();
const page = ref<PageKey>("dashboard");
const pageVersion = ref(0);
const currentView = ref<PageView>();
const views = {
  dashboard: DashboardView,
  users: UsersView,
  routes: RoutesView,
  online: OnlineView,
  system: SystemView,
  audit: AuditView,
  settings: SettingsView,
};
const nav: { key: PageKey; label: string; icon: string }[] = [
  { key: "dashboard", label: "仪表盘", icon: "◫" },
  { key: "users", label: "VPN 用户", icon: "♙" },
  { key: "routes", label: "分流路由", icon: "⇄" },
  { key: "online", label: "在线用户", icon: "◉" },
  { key: "system", label: "系统状态", icon: "▤" },
  { key: "audit", label: "审计日志", icon: "≡" },
  { key: "settings", label: "系统设置", icon: "⚙" },
];
const title = computed(
  () => nav.find((item) => item.key === page.value)?.label,
);
async function refresh() {
  await currentView.value?.refresh();
}
async function navigate(key: PageKey) {
  await run(async () => {
    page.value = key;
    pageVersion.value++;
    await nextTick();
    await refresh();
  });
}
function logout() {
  return run(async () => {
    await api("/auth/logout", "POST");
    logged.value = false;
  });
}
let timer: ReturnType<typeof setInterval> | undefined;
let lastRefresh = Date.now();
onMounted(() => {
  timer = setInterval(() => {
    if (
      !loading.value &&
      !currentView.value?.isEditing &&
      Date.now() - lastRefresh >= settings.refresh * 1000 &&
      ["dashboard", "users", "online", "system"].includes(page.value)
    ) {
      lastRefresh = Date.now();
      run(refresh);
    }
  }, 1000);
});
onUnmounted(() => clearInterval(timer));
defineExpose({ refresh });
</script>
<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-mark">V</span>
        <div>{{ settings.name }}<small>OPENVPN CONTROL PANEL</small></div>
      </div>
      <div class="nav-label">工作空间</div>
      <nav>
        <button
          v-for="item in nav"
          :key="item.key"
          :class="{ active: page === item.key }"
          @click="navigate(item.key)"
        >
          <span>{{ item.icon }}</span
          >{{ item.label }}
        </button>
      </nav>
      <div class="sidebar-foot">
        <span class="dot"></span>{{ fake ? "演示环境" : "单节点 · 私有网络"
        }}<small>安全连接，从这里开始</small>
      </div>
    </aside>
    <div class="workspace">
      <header>
        <span class="breadcrumb">工作空间 <span>/</span> {{ title }}</span
        ><NButton quaternary :disabled="loading" @click="logout"
          >退出登录</NButton
        >
      </header>
      <main>
        <div class="page-heading">
          <div>
            <div class="eyebrow">NETWORK MANAGEMENT</div>
            <h1>{{ title }}</h1>
            <p class="muted">
              {{
                page === "dashboard"
                  ? "查看网络概况，掌握每一次连接。"
                  : page === "routes"
                    ? "只让指定目标经过 VPN，其余流量保持本地出口。"
                    : page === "users"
                      ? "为每位用户管理独立的证书与客户端配置。"
                      : "管理你的 OpenVPN 私有网络。"
              }}
            </p>
          </div>
          <NButton :loading="loading" @click="run(refresh)">刷新数据</NButton>
        </div>
        <NAlert v-if="fake" type="warning" :bordered="false" class="banner"
          >当前为演示模式。证书仅用于测试，服务状态为模拟结果；在线数据需提供测试状态文件。</NAlert
        >
        <NAlert
          v-if="error"
          type="error"
          closable
          class="banner"
          @close="error = ''"
          >{{ error }}</NAlert
        ><NAlert
          v-if="notice"
          type="success"
          closable
          class="banner"
          @close="notice = ''"
          >{{ notice }}</NAlert
        >
        <component
          :is="views[page]"
          :key="page + pageVersion"
          ref="currentView"
          @navigate="navigate"
        />
        <footer>VPN Admin <span>OpenVPN Community · JSON 持久化</span></footer>
      </main>
    </div>
  </div>
</template>
