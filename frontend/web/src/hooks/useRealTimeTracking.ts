import { useEffect, useCallback, useRef } from "react"
import { useDispatch } from "react-redux"
import { updatePosition } from "@/store/slices/fleetSlice"
import { getWSClient, initWSClient } from "@/services/websocket"
import type { LivePosition, WSLocationData } from "@/types"

// useRealTimeTracking subscribes to live GPS updates for a list of devices.
// Returns the cleanup function to unsubscribe.
export function useRealTimeTracking(deviceIds: string[]) {
  const dispatch = useDispatch()
  const prevDevices = useRef<string[]>([])

  const handleLocation = useCallback(
    (data: WSLocationData) => {
      const pos: LivePosition = {
        imei: data.imei,
        lat: data.lat,
        lng: data.lng,
        speed: data.speed,
        heading: data.heading,
        ignition: data.ignition,
        gps_fixed: data.gps_fixed,
        recorded_at: data.recorded_at,
        updated_at: data.received_at,
      }
      dispatch(updatePosition({ deviceId: data.device_id, position: pos }))
    },
    [dispatch]
  )

  useEffect(() => {
    const ws = getWSClient()
    if (!ws) return

    const newDevices = deviceIds.filter((id) => !prevDevices.current.includes(id))
    newDevices.forEach((id) => {
      ws.subscribeDevice(id)
    })

    prevDevices.current = deviceIds
  }, [deviceIds])

  useEffect(() => {
    const ws = getWSClient()
    if (!ws) return

    // Subscribe to all location events globally (fleet map)
    const unsub = ws.onGlobalLocation(handleLocation)
    return unsub
  }, [handleLocation])
}

// useDeviceTracking tracks a single device
export function useDeviceTracking(deviceId: string | null, onUpdate?: (pos: LivePosition) => void) {
  const dispatch = useDispatch()

  useEffect(() => {
    if (!deviceId) return

    const ws = getWSClient()
    if (!ws) return

    ws.subscribeDevice(deviceId)

    const unsub = ws.onLocation(deviceId, (data: WSLocationData) => {
      const pos: LivePosition = {
        imei: data.imei,
        lat: data.lat,
        lng: data.lng,
        speed: data.speed,
        heading: data.heading,
        ignition: data.ignition,
        gps_fixed: data.gps_fixed,
        recorded_at: data.recorded_at,
        updated_at: data.received_at,
      }
      dispatch(updatePosition({ deviceId, position: pos }))
      onUpdate?.(pos)
    })

    return () => {
      unsub()
    }
  }, [deviceId, dispatch, onUpdate])
}

// useInitWS initializes or reinitializes the WebSocket client
export function useInitWS(token: string | null) {
  useEffect(() => {
    if (!token) return
    const ws = initWSClient(token)
    return () => ws.disconnect()
  }, [token])
}
