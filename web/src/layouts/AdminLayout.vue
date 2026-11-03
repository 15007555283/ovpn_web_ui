<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { NAlert } from "naive-ui";
import AccountMenu from "../components/account/AccountMenu.vue";
import { useAppContext } from "../composables/useAppContext";
import type { PageKey, PageView } from "../types";
import DashboardView from "../views/DashboardView.vue";
import UsersView from "../views/users/UsersView.vue";
import RoutesView from "../views/routes/RoutesView.vue";
import OnlineView from "../views/OnlineView.vue";
import AuditView from "../views/AuditView.vue";
const { settings, fake, loading, error, notice, run } = useAppContext();
const page = ref<PageKey>("dashboard");
const pageVersion = ref(0);
const currentView = ref<PageView>();
const accountMenu = ref<InstanceType<typeof AccountMenu>>();
const views = {
  dashboard: DashboardView,
  users: UsersView,
  routes: RoutesView,
  online: OnlineView,
  audit: AuditView,
};
const nav: { key: PageKey; label: string; icon: string }[] = [
  { key: "dashboard", label: "仪表盘", icon: "◫" },
  { key: "users", label: "VPN 用户", icon: "♙" },
  { key: "routes", label: "分流路由", icon: "⇄" },
  { key: "online", label: "在线用户", icon: "◉" },
  { key: "audit", label: "审计日志", icon: "≡" },
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
let timer: ReturnType<typeof setInterval> | undefined;
let lastRefresh = Date.now();
onMounted(() => {
  timer = setInterval(() => {
    if (
      !loading.value &&
      !currentView.value?.isEditing &&
      !accountMenu.value?.isOpen &&
      Date.now() - lastRefresh >= settings.refresh * 1000 &&
      ["dashboard", "users", "online"].includes(page.value)
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
        >
        <AccountMenu ref="accountMenu" />
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
        </div>
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
