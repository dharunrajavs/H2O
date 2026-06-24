// React Native WebSocket service with auto-reconnect and background support

const WS_URL = process.env.REACT_APP_WS_URL ?? "ws://localhost:8081/ws"

type LocationHandler = (data: LiveLocationEvent) => void
type AlarmHandler = (data: AlarmEvent) => void

export interface LiveLocationEvent {
  imei: string
  device_id: string
  lat: number
  lng: number
  speed: number
  heading: number
  ignition: boolean
  gps_fixed: boolean
  recorded_at: string
}

export interface AlarmEvent extends LiveLocationEvent {
  alarm_code: number
  alarm_name: string
}

export class MobileGPSWebSocket {
  private ws: WebSocket | null = null
  private token: string
  private reconnectDelay = 1000
  private maxDelay = 60_000
  private reconnectHandle: ReturnType<typeof setTimeout> | null = null
  private subscribedDevices = new Set<string>()
  private locationHandlers = new Set<LocationHandler>()
  private alarmHandlers = new Set<AlarmHandler>()
  private connectHandlers = new Set<() => void>()
  private disconnectHandlers = new Set<() => void>()

  constructor(token: string) {
    this.token = token
  }

  connect() {
    if (this.ws?.readyState === WebSocket.OPEN) return

    this.ws = new WebSocket(`${WS_URL}?token=${this.token}`)

    this.ws.onopen = () => {
      this.reconnectDelay = 1000
      this.connectHandlers.forEach((h) => h())

      if (this.subscribedDevices.size > 0) {
        this.subscribe([...this.subscribedDevices])
      }
    }

    this.ws.onmessage = ({ data }) => {
      try {
        const msg = JSON.parse(data)
        if (msg.type === "location") {
          this.locationHandlers.forEach((h) => h(msg.data))
        } else if (msg.type === "alarm") {
          this.alarmHandlers.forEach((h) => h(msg.data))
        }
      } catch {}
    }

    this.ws.onclose = () => {
      this.disconnectHandlers.forEach((h) => h())
      this.scheduleReconnect()
    }
  }

  disconnect() {
    if (this.reconnectHandle) clearTimeout(this.reconnectHandle)
    this.ws?.close()
    this.ws = null
  }

  subscribe(deviceIds: string[]) {
    deviceIds.forEach((id) => this.subscribedDevices.add(id))
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ action: "subscribe", devices: deviceIds }))
    }
  }

  onLocation(handler: LocationHandler) {
    this.locationHandlers.add(handler)
    return () => this.locationHandlers.delete(handler)
  }

  onAlarm(handler: AlarmHandler) {
    this.alarmHandlers.add(handler)
    return () => this.alarmHandlers.delete(handler)
  }

  onConnect(handler: () => void) {
    this.connectHandlers.add(handler)
    return () => this.connectHandlers.delete(handler)
  }

  onDisconnect(handler: () => void) {
    this.disconnectHandlers.add(handler)
    return () => this.disconnectHandlers.delete(handler)
  }

  get isConnected() {
    return this.ws?.readyState === WebSocket.OPEN
  }

  private scheduleReconnect() {
    if (this.reconnectHandle) return
    this.reconnectHandle = setTimeout(() => {
      this.reconnectHandle = null
      this.connect()
    }, this.reconnectDelay)
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxDelay)
  }
}
