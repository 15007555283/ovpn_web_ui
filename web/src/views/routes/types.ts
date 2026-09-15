export type RouteAction = "batch" | "edit-route" | "delete-route" | "apply";
export interface RouteForm {
  lines: string;
  remark: string;
}
