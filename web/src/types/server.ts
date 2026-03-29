export interface DatabaseServer {
  id: number;
  name: string;
  hostname: string;
  port: number;
  server_type: string;
  version: string;
  status: 'active' | 'inactive' | 'maintenance' | 'error';
  max_connections: number;
  current_connections: number;
  team_id?: number;
  created_at: string;
  updated_at: string;
}

export interface ManagedDatabase {
  id: number;
  name: string;
  server_id: number;
  server_name?: string;
  size_mb: number;
  charset: string;
  collation: string;
  status: 'active' | 'suspended' | 'archived';
  owner_id?: number;
  team_id?: number;
  created_at: string;
  updated_at: string;
}

export interface SqlFile {
  id: number;
  name: string;
  description?: string;
  content: string;
  database_id?: number;
  status: 'pending' | 'approved' | 'rejected' | 'executed';
  submitted_by?: number;
  reviewed_by?: number;
  created_at: string;
  executed_at?: string;
}

export interface SecurityRule {
  id: number;
  name: string;
  rule_type: 'firewall' | 'access_control' | 'audit' | 'encryption';
  description?: string;
  pattern: string;
  action: 'allow' | 'deny' | 'log';
  priority: number;
  enabled: boolean;
  server_id?: number;
  created_at: string;
}

export interface ThreatIntelEntry {
  id: number;
  source: string;
  threat_type: string;
  indicator: string;
  severity: 'critical' | 'high' | 'medium' | 'low';
  description?: string;
  first_seen: string;
  last_seen: string;
  is_active: boolean;
}

export interface BlockedDatabase {
  id: number;
  name: string;
  reason: string;
  blocked_by?: number;
  server_id?: number;
  created_at: string;
  expires_at?: string;
}

export interface CloudProvider {
  id: number;
  name: string;
  provider_type: string;
  region: string;
  status: 'connected' | 'disconnected' | 'error';
  endpoint_url?: string;
  created_at: string;
}

export interface ScalingPolicy {
  id: number;
  name: string;
  server_id: number;
  server_name?: string;
  metric: string;
  threshold_up: number;
  threshold_down: number;
  scale_up_by: number;
  scale_down_by: number;
  cooldown_seconds: number;
  enabled: boolean;
  last_triggered?: string;
  created_at: string;
}

export interface TemporaryAccess {
  id: number;
  user_id: number;
  user_email?: string;
  database_id: number;
  database_name?: string;
  permission_level: 'read' | 'write' | 'admin';
  reason: string;
  granted_by?: number;
  granted_at: string;
  expires_at: string;
  revoked_at?: string;
  status: 'active' | 'expired' | 'revoked';
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  per_page: number;
}
