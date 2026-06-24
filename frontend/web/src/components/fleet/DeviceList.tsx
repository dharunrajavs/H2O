"use client"

import { useState } from "react"
import { useSelector } from "react-redux"
import { Search, Navigation, Zap, ZapOff } from "lucide-react"
import type { RootState } from "@/store"
import type { Device, LivePosition } from "@/types"

interface DeviceListProps {
  devices: Device[]
  onSelectDevice: (imei: string) => void
  selectedDeviceId?: string | null
}

export function DeviceList({ devices, onSelectDevice, selectedDeviceId }: DeviceListProps) {
  const [search, setSearch] = useState("")
  const positions = useSelector((state: RootState) => state.fleet.positions)

  const filtered = devices.filter(
    (d) =>
      d.imei.includes(search) ||
      d.vehicle?.reg_number?.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div className="flex flex-col h-full bg-white rounded-xl shadow-sm border border-gray-100">
      {/* Search */}
      <div className="p-3 border-b border-gray-100">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" size={16} />
          <input
            className="w-full pl-9 pr-3 py-2 text-sm bg-gray-50 rounded-lg border border-gray-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="Search by IMEI or vehicle..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      {/* Device rows */}
      <div className="flex-1 overflow-y-auto">
        {filtered.map((device) => {
          const pos: LivePosition | undefined = positions[device.imei]
          const isOnline = !!pos
          const isMoving = isOnline && pos.speed > 2
          const isSelected = selectedDeviceId === device.imei

          return (
            <button
              key={device.imei}
              onClick={() => onSelectDevice(device.imei)}
              className={`w-full text-left px-4 py-3 border-b border-gray-50 hover:bg-gray-50 transition-colors ${
                isSelected ? "bg-blue-50 border-l-4 border-l-blue-500" : ""
              }`}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  {/* Status dot */}
                  <div
                    className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${
                      isMoving
                        ? "bg-green-500 animate-pulse"
                        : isOnline
                        ? "bg-blue-500"
                        : "bg-gray-300"
                    }`}
                  />
                  <div>
                    <p className="text-sm font-medium text-gray-900">
                      {device.vehicle?.reg_number ?? device.imei}
                    </p>
                    <p className="text-xs text-gray-400">{device.imei}</p>
                  </div>
                </div>

                <div className="flex flex-col items-end gap-1">
                  {isOnline ? (
                    <>
                      <div className="flex items-center gap-1 text-xs text-gray-600">
                        <Navigation size={10} />
                        <span>{pos.speed.toFixed(0)} km/h</span>
                      </div>
                      <div className={`flex items-center gap-1 text-xs ${pos.ignition ? "text-green-600" : "text-gray-400"}`}>
                        {pos.ignition ? <Zap size={10} /> : <ZapOff size={10} />}
                        <span>{pos.ignition ? "ON" : "OFF"}</span>
                      </div>
                    </>
                  ) : (
                    <span className="text-xs text-gray-400">Offline</span>
                  )}
                </div>
              </div>
            </button>
          )
        })}

        {filtered.length === 0 && (
          <div className="p-8 text-center text-sm text-gray-400">No devices found</div>
        )}
      </div>
    </div>
  )
}
