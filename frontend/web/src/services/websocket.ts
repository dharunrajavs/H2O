import type { WSMessage, WSLocationData } from "@/types"

type LocationHandler = (data: WSLocationData) => void
type AlarmHandler    = (data: WSLocationData) => void

const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:8081/ws"

// ─── GPSWebSocket ─────────────────────────────────────────────────────────────

export class GPSWebSocket {
  private ws: WebSocket | null = null
  private token: string
  private reconnectDelay = 1000
  private maxReconnectDelay = 60_000
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private subscribedDevices: Set<string> = new Set()

  private locationHandlers: Map<string, Set<LocationHandler>> = new Map()
  private alarmHandlers: Map<string, Set<AlarmHandler>> = new Map()
  private globalLocationHandlers: Set<LocationHandler> = new Set()

  constructor(token: string) {
    this.token = token
  }

  connect() {
    if (this.ws?.readyState === WebSocket.OPEN) return

    const url = `${WS_URL}?token=${this.token}`
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      console.log("[WS] connected")
      this.reconnectDelay = 1000

      // Re-subscribe to all tracked devices after reconnect
      if (this.subscribedDevices.size > 0) {
        this.sendSubscribe([...this.subscribedDevices])
      }
    }

    this.ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data)
        this.dispatch(msg)
      } catch (e) {
        console.error("[WS] parse error", e)
      }
    }

    this.ws.onclose = () => {
      console.log(`[WS] disconnected, retrying in ${this.reconnectDelay}ms`)
      this.scheduleReconnect()
    }

    this.ws.onerror = (e) => {
      console.error("[WS] error", e)
    }
  }

  disconnect() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer)
    this.ws?.close(1000, "intentional disconnect")
    this.ws = null
  }

  subscribeDevice(deviceID: string) {
    if (!this.subscribedDevices.has(deviceID)) {
      this.subscribedDevices.add(deviceID)
      this.sendSubscribe([deviceID])
    }
  }

  unsubscribeDevice(deviceID: string) {
    this.subscribedDevices.delete(deviceID)
    this.locationHandlers.delete(deviceID)
    this.alarmHandlers.delete(deviceID)
  }

  onLocation(deviceID: string, handler: LocationHandler): () => void {
    if (!this.locationHandlers.has(deviceID)) {
      this.locationHandlers.set(deviceID, new Set())
    }
    this.locationHandlers.get(deviceID)!.add(handler)
    return () => this.locationHandlers.get(deviceID)?.delete(handler)
  }

  onGlobalLocation(handler: LocationHandler): () => void {
    this.globalLocationHandlers.add(handler)
    return () => this.globalLocationHandlers.delete(handler)
  }

  onAlarm(deviceID: string, handler: AlarmHandler): () => void {
    if (!this.alarmHandlers.has(deviceID)) {
      this.alarmHandlers.set(deviceID, new Set())
    }
    this.alarmHandlers.get(deviceID)!.add(handler)
    return () => this.alarmHandlers.get(deviceID)?.delete(handler)
  }

  private dispatch(msg: WSMessage) {
    switch (msg.type) {
      case "location": {
        const data = msg.data as WSLocationData
        // Notify device-specific handlers
        this.locationHandlers.get(msg.device_id)?.forEach((h) => h(data))
        // Notify global handlers (fleet overview)
        this.globalLocationHandlers.forEach((h) => h(data))
        break
      }
      case "alarm": {
        const data = msg.data as WSLocationData
        this.alarmHandlers.get(msg.device_id)?.forEach((h) => h(data))
        break
      }
    }
  }

  private sendSubscribe(deviceIDs: string[]) {
    if (this.ws?.readyState !== WebSocket.OPEN) return
    this.ws.send(JSON.stringify({ action: "subscribe", devices: deviceIDs }))
  }

  private scheduleReconnect() {
    if (this.reconnectTimer) return
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      this.connect()
    }, this.reconnectDelay)
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay)
  }
}

// Singleton instance (lazy-initialized after login)
let _instance: GPSWebSocket | null = null

export function getWSClient(): GPSWebSocket | null {
  return _instance
}

export function initWSClient(token: string): GPSWebSocket {
  if (_instance) _instance.disconnect()
  _instance = new GPSWebSocket(token)
  _instance.connect()
  return _instance
}

export function destroyWSClient() {
  _instance?.disconnect()
  _instance = null
}
