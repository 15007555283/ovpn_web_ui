<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, reactive, ref } from "vue";
import {
  NConfigProvider,
  NButton,
  NInput,
  NInputNumber,
  NSelect,
  NDataTable,
  NTag,
  NModal,
  NForm,
  NFormItem,
  NAlert,
  NSwitch,
  NPagination,
  NDescriptions,
  NDescriptionsItem,
  zhCN,
  dateZhCN,
} from "naive-ui";

type Row = Record<string, any>;
const loading = ref(false),
  error = ref(""),
  notice = ref(""),
  logged = ref(false),
  setup = ref(false),
  fake = ref(false),
  page = ref("dashboard");
const auth = reactive({ username: "", password: "" }),
  users = ref<Row[]>([]),
  routes = ref<Row[]>([]),
  logs = ref<Row[]>([]),
  online = ref<Row>({ available: false, clients: [] }),
  health = ref<Row>({}),
  dashboard = ref<Row>({}),
  pending = ref<Row>({ revision: 0, pending: false, history: [] }),
  total = ref(0),
  auditPage = ref(1),
  search = ref(""),
  statusFilter = ref<string | null>(null);
const settings = reactive({
  endpoint: "vpn.example.com",
  port: 1194,
  protocol: "udp",
  dns: "",
  refresh: 10,
  session_hours: 8,
  name: "VPN Admin",
});
const password = reactive({ old_password: "", new_password: "" });
const modal = ref(""),
  selected = ref<Row>({}),
  confirmation = ref(""),
  form = reactive({ username: "", remark: "", endpoint: "", lines: "" }),
  batch = ref<Row[]>([]);
const nav = [
  { key: "dashboard", label: "仪表盘", icon: "◫" },
  { key: "users", label: "VPN 用户", icon: "♙" },
  { key: "routes", label: "分流路由", icon: "⇄" },
  { key: "online", label: "在线用户", icon: "◉" },
  { key: "system", label: "系统状态", icon: "▤" },
  { key: "audit", label: "审计日志", icon: "≡" },
  { key: "settings", label: "系统设置", icon: "⚙" },
];
const title = computed(() => nav.find((n) => n.key === page.value)?.label);
async function api(path: string, method = "GET", data?: unknown) {
  const response = await fetch("/api/v1" + path, {
    method,
    headers: { "Content-Type": "application/json", "X-VPN-Admin": "1" },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
  const result = await response.json();
  if (!response.ok) {
    if (response.status === 401) logged.value = false;
    throw new Error(`${result.message}（请求 ID：${result.request_id}）`);
  }
  return result.data;
}
async function run(action: () => Promise<void>) {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  notice.value = "";
  try {
    await action();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}
async function load() {
  switch (page.value) {
    case "dashboard":
      dashboard.value = await api("/dashboard");
      online.value = dashboard.value.online;
      health.value = dashboard.value.health;
      break;
    case "users":
      users.value = await api("/vpn-users");
      online.value = await api("/sessions/online");
      break;
    case "routes":
      routes.value = await api("/routes");
      pending.value = await api("/routes/pending");
      break;
    case "online":
      online.value = await api("/sessions/online");
      break;
    case "system":
      health.value = await api("/system/health");
      break;
    case "audit": {
      const d = await api(
        `/audit-logs?page=${auditPage.value}&q=${encodeURIComponent(search.value)}`,
      );
      logs.value = d.items;
      total.value = d.total;
      break;
    }
    case "settings":
      Object.assign(settings, await api("/settings"));
      break;
  }
}
function navigate(key: string) {
  if (loading.value) return;
  page.value = key;
  search.value = "";
  statusFilter.value = null;
  run(load);
}
async function enter() {
  Object.assign(settings, await api("/settings"));
  logged.value = true;
  await load();
}
async function submitAuth() {
  await run(async () => {
    if (setup.value) {
      await api("/setup/admin", "POST", auth);
      setup.value = false;
      notice.value = "管理员已初始化，请登录";
      auth.password = "";
    } else {
      await api("/auth/login", "POST", auth);
      auth.password = "";
      await enter();
    }
  });
}
function open(kind: string, row: Row = {}) {
  error.value = "";
  selected.value = row;
  confirmation.value = "";
  Object.assign(form, {
    username: "",
    remark: row.remark || "",
    endpoint: settings.endpoint,
    lines: "",
  });
  batch.value = [];
  modal.value = kind;
}
const fmt = (value: any) =>
  !value || String(value).startsWith("0001")
    ? "—"
    : new Date(value).toLocaleString("zh-CN");
function bytes(n: number) {
  if (!n) return "0 B";
  const i = Math.min(3, Math.floor(Math.log(n) / Math.log(1024)));
  return (n / 1024 ** i).toFixed(1) + " " + ["B", "KB", "MB", "GB"][i];
}
function tag(
  text: string,
  type: "success" | "warning" | "error" | "default" = "default",
) {
  return h(NTag, { type, bordered: false, size: "small" }, () => text);
}
function button(
  text: string,
  action: () => void,
  danger = false,
  disabled = false,
) {
  return h(
    NButton,
    {
      size: "small",
      type: danger ? "error" : "default",
      quaternary: true,
      disabled: disabled || loading.value,
      onClick: action,
    },
    () => text,
  );
}
const labels: Row = {
  active: "有效",
  revoked: "已撤销",
  create_failed: "创建失败",
  revoke_pending: "撤销待完成",
};
function client(u: Row) {
  return online.value.clients?.find((c: Row) => c.username === u.username);
}
const filteredUsers = computed(() =>
  users.value.filter(
    (u) =>
      u.username.includes(search.value) &&
      (!statusFilter.value || u.status === statusFilter.value),
  ),
);
const filteredRoutes = computed(() =>
  routes.value.filter((r) => (r.cidr + " " + r.remark).includes(search.value)),
);
const filteredOnline = computed(
  () =>
    online.value.clients?.filter((r: Row) =>
      (r.username + " " + r.remote).includes(search.value),
    ) || [],
);
const userColumns = [
  {
    title: "用户名",
    key: "username",
    minWidth: 120,
    render: (r: Row) => button(r.username, () => open("detail", r)),
  },
  {
    title: "证书状态",
    key: "status",
    minWidth: 115,
    render: (r: Row) =>
      tag(
        labels[r.status] || r.status,
        r.status === "active"
          ? "success"
          : r.status === "revoked"
            ? "default"
            : "warning",
      ),
  },
  {
    title: "在线 / VPN IP",
    key: "online",
    minWidth: 160,
    render: (r: Row) =>
      online.value.available
        ? client(r)
          ? tag("在线 · " + client(r).vpn_ip, "success")
          : "离线 · —"
        : tag("状态暂不可用", "warning"),
  },
  { title: "备注", key: "remark", minWidth: 150 },
  {
    title: "证书到期",
    key: "certificate_expires_at",
    minWidth: 170,
    render: (r: Row) => fmt(r.certificate_expires_at),
  },
  {
    title: "创建时间",
    key: "created_at",
    minWidth: 170,
    render: (r: Row) => fmt(r.created_at),
  },
  {
    title: "最后连接",
    key: "last_connected_at",
    minWidth: 170,
    render: (r: Row) => fmt(r.last_connected_at),
  },
  {
    title: "操作",
    key: "actions",
    minWidth: 250,
    render: (r: Row) =>
      h("div", { class: "actions" }, [
        button(
          "下载",
          () =>
            download("/vpn-users/" + r.id + "/config", r.username + ".ovpn"),
          false,
          r.status !== "active",
        ),
        button("编辑", () => open("edit-user", r)),
        button(
          "重新生成",
          () => open("regenerate", r),
          false,
          !["active", "create_failed"].includes(r.status),
        ),
        button(
          r.status === "revoke_pending" ? "重试撤销" : "撤销",
          () => open("revoke", r),
          true,
          r.status === "revoked",
        ),
      ]),
  },
];
const routeColumns = [
  { title: "目标 CIDR", key: "cidr", minWidth: 150 },
  { title: "备注", key: "remark", minWidth: 200 },
  {
    title: "状态",
    key: "enabled",
    minWidth: 110,
    render: (r: Row) =>
      h(
        NSwitch,
        {
          value: r.enabled,
          disabled: loading.value,
          "onUpdate:value": (value: boolean) =>
            run(async () => {
              await api("/routes/" + r.id, "PUT", {
                remark: r.remark,
                enabled: value,
              });
              await load();
            }),
        },
        { checked: () => "启用", unchecked: () => "禁用" },
      ),
  },
  {
    title: "更新时间",
    key: "updated_at",
    minWidth: 175,
    render: (r: Row) => fmt(r.updated_at),
  },
  {
    title: "操作",
    key: "actions",
    minWidth: 130,
    render: (r: Row) =>
      h("div", { class: "actions" }, [
        button("编辑备注", () => open("edit-route", r)),
        button("删除", () => open("delete-route", r), true),
      ]),
  },
];
const onlineColumns = [
  { title: "用户名", key: "username" },
  { title: "VPN IP", key: "vpn_ip" },
  { title: "来源 IP / 端口", key: "remote" },
  {
    title: "连接时间",
    key: "connected_at",
    render: (r: Row) => fmt(r.connected_at),
  },
  {
    title: "在线时长",
    key: "duration",
    render: (r: Row) =>
      Math.max(
        0,
        Math.floor((Date.now() - new Date(r.connected_at).getTime()) / 60000),
      ) + " 分钟",
  },
  {
    title: "接收",
    key: "bytes_received",
    render: (r: Row) => bytes(r.bytes_received),
  },
  { title: "发送", key: "bytes_sent", render: (r: Row) => bytes(r.bytes_sent) },
];
const auditColumns = [
  {
    title: "时间",
    key: "created_at",
    minWidth: 170,
    render: (r: Row) => fmt(r.created_at),
  },
  { title: "管理员", key: "admin", minWidth: 100 },
  { title: "操作", key: "action", minWidth: 130 },
  { title: "资源", key: "resource_id", minWidth: 130 },
  { title: "来源 IP", key: "source_ip", minWidth: 100 },
  {
    title: "结果",
    key: "success",
    minWidth: 85,
    render: (r: Row) =>
      tag(r.success ? "成功" : "失败", r.success ? "success" : "error"),
  },
  {
    title: "摘要 / 请求 ID",
    key: "detail",
    minWidth: 230,
    render: (r: Row) =>
      h("div", [h("div", r.detail), h("small", r.request_id)]),
  },
];
const healthNames: Row = {
  service: "服务状态",
  service_name: "服务名称",
  version: "OpenVPN 版本",
  started_at: "服务启动时间",
  last_result: "最近服务结果",
  vpn_network: "VPN 网段",
  server_config: "主配置权限检查",
  routes_config: "路由文件检查",
  crl: "CRL 文件检查",
  preflight: "配置结构预检查",
  status_updated_at: "状态文件更新时间",
  ipv4_forward: "IPv4 Forward",
  nat: "NAT 规则存在性",
  forward: "FORWARD 规则存在性",
  firewall_note: "防火墙检查说明",
  fake: "开发模拟模式",
};
async function download(path: string, filename: string) {
  await run(async () => {
    const response = await fetch("/api/v1" + path);
    if (!response.ok) {
      const e = await response.json();
      throw new Error(`${e.message}（请求 ID：${e.request_id}）`);
    }
    const blob = await response.blob(),
      url = URL.createObjectURL(blob),
      a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  });
}
async function confirmModal() {
  await run(async () => {
    const r = selected.value;
    switch (modal.value) {
      case "create":
        await api("/vpn-users", "POST", form);
        notice.value = "用户创建成功，可以下载客户端配置";
        break;
      case "edit-user":
        await api("/vpn-users/" + r.id, "PUT", { remark: form.remark });
        break;
      case "revoke":
        await api("/vpn-users/" + r.id + "/revoke", "POST", {
          confirm: confirmation.value,
        });
        notice.value = "证书已撤销；通常在下次 TLS 校验或重连时生效";
        break;
      case "regenerate":
        await api("/vpn-users/" + r.id + "/regenerate-config", "POST");
        notice.value = "已按当前设置重新生成，请重新下载配置";
        break;
      case "batch":
        batch.value = await api("/routes/batch", "POST", {
          lines: form.lines,
          remark: form.remark,
        });
        await load();
        return;
      case "edit-route":
        await api("/routes/" + r.id, "PUT", {
          remark: form.remark,
          enabled: r.enabled,
        });
        break;
      case "delete-route":
        await api("/routes/" + r.id, "DELETE");
        break;
      case "apply": {
        const data = await api("/routes/apply", "POST", {
          revision: pending.value.revision,
          confirm: true,
        });
        notice.value = data.message;
        break;
      }
      case "restart":
        await api("/system/openvpn/restart", "POST", { confirm: true });
        notice.value = "服务已重启";
        break;
    }
    modal.value = "";
    await load();
  });
}
const modalTitles: Row = {
  create: "创建 VPN 用户",
  "edit-user": "编辑用户备注",
  revoke: "撤销用户证书",
  regenerate: "重新生成客户端配置",
  batch: "新增分流路由",
  "edit-route": "编辑路由备注",
  "delete-route": "删除分流路由",
  apply: "应用路由配置",
  restart: "重启 OpenVPN 服务",
  detail: "用户详情",
};
let timer: ReturnType<typeof setInterval> | undefined;
let lastRefresh = Date.now();
onMounted(async () => {
  await run(async () => {
    const state = await api("/setup/status");
    setup.value = state.needs_setup;
    fake.value = state.fake;
    if (!setup.value) {
      try {
        await api("/auth/me");
      } catch {
        return;
      }
      await enter();
    }
  });
  timer = setInterval(() => {
    if (
      logged.value &&
      !loading.value &&
      !modal.value &&
      Date.now() - lastRefresh >= settings.refresh * 1000 &&
      ["dashboard", "online", "system", "users"].includes(page.value)
    ) {
      lastRefresh = Date.now();
      run(load);
    }
  }, 1000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <NConfigProvider
    :locale="zhCN"
    :date-locale="dateZhCN"
    :theme-overrides="{
      common: {
        primaryColor: '#0d776f',
        primaryColorHover: '#12958a',
        primaryColorPressed: '#09665f',
        borderRadius: '8px',
      },
    }"
  >
    <div v-if="!logged" class="login-screen">
      <div class="login-intro">
        <span class="brand-mark">V</span>
        <div class="eyebrow">PRIVATE NETWORK · SIMPLE CONTROL</div>
        <h1>你的网络，<br />尽在掌握。</h1>
        <p>统一管理 VPN 用户、分流路由和连接状态。</p>
        <div class="intro-foot">OpenVPN Community · 单节点管理控制台</div>
      </div>
      <section class="login-card">
        <span class="eyebrow">VPN ADMIN</span>
        <h2>{{ setup ? "初始化管理员" : "欢迎回来" }}</h2>
        <p class="muted">
          {{
            setup
              ? "创建唯一管理员账号，初始化后此入口将关闭。"
              : "登录以管理你的专属 VPN 网络。"
          }}
        </p>
        <NAlert v-if="fake" type="warning" :bordered="false"
          >开发模拟模式，不会操作真实 VPN</NAlert
        ><NAlert v-if="error" type="error">{{ error }}</NAlert
        ><NAlert v-if="notice" type="success">{{ notice }}</NAlert
        ><NForm @submit.prevent="submitAuth"
          ><NFormItem label="管理员账号"
            ><NInput
              v-model:value="auth.username"
              placeholder="输入管理员账号"
              autocomplete="username" /></NFormItem
          ><NFormItem label="密码"
            ><NInput
              v-model:value="auth.password"
              type="password"
              show-password-on="click"
              :placeholder="setup ? '至少 12 个字符' : '输入密码'"
              :autocomplete="
                setup ? 'new-password' : 'current-password'
              " /></NFormItem
          ><NButton
            attr-type="submit"
            type="primary"
            block
            size="large"
            :loading="loading"
            >{{ setup ? "创建管理员" : "登录控制台" }}</NButton
          ></NForm
        >
      </section>
    </div>
    <div v-else class="shell">
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
          <span class="dot"></span
          >{{ fake ? "开发模拟环境" : "单节点 · 私有网络"
          }}<small>安全连接，从这里开始</small>
        </div>
      </aside>
      <div class="workspace">
        <header>
          <span class="breadcrumb">工作空间 <span>/</span> {{ title }}</span
          ><NButton
            quaternary
            :disabled="loading"
            @click="
              run(async () => {
                await api('/auth/logout', 'POST');
                logged = false;
              })
            "
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
            <NButton :loading="loading" @click="run(load)">刷新数据</NButton>
          </div>
          <NAlert v-if="fake" type="warning" :bordered="false" class="banner"
            >当前为 fake
            开发模式。证书仅用于测试，服务状态为模拟结果；在线数据需提供测试状态文件。</NAlert
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
          <template v-if="page === 'dashboard'">
            <div class="stats">
              <div class="stat">
                <span>服务状态</span
                ><strong class="small-stat">{{
                  health.service === "active"
                    ? "运行中"
                    : health.service === "active (fake)"
                      ? "运行中（模拟）"
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
                <span>VPN 用户</span
                ><strong>{{ dashboard.total_users || 0 }}<em>个</em></strong
                ><small
                  >有效 {{ dashboard.active_certificates || 0 }} · 已撤销
                  {{ dashboard.revoked_certificates || 0 }}</small
                >
              </div>
              <div class="stat">
                <span>分流规则</span
                ><strong>{{ dashboard.routes || 0 }}<em>条</em></strong
                ><small>全局 Split Tunnel</small>
              </div>
            </div>
            <div class="dashboard-grid">
              <section class="panel">
                <div class="panel-title">
                  <h3>网络健康</h3>
                  <NTag size="small" :bordered="false">实时概况</NTag>
                </div>
                <div class="health-row">
                  <span>服务启动时间</span
                  ><span>{{ health.started_at || "—" }}</span>
                </div>
                <div
                  v-for="k in [
                    'server_config',
                    'routes_config',
                    'crl',
                    'ipv4_forward',
                  ]"
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
                <NAlert
                  v-if="!online.available"
                  type="warning"
                  :bordered="false"
                  >{{ online.message }}</NAlert
                >
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
              <NDataTable
                :columns="auditColumns"
                :data="dashboard.recent || []"
                :loading="loading"
                :scroll-x="1050"
              />
            </section>
          </template>
          <section v-else-if="page === 'users'" class="panel">
            <div class="toolbar">
              <NInput
                v-model:value="search"
                clearable
                placeholder="搜索用户名"
              /><NSelect
                v-model:value="statusFilter"
                clearable
                placeholder="全部状态"
                :options="
                  Object.entries(labels).map(([value, label]) => ({
                    value,
                    label: String(label),
                  }))
                "
              /><NButton type="primary" @click="open('create')"
                >＋ 创建用户</NButton
              >
            </div>
            <NDataTable
              :columns="userColumns"
              :data="filteredUsers"
              :loading="loading"
              :scroll-x="1450"
              :pagination="{ pageSize: 15 }"
            />
          </section>
          <template v-else-if="page === 'routes'"
            ><NAlert
              :type="pending.pending ? 'warning' : 'success'"
              :bordered="false"
              class="banner"
              >{{
                pending.pending
                  ? "存在未应用变更，保存规则不会立即中断连接。"
                  : "当前规则已应用。"
              }}
              当前版本 {{ pending.revision }} / 已应用
              {{
                pending.applied_revision < 0
                  ? "尚未应用"
                  : pending.applied_revision
              }}<NButton
                style="margin-left: 16px"
                :disabled="!pending.pending || loading"
                type="error"
                secondary
                @click="open('apply')"
                >应用配置</NButton
              ></NAlert
            >
            <section class="panel">
              <div class="toolbar">
                <NInput
                  v-model:value="search"
                  clearable
                  placeholder="搜索目标或备注"
                /><NButton type="primary" @click="open('batch')"
                  >＋ 新增 / 批量新增</NButton
                >
              </div>
              <NDataTable
                :columns="routeColumns"
                :data="filteredRoutes"
                :loading="loading"
                :scroll-x="800"
                :pagination="{ pageSize: 15 }"
              />
            </section>
            <section v-if="pending.history?.length" class="panel">
              <h3>配置应用记录</h3>
              <NDataTable
                :columns="[
                  { title: '版本', key: 'revision' },
                  {
                    title: '状态',
                    key: 'status',
                    render: (r: Row) =>
                      (
                        ({
                          applying: '应用中 / 待恢复',
                          applied: '已应用',
                          failed: '失败',
                          rolled_back: '已回滚',
                        }) as Row
                      )[r.status] || r.status,
                  },
                  {
                    title: '时间',
                    key: 'created_at',
                    render: (r: Row) => fmt(r.created_at),
                  },
                ]"
                :data="pending.history.slice().reverse()"
                :pagination="{ pageSize: 5 }"
              /></section
          ></template>
          <section v-else-if="page === 'online'" class="panel">
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
          <section v-else-if="page === 'system'" class="panel">
            <div class="panel-title">
              <h3>服务与运行环境</h3>
              <div class="actions">
                <NButton
                  @click="
                    download('/system/diagnostics', 'vpn-diagnostics.json')
                  "
                  >下载脱敏诊断</NButton
                ><NButton type="error" secondary @click="open('restart')"
                  >重启 OpenVPN</NButton
                >
              </div>
            </div>
            <NDescriptions bordered :column="1" label-placement="left"
              ><NDescriptionsItem
                v-for="(value, key) in health"
                :key="key"
                :label="healthNames[key] || key"
                >{{
                  typeof value === "boolean" ? (value ? "是" : "否") : value
                }}</NDescriptionsItem
              ></NDescriptions
            >
            <p class="muted">
              健康检查不替代真实客户端的 VPN 连通性、NAT 和分流验证。
            </p>
          </section>
          <section v-else-if="page === 'audit'" class="panel">
            <div class="toolbar">
              <NInput
                v-model:value="search"
                placeholder="搜索动作、资源或请求 ID"
                @keyup.enter="
                  run(async () => {
                    auditPage = 1;
                    await load();
                  })
                "
              /><NButton
                @click="
                  run(async () => {
                    auditPage = 1;
                    await load();
                  })
                "
                >搜索</NButton
              >
            </div>
            <NDataTable
              :columns="auditColumns"
              :data="logs"
              :loading="loading"
              :scroll-x="1050"
            /><NPagination
              v-model:page="auditPage"
              :item-count="total"
              :page-size="50"
              class="pagination"
              @update:page="run(load)"
            />
          </section>
          <template v-else-if="page === 'settings'"
            ><section class="panel settings">
              <h3>连接与界面设置</h3>
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
                <NButton
                  type="primary"
                  :loading="loading"
                  @click="
                    run(async () => {
                      await api('/settings', 'PUT', settings);
                      notice = '设置已保存，新会话使用新的有效期';
                    })
                  "
                  >保存设置</NButton
                ></NForm
              >
            </section>
            <section class="panel settings">
              <h3>修改管理员密码</h3>
              <NForm
                ><NFormItem label="原密码"
                  ><NInput
                    v-model:value="password.old_password"
                    type="password"
                    autocomplete="current-password" /></NFormItem
                ><NFormItem label="新密码（12～72 字节）"
                  ><NInput
                    v-model:value="password.new_password"
                    type="password"
                    autocomplete="new-password" /></NFormItem
                ><NButton
                  :loading="loading"
                  @click="
                    run(async () => {
                      await api('/auth/password', 'PUT', password);
                      password.old_password = '';
                      password.new_password = '';
                      logged = false;
                      notice = '密码已修改，请重新登录';
                    })
                  "
                  >修改密码并退出</NButton
                ></NForm
              >
            </section></template
          >
          <footer>
            VPN Admin <span>OpenVPN Community · JSON 持久化</span>
          </footer>
        </main>
      </div>
    </div>
    <NModal
      :show="!!modal"
      preset="card"
      :title="modalTitles[modal]"
      :style="{ width: 'min(580px, calc(100vw - 32px))' }"
      :mask-closable="!loading"
      :closable="!loading"
      @update:show="
        (value) => {
          if (!value) modal = '';
        }
      "
    >
      <NAlert v-if="error" type="error" class="banner">{{ error }}</NAlert>
      <template v-if="modal === 'detail'"
        ><NDescriptions bordered :column="1"
          ><NDescriptionsItem
            v-for="[key, label] in [
              ['username', '用户名'],
              ['remark', '备注'],
              ['certificate_serial', '证书序列号'],
              ['endpoint', '连接地址'],
            ]"
            :key="key"
            :label="label"
            >{{ selected[key] || "—" }}</NDescriptionsItem
          ><NDescriptionsItem label="创建时间">{{
            fmt(selected.created_at)
          }}</NDescriptionsItem
          ><NDescriptionsItem label="最近连接">{{
            fmt(selected.last_connected_at)
          }}</NDescriptionsItem
          ><NDescriptionsItem label="证书到期">{{
            fmt(selected.certificate_expires_at)
          }}</NDescriptionsItem></NDescriptions
        ></template
      >
      <NForm
        v-else-if="
          ['create', 'edit-user', 'batch', 'edit-route'].includes(modal)
        "
        ><NFormItem v-if="modal === 'create'" label="用户名"
          ><NInput
            v-model:value="form.username"
            placeholder="例如 macbook，3～32 位小写字母、数字、- 或 _" /></NFormItem
        ><NFormItem v-if="modal === 'create'" label="VPN 连接地址"
          ><NInput v-model:value="form.endpoint" /></NFormItem
        ><NFormItem
          v-if="modal === 'batch'"
          label="目标 IP / CIDR（每行一条，最多 500 行）"
          ><NInput
            v-model:value="form.lines"
            type="textarea"
            :rows="6"
            placeholder="8.8.8.8&#10;20.30.40.0/24" /></NFormItem
        ><NFormItem label="备注"
          ><NInput v-model:value="form.remark" type="textarea" :rows="3"
        /></NFormItem>
        <div v-for="r in batch" :key="r.line" class="batch-result">
          <NTag :type="r.success ? 'success' : 'error'" size="small"
            >第 {{ r.line }} 行</NTag
          >
          {{ r.input }} · {{ r.message }}
        </div></NForm
      >
      <template v-else
        ><NAlert
          :type="modal === 'regenerate' ? 'info' : 'warning'"
          :bordered="false"
          >{{
            modal === "revoke"
              ? "撤销证书不可恢复。在线连接通常在下次 TLS 校验或重连时失效，不会立即踢下线。"
              : modal === "apply"
                ? "应用配置会用已启用路由替换服务器现有分流规则，并重启 OpenVPN、中断当前连接。客户端需要重新连接才能收到新路由。"
                : modal === "restart"
                  ? "重启 OpenVPN 会中断当前 VPN 连接，请确认操作。"
                  : modal === "regenerate"
                    ? "按当前系统设置重新生成，原有已下载配置不会自动更新。创建失败但证书已生成的用户也可通过此操作恢复。"
                    : "删除规则后需应用配置才会生效。"
          }}</NAlert
        ><NFormItem
          v-if="modal === 'revoke'"
          :label="'输入 ' + selected.username + ' 确认撤销'"
          style="margin-top: 20px"
          ><NInput v-model:value="confirmation"
        /></NFormItem>
        <p v-if="modal === 'delete-route'">{{ selected.cidr }}</p></template
      >
      <template #footer
        ><div class="modal-footer">
          <NButton :disabled="loading" @click="modal = ''">{{
            modal === "detail" ? "关闭" : "取消"
          }}</NButton
          ><NButton
            v-if="modal !== 'detail'"
            :type="
              ['revoke', 'delete-route', 'apply', 'restart'].includes(modal)
                ? 'error'
                : 'primary'
            "
            :loading="loading"
            :disabled="modal === 'revoke' && confirmation !== selected.username"
            @click="confirmModal"
            >{{ modal === "batch" ? "添加路由" : "确认" }}</NButton
          >
        </div></template
      >
    </NModal>
  </NConfigProvider>
</template>
