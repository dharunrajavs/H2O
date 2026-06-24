// ─── Core domain types ───────────────────────────────────────────────────────

export interface Device {
  id: string
  imei: string
  vehicle_id?: string
  model: string
  sim_number: string
  sim_operator: string
  is_active: boolean
  last_seen_at?: string
  installed_at?: string
  vehicle?: Vehicle
}

export interface Vehicle {
  id: string
  reg_number: string
  make?: string
  model?: string
  year?: number
  fuel_type?: string
  color?: string
  icon: string
}

export interface LivePosition {
  imei: string
  lat: number
  lng: number
  speed: number
  heading: number
  ignition: boolean
  gps_fixed: boolean
  recorded_at: string
  updated_at: string
  device?: Device
}

export interface LocationPoint {
  lat: number
  lng: number
  speed: number
  heading: number
  ignition: boolean
  gps_fixed: boolean
  satellites: number
  recorded_at: string
}

export interface Trip {
  id: string
  start_lat: number
  start_lng: number
  end_lat: number
  end_lng: number
  distance_km: number
  max_speed: number
  avg_speed: number
  duration_secs: number
  started_at: string
  ended_at?: string
}

export interface Geofence {
  id: string
  name: string
  type: "circle" | "polygon" | "rectangle"
  color: string
  coordinates: GeoJSON.Geometry
  center_lat?: number
  center_lng?: number
  radius_m?: number
}

export interface Alert {
  id: string
  type: string
  severity: "info" | "warning" | "critical"
  lat?: number
  lng?: number
  speed?: number
  message?: string
  is_read: boolean
  triggered_at: string
  imei: string
}

export interface AlertRule {
  id: string
  name: string
  type: string
  device_ids: string[]
  config: Record<string, unknown>
  channels: Record<string, boolean>
  is_active: boolean
}

export interface FleetSummary {
  total_devices: number
  active_devices: number
  moving: number
  idle: number
}

// ─── WebSocket message types ──────────────────────────────────────────────────

export type WSMessageType = "location" | "alarm" | "subscribed" | "pong"

export interface WSMessage<T = unknown> {
  type: WSMessageType
  device_id: string
  ts: string
  data: T
}

export interface WSLocationData {
  imei: string
  tenant_id: string
  device_id: string
  lat: number
  lng: number
  speed: number
  heading: number
  satellites: number
  gps_fixed: boolean
  ignition: boolean
  alarm_type?: number
  recorded_at: string
  received_at: string
}

// ─── API response types ───────────────────────────────────────────────────────

export interface PaginatedResponse<T> {
  data: T[]
  count: number
  page: number
  total?: number
}

export interface AuthTokens {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: "Bearer"
}

export interface User {
  id: string
  email: string
  name: string
  role: "admin" | "manager" | "viewer"
  tenant_id: string
}
