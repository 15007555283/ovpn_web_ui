<script setup lang="ts">
import { computed, h, ref } from "vue";
import { NButton, NDataTable, NInput, NSelect } from "naive-ui";
import { api, downloadFile } from "../../api/client";
import { useAppContext } from "../../composables/useAppContext";
import { fmt } from "../../utils/format";
import { tableButton, tag } from "../../utils/table";
import type { VPNUser, OnlineStatus } from "../../types";
import type { UserAction, UserForm } from "./types";
import UserDialog from "./UserDialog.vue";
const { loading, error, notice, run } = useAppContext();
const users = ref<VPNUser[]>([]),
  online = ref<OnlineStatus>({ available: false, clients: [] });
const search = ref(""),
  statusFilter = ref<string | null>(null);
const modal = ref<UserAction | "">(""),
  selected = ref<VPNUser>();
const isEditing = computed(() => !!modal.value);
const labels: Record<string, string> = {
  active: "有效",
  revoked: "已撤销",
  create_failed: "创建失败",
  revoke_pending: "撤销待完成",
};
const filteredUsers = computed(() =>
  users.value.filter(
    (u) =>
      u.username.includes(search.value) &&
      (!statusFilter.value || u.status === statusFilter.value),
  ),
);
const client = (user: VPNUser) =>
  online.value.clients.find((c) => c.username === user.username);
const button = (
  text: string,
  action: () => void,
  danger = false,
  disabled = false,
) => tableButton(text, action, danger, disabled || loading.value);
const download = (path: string, name: string) =>
  run(() => downloadFile(path, name));
function open(action: UserAction, user?: VPNUser) {
  error.value = "";
  selected.value = user;
  modal.value = action;
}
async function refresh() {
  users.value = await api<VPNUser[]>("/vpn-users");
  online.value = await api<OnlineStatus>("/sessions/online");
}
async function save(form: UserForm) {
  await run(async () => {
    const path = "/vpn-users/" + selected.value?.id;
    switch (modal.value) {
      case "create":
        await api("/vpn-users", "POST", {
          username: form.username,
          remark: form.remark,
          endpoint: form.endpoint,
        });
        notice.value = "用户创建成功，可以下载客户端配置";
        break;
      case "edit-user":
        await api(path, "PUT", { remark: form.remark });
        break;
      case "revoke":
        await api(path + "/revoke", "POST", { confirm: form.confirm });
        notice.value = "证书已撤销；通常在下次 TLS 校验或重连时生效";
        break;
      case "regenerate":
        await api(path + "/regenerate-config", "POST");
        notice.value = "已按当前设置重新生成，请重新下载配置";
        break;
    }
    modal.value = "";
    await refresh();
  });
}
defineExpose({ refresh, isEditing });
const userColumns = [
  {
    title: "用户名",
    key: "username",
    minWidth: 120,
    render: (r: VPNUser) => button(r.username, () => open("detail", r)),
  },
  {
    title: "证书状态",
    key: "status",
    minWidth: 115,
    render: (r: VPNUser) =>
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
    render: (r: VPNUser) =>
      online.value.available
        ? client(r)
          ? tag("在线 · " + client(r)?.vpn_ip, "success")
          : "离线 · —"
        : tag("状态暂不可用", "warning"),
  },
  { title: "备注", key: "remark", minWidth: 150 },
  {
    title: "证书到期",
    key: "certificate_expires_at",
    minWidth: 170,
    render: (r: VPNUser) => fmt(r.certificate_expires_at),
  },
  {
    title: "创建时间",
    key: "created_at",
    minWidth: 170,
    render: (r: VPNUser) => fmt(r.created_at),
  },
  {
    title: "最后连接",
    key: "last_connected_at",
    minWidth: 170,
    render: (r: VPNUser) => fmt(r.last_connected_at),
  },
  {
    title: "操作",
    key: "actions",
    minWidth: 250,
    render: (r: VPNUser) =>
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
</script>
<template>
  <section class="panel">
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
      /><NButton type="primary" @click="open('create')">＋ 创建用户</NButton>
    </div>
    <NDataTable
      :columns="userColumns"
      :data="filteredUsers"
      :loading="loading"
      :scroll-x="1450"
      :pagination="{ pageSize: 15 }"
    />
  </section>
  <UserDialog
    v-if="modal"
    :action="modal"
    :user="selected"
    @close="modal = ''"
    @submit="save"
  />
</template>
