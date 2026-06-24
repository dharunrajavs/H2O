"use client"

import { useEffect, useState } from "react"
import { useDispatch, useSelector } from "react-redux"
import { useQuery } from "@tanstack/react-query"
import { FleetMap } from "@/components/map/FleetMap"
import { StatsCards } from "@/components/dashboard/StatsCards"
import { DeviceList } from "@/components/fleet/DeviceList"
import { AlertPanel } from "@/components/alerts/AlertPanel"
import { devicesApi, fleetApi } from "@/services/api"
import { setPositions, setSummary, selectDevice } from "@/store/slices/fleetSlice"
import { useRealTimeTracking, useInitWS } from "@/hooks/useRealTimeTracking"
import type { RootState } from "@/store"

export function DashboardPage() {
  const dispatch = useDispatch()
  const selectedDeviceId = useSelector((state: RootState) => state.fleet.selectedDeviceId)
  const accessToken = useSelector((state: RootState) => state.auth.accessToken)
  const [showAlerts, setShowAlerts] = useState(false)

  // Re-initialise WS on page load if already authenticated (e.g. token from localStorage)
  useInitWS(accessToken)

  // Fetch all devices
  const { data: devicesData } = useQuery({
    queryKey: ["devices"],
    queryFn: devicesApi.list,
    refetchInterval: 30_000,
  })

  // Fetch fleet summary
  const { data: summaryData } = useQuery({
    queryKey: ["fleet-summary"],
    queryFn: fleetApi.summary,
    refetchInterval: 15_000,
  })

  // Fetch all live positions (initial load)
  const { data: liveData } = useQuery({
    queryKey: ["fleet-live"],
    queryFn: fleetApi.liveAll,
    refetchInterval: 60_000,
  })

  // Seed Redux store with initial live positions
  useEffect(() => {
    if (liveData?.positions) {
      dispatch(setPositions(liveData.positions))
    }
  }, [liveData, dispatch])

  useEffect(() => {
    if (summaryData) dispatch(setSummary(summaryData))
  }, [summaryData, dispatch])

  // Subscribe to real-time GPS updates via WebSocket
  const deviceIds = devicesData?.devices.map((d) => d.id) ?? []
  useRealTimeTracking(deviceIds)

  return (
    <div className="flex flex-col h-screen bg-gray-50">
      {/* Top bar */}
      <header className="bg-white border-b border-gray-200 px-6 py-3 flex items-center justify-between flex-shrink-0">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
            <span className="text-white font-bold text-sm">H2</span>
          </div>
          <h1 className="text-lg font-bold text-gray-900">H2O Fleet Manager</h1>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowAlerts(!showAlerts)}
            className="text-sm text-gray-600 hover:text-gray-900 px-3 py-1.5 rounded-lg hover:bg-gray-100"
          >
            Alerts
          </button>
        </div>
      </header>

      {/* Stats row */}
      <div className="px-6 py-4 flex-shrink-0">
        <StatsCards />
      </div>

      {/* Main content: map + sidebar */}
      <div className="flex-1 flex gap-4 px-6 pb-4 min-h-0">
        {/* Device list sidebar */}
        <div className="w-72 flex-shrink-0">
          <DeviceList
            devices={devicesData?.devices ?? []}
            selectedDeviceId={selectedDeviceId}
            onSelectDevice={(imei) => dispatch(selectDevice(imei))}
          />
        </div>

        {/* Map */}
        <div className="flex-1 rounded-xl overflow-hidden shadow-sm">
          <FleetMap
            onDeviceClick={(imei) => dispatch(selectDevice(imei))}
            selectedDeviceId={selectedDeviceId}
          />
        </div>

        {/* Alert panel */}
        {showAlerts && (
          <div className="w-80 flex-shrink-0">
            <AlertPanel />
          </div>
        )}
      </div>
    </div>
  )
}
