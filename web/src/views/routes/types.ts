export type RouteAction = "batch" | "edit-route" | "delete-route" | "apply";
export interface RouteForm {
  project_id: string;
  lines: string;
  remark: string;
}
