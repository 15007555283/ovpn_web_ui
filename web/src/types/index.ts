export type PageKey =
  | "dashboard"
  | "users"
  | "routes"
  | "online"
  | "system"
  | "audit"
  | "settings";
export interface PageView {
  refresh(): Promise<void>;
  isEditing?: boolean;
}
export interface Settings {
  endpoint: string;
  port: number;
  protocol: string;
  dns: string;
  refresh: number;
  session_hours: number;
  name: string;
}
export interface VPNUser {
  id: string;
  username: string;
  remark: string;
  status: string;
  endpoint: string;
  certificate_serial: string;
  certificate_expires_at: string;
  created_at: string;
  last_connected_at: string;
}
export interface Route {
  id: string;
  cidr: string;
  remark: string;
  enabled: boolean;
  updated_at: string;
}
export interface RouteRevision {
  revision: number;
  status: string;
  created_at: string;
}
export interface PendingRoutes {
  revision: number;
  applied_revision: number;
  pending: boolean;
  history: RouteRevision[];
}
export interface BatchResult {
  line: number;
  input: string;
  success: boolean;
  message: string;
}
export interface OnlineClient {
  username: string;
  vpn_ip: string;
  remote: string;
  connected_at: string;
  bytes_received: number;
  bytes_sent: number;
}
export interface OnlineStatus {
  available: boolean;
  message?: string;
  updated_at?: string;
  clients: OnlineClient[];
}
export interface AuditLog {
  created_at: string;
  admin: string;
  action: string;
  resource_id: string;
  source_ip: string;
  success: boolean;
  detail: string;
  request_id: string;
}
export interface Health extends Record<string, unknown> {
  service?: string;
  vpn_network?: string;
  started_at?: string;
  error?: string;
  certificate_error?: string;
  routes_error?: string;
}
export interface Dashboard {
  online: OnlineStatus;
  health: Health;
  total_users: number | null;
  active_certificates: number | null;
  revoked_certificates: number | null;
  routes: number | null;
  recent: AuditLog[];
  settings: Settings;
}
