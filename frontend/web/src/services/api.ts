import axios, { AxiosInstance, InternalAxiosRequestConfig } from "axios"
import type {
  AuthTokens,
  Device,
  LivePosition,
  LocationPoint,
  FleetSummary,
  Trip,
  Alert,
  AlertRule,
  Geofence,
} from "@/types"

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081"

// ─── Axios instance ──────────────────────────────────────────────────────────

const http: AxiosInstance = axios.create({
  baseURL: `${BASE_URL}/api/v1`,
  timeout: 15_000,
  headers: { "Content-Type": "application/json" },
})

// Attach JWT to every request
http.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = localStorage.getItem("access_token")
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// Auto-refresh on 401
http.interceptors.response.use(
  (r) => r,
  async (error) => {
    const original = error.config
    if (error.response?.status === 401 && !original._retry) {
      original._retry = true
      try {
        const refreshToken = localStorage.getItem("refresh_token")
        const { data } = await axios.post<AuthTokens>(
          `${BASE_URL}/api/v1/auth/refresh`,
          { refresh_token: refreshToken }
        )
        localStorage.setItem("access_token", data.access_token)
        original.headers.Authorization = `Bearer ${data.access_token}`
        return http(original)
      } catch {
        localStorage.clear()
        window.location.href = "/login"
      }
    }
    return Promise.reject(error)
  }
)

// ─── Auth API ────────────────────────────────────────────────────────────────

export const authApi = {
  login: (email: string, password: string) =>
    http.post<AuthTokens>("/auth/login", { email, password }).then((r) => r.data),

  refresh: (token: string) =>
    http.post<{ access_token: string }>("/auth/refresh", { refresh_token: token }).then((r) => r.data),

  logout: (refreshToken: string) =>
    http.post("/auth/logout", { refresh_token: refreshToken }),
}

// ─── Devices API ─────────────────────────────────────────────────────────────

export const devicesApi = {
  list: () =>
    http.get<{ devices: Device[]; count: number }>("/devices").then((r) => r.data),

  get: (imei: string) =>
    http.get<Device>(`/devices/${imei}`).then((r) => r.data),

  create: (data: Partial<Device>) =>
    http.post<{ id: string; imei: string }>("/devices", data).then((r) => r.data),

  update: (imei: string, data: Partial<Device>) =>
    http.put(`/devices/${imei}`, data).then((r) => r.data),

  delete: (imei: string) =>
    http.delete(`/devices/${imei}`).then((r) => r.data),

  getLive: (imei: string) =>
    http.get<LivePosition>(`/devices/${imei}/live`).then((r) => r.data),

  getHistory: (imei: string, from: Date, to: Date, limit = 1000) =>
    http
      .get<{ points: LocationPoint[]; count: number }>(`/devices/${imei}/history`, {
        params: {
          from: from.toISOString(),
          to: to.toISOString(),
          limit,
        },
      })
      .then((r) => r.data),

  getTrips: (imei: string) =>
    http.get<{ trips: Trip[] }>(`/devices/${imei}/trips`).then((r) => r.data),
}

// ─── Fleet API ───────────────────────────────────────────────────────────────

export const fleetApi = {
  summary: () =>
    http.get<FleetSummary>("/fleet").then((r) => r.data),

  liveAll: () =>
    http.get<{ positions: LivePosition[]; count: number }>("/fleet/live").then((r) => r.data),

  heatmap: () =>
    http.get<{ points: { lat: number; lng: number; weight: number }[] }>("/fleet/heatmap").then((r) => r.data),
}

// ─── Alerts API ──────────────────────────────────────────────────────────────

export const alertsApi = {
  list: () =>
    http.get<{ alerts: Alert[]; count: number }>("/alerts").then((r) => r.data),

  markRead: (id: string) =>
    http.put(`/alerts/${id}/read`).then((r) => r.data),

  listRules: () =>
    http.get<{ rules: AlertRule[] }>("/alert-rules").then((r) => r.data),

  createRule: (data: Partial<AlertRule>) =>
    http.post<{ id: string }>("/alert-rules", data).then((r) => r.data),

  deleteRule: (id: string) =>
    http.delete(`/alert-rules/${id}`).then((r) => r.data),
}

// ─── Geofences API ───────────────────────────────────────────────────────────

export const geofencesApi = {
  list: () =>
    http.get<{ geofences: Geofence[] }>("/geofences").then((r) => r.data),

  create: (data: Omit<Geofence, "id">) =>
    http.post<{ id: string }>("/geofences", data).then((r) => r.data),

  update: (id: string, data: Partial<Geofence>) =>
    http.put(`/geofences/${id}`, data).then((r) => r.data),

  delete: (id: string) =>
    http.delete(`/geofences/${id}`).then((r) => r.data),
}
