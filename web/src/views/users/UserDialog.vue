<script setup lang="ts">
import { reactive } from "vue";
import {
  NAlert,
  NButton,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NInput,
  NModal,
} from "naive-ui";
import { useAppContext } from "../../composables/useAppContext";
import { fmt } from "../../utils/format";
import type { VPNUser } from "../../types";
import type { UserAction, UserForm } from "./types";
const props = defineProps<{ action: UserAction; user?: VPNUser }>();
const emit = defineEmits<{ close: []; submit: [form: UserForm] }>();
const { settings, loading, error } = useAppContext();
const form = reactive<UserForm>({
  username: "",
  remark: props.user?.remark ?? "",
  endpoint: settings.endpoint,
  confirm: "",
});
const titles: Record<UserAction, string> = {
  create: "创建 VPN 用户",
  "edit-user": "编辑用户备注",
  revoke: "撤销用户证书",
  regenerate: "重新生成客户端配置",
  detail: "用户详情",
};
const detailFields: { key: keyof VPNUser; label: string }[] = [
  { key: "username", label: "用户名" },
  { key: "remark", label: "备注" },
  { key: "certificate_serial", label: "证书序列号" },
  { key: "endpoint", label: "连接地址" },
];
</script>
<template>
  <NModal
    :show="true"
    preset="card"
    :title="titles[action]"
    :style="{ width: 'min(580px, calc(100vw - 32px))' }"
    :mask-closable="!loading"
    :closable="!loading"
    @update:show="
      (value) => {
        if (!value) emit('close');
      }
    "
  >
    <NAlert v-if="error" type="error" class="banner">{{ error }}</NAlert>
    <NDescriptions v-if="action === 'detail' && user" bordered :column="1">
      <NDescriptionsItem
        v-for="field in detailFields"
        :key="field.key"
        :label="field.label"
        >{{ user[field.key] || "—" }}</NDescriptionsItem
      >
      <NDescriptionsItem label="创建时间">{{
        fmt(user.created_at)
      }}</NDescriptionsItem>
      <NDescriptionsItem label="最近连接">{{
        fmt(user.last_connected_at)
      }}</NDescriptionsItem>
      <NDescriptionsItem label="证书到期">{{
        fmt(user.certificate_expires_at)
      }}</NDescriptionsItem>
    </NDescriptions>
    <NForm v-else-if="action === 'create' || action === 'edit-user'">
      <NFormItem v-if="action === 'create'" label="用户名"
        ><NInput
          v-model:value="form.username"
          placeholder="例如 macbook，3～32 位小写字母、数字、- 或 _"
      /></NFormItem>
      <NFormItem v-if="action === 'create'" label="VPN 连接地址"
        ><NInput v-model:value="form.endpoint"
      /></NFormItem>
      <NFormItem label="备注"
        ><NInput v-model:value="form.remark" type="textarea" :rows="3"
      /></NFormItem>
    </NForm>
    <template v-else>
      <NAlert
        :type="action === 'regenerate' ? 'info' : 'warning'"
        :bordered="false"
        >{{
          action === "revoke"
            ? "撤销证书不可恢复。在线连接通常在下次 TLS 校验或重连时失效，不会立即踢下线。"
            : "按当前系统设置重新生成，原有已下载配置不会自动更新。创建失败但证书已生成的用户也可通过此操作恢复。"
        }}</NAlert
      >
      <NFormItem
        v-if="action === 'revoke'"
        :label="'输入 ' + user?.username + ' 确认撤销'"
        style="margin-top: 20px"
        ><NInput v-model:value="form.confirm"
      /></NFormItem>
    </template>
    <template #footer
      ><div class="modal-footer">
        <NButton :disabled="loading" @click="emit('close')">{{
          action === "detail" ? "关闭" : "取消"
        }}</NButton>
        <NButton
          v-if="action !== 'detail'"
          :type="action === 'revoke' ? 'error' : 'primary'"
          :loading="loading"
          :disabled="action === 'revoke' && form.confirm !== user?.username"
          @click="emit('submit', form)"
          >确认</NButton
        >
      </div></template
    >
  </NModal>
</template>
