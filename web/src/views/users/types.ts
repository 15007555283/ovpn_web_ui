export type UserAction =
  | "create"
  | "edit-user"
  | "revoke"
  | "regenerate"
  | "detail";
export interface UserForm {
  username: string;
  remark: string;
  endpoint: string;
  confirm: string;
}
