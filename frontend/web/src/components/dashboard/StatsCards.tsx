"use client"

import { Car, Activity, PauseCircle, WifiOff } from "lucide-react"
import { useSelector } from "react-redux"
import type { RootState } from "@/store"

export function StatsCards() {
  const summary = useSelector((state: RootState) => state.fleet.summary)
  const positions = useSelector((state: RootState) => state.fleet.positions)

  const movingCount = Object.values(positions).filter((p) => p.speed > 2).length
  const idleCount = Object.values(positions).filter((p) => p.ignition && p.speed <= 2).length

  const cards = [
    {
      label: "Total Vehicles",
      value: summary?.total_devices ?? 0,
      icon: Car,
      color: "bg-blue-500",
      change: null,
    },
    {
      label: "Moving",
      value: movingCount,
      icon: Activity,
      color: "bg-green-500",
      change: null,
    },
    {
      label: "Idle",
      value: idleCount,
      icon: PauseCircle,
      color: "bg-yellow-500",
      change: null,
    },
    {
      label: "Offline",
      value: (summary?.total_devices ?? 0) - Object.keys(positions).length,
      icon: WifiOff,
      color: "bg-gray-500",
      change: null,
    },
  ]

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
      {cards.map((card) => (
        <div
          key={card.label}
          className="bg-white rounded-xl shadow-sm border border-gray-100 p-4 flex items-center gap-4"
        >
          <div className={`${card.color} p-3 rounded-lg text-white flex-shrink-0`}>
            <card.icon size={20} />
          </div>
          <div>
            <p className="text-2xl font-bold text-gray-900">{card.value}</p>
            <p className="text-sm text-gray-500">{card.label}</p>
          </div>
        </div>
      ))}
    </div>
  )
}
