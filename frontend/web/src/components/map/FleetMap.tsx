"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import Map, { Marker, Popup, NavigationControl, Source, Layer } from "react-map-gl"
import type { MapRef } from "react-map-gl"
import { useSelector } from "react-redux"
import type { RootState } from "@/store"
import type { LivePosition } from "@/types"
import { VehicleMarker } from "./VehicleMarker"
import "mapbox-gl/dist/mapbox-gl.css"

const MAPBOX_TOKEN = process.env.NEXT_PUBLIC_MAPBOX_TOKEN ?? ""

interface FleetMapProps {
  onDeviceClick?: (imei: string) => void
  selectedDeviceId?: string | null
  showHeatmap?: boolean
}

export function FleetMap({ onDeviceClick, selectedDeviceId, showHeatmap }: FleetMapProps) {
  const mapRef = useRef<MapRef>(null)
  const positions = useSelector((state: RootState) => state.fleet.positions)
  const [popup, setPopup] = useState<LivePosition | null>(null)

  // Fly to selected device
  useEffect(() => {
    if (!selectedDeviceId || !mapRef.current) return
    const pos = positions[selectedDeviceId]
    if (!pos) return

    mapRef.current.flyTo({
      center: [pos.lng, pos.lat],
      zoom: 16,
      speed: 1.5,
    })
  }, [selectedDeviceId, positions])

  const handleMarkerClick = useCallback(
    (pos: LivePosition) => {
      setPopup(pos)
      onDeviceClick?.(pos.imei)
    },
    [onDeviceClick]
  )

  if (!MAPBOX_TOKEN) {
    return (
      <div className="w-full h-full flex flex-col items-center justify-center bg-slate-800 rounded-xl text-slate-400 gap-3">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7"/>
        </svg>
        <div className="text-center">
          <p className="font-medium text-slate-300">Map not configured</p>
          <p className="text-xs mt-1">Add <code className="text-blue-400">NEXT_PUBLIC_MAPBOX_TOKEN</code> to <code className="text-blue-400">.env.local</code></p>
          <p className="text-xs text-slate-500 mt-2">Get a free token at <span className="text-blue-400">mapbox.com</span></p>
        </div>
        <div className="mt-2 text-xs text-slate-600">
          {Object.keys(positions).length > 0
            ? `${Object.keys(positions).length} device(s) tracked`
            : "Waiting for device data…"}
        </div>
      </div>
    )
  }

  return (
    <Map
      ref={mapRef}
      mapboxAccessToken={MAPBOX_TOKEN}
      initialViewState={{
        longitude: 77.5946,
        latitude: 12.9716,
        zoom: 11,
      }}
      style={{ width: "100%", height: "100%" }}
      mapStyle="mapbox://styles/mapbox/dark-v11"
    >
      <NavigationControl position="top-right" />

      {/* Vehicle markers */}
      {Object.values(positions).map((pos) => (
        <Marker
          key={pos.imei}
          longitude={pos.lng}
          latitude={pos.lat}
          anchor="center"
          onClick={() => handleMarkerClick(pos)}
        >
          <VehicleMarker
            heading={pos.heading}
            speed={pos.speed}
            ignition={pos.ignition}
            isSelected={selectedDeviceId === pos.imei}
          />
        </Marker>
      ))}

      {/* Popup on click */}
      {popup && (
        <Popup
          longitude={popup.lng}
          latitude={popup.lat}
          anchor="bottom"
          onClose={() => setPopup(null)}
          closeButton
          closeOnClick={false}
        >
          <DevicePopup position={popup} />
        </Popup>
      )}
    </Map>
  )
}

function DevicePopup({ position }: { position: LivePosition }) {
  return (
    <div className="p-2 min-w-[200px]">
      <div className="font-bold text-sm mb-1">{position.imei}</div>
      <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-gray-600">
        <span>Speed</span>
        <span className="font-medium">{position.speed.toFixed(1)} km/h</span>
        <span>Heading</span>
        <span className="font-medium">{position.heading.toFixed(0)}°</span>
        <span>Ignition</span>
        <span className={position.ignition ? "text-green-600 font-medium" : "text-red-500 font-medium"}>
          {position.ignition ? "ON" : "OFF"}
        </span>
        <span>GPS</span>
        <span className={position.gps_fixed ? "text-green-600 font-medium" : "text-yellow-500 font-medium"}>
          {position.gps_fixed ? "Fixed" : "LBS"}
        </span>
        <span>Updated</span>
        <span className="font-medium text-gray-500">
          {new Date(position.updated_at).toLocaleTimeString()}
        </span>
      </div>
    </div>
  )
}
