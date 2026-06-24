"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { alertsApi } from "@/services/api"
import { AlertTriangle, Zap, MapPin, Clock } from "lucide-react"
import { formatDistanceToNow } from "date-fns"
import type { Alert } from "@/types"

const SEVERITY_STYLES: Record<string, string> = {
  critical: "border-l-4 border-red-500 bg-red-50",
  warning:  "border-l-4 border-yellow-400 bg-yellow-50",
  info:     "border-l-4 border-blue-400 bg-blue-50",
}

const TYPE_ICONS: Record<string, React.ElementType> = {
  overspeed:   Zap,
  geofence_in: MapPin,
  geofence_out: MapPin,
  sos:         AlertTriangle,
  power_cut:   Zap,
}

export function AlertPanel() {
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ["alerts"],
    queryFn: alertsApi.list,
    refetchInterval: 10_000,
  })

  const markRead = useMutation({
    mutationFn: alertsApi.markRead,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alerts"] }),
  })

  if (isLoading) {
    return <div className="bg-white rounded-xl shadow-sm p-4 text-sm text-gray-400">Loading alerts…</div>
  }

  const alerts = data?.alerts ?? []
  const unread = alerts.filter((a) => !a.is_read)

  return (
    <div className="flex flex-col bg-white rounded-xl shadow-sm border border-gray-100 h-full">
      <div className="px-4 py-3 border-b border-gray-100 flex items-center justify-between">
        <h2 className="font-semibold text-gray-900 text-sm">Alerts</h2>
        {unread.length > 0 && (
          <span className="bg-red-500 text-white text-xs font-bold px-2 py-0.5 rounded-full">
            {unread.length}
          </span>
        )}
      </div>

      <div className="flex-1 overflow-y-auto divide-y divide-gray-50">
        {alerts.length === 0 && (
          <div className="p-8 text-center text-sm text-gray-400">No alerts</div>
        )}
        {alerts.map((alert) => (
          <AlertRow
            key={alert.id}
            alert={alert}
            onRead={() => markRead.mutate(alert.id)}
          />
        ))}
      </div>
    </div>
  )
}

function AlertRow({ alert, onRead }: { alert: Alert; onRead: () => void }) {
  const Icon = TYPE_ICONS[alert.type] ?? AlertTriangle

  return (
    <div
      className={`px-4 py-3 ${SEVERITY_STYLES[alert.severity] ?? ""} ${alert.is_read ? "opacity-60" : ""}`}
    >
      <div className="flex items-start gap-3">
        <Icon size={16} className="mt-0.5 flex-shrink-0 text-gray-600" />
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <p className="text-xs font-semibold text-gray-800 uppercase tracking-wide">
              {alert.type.replace(/_/g, " ")}
            </p>
            {!alert.is_read && (
              <button
                onClick={onRead}
                className="text-xs text-blue-500 hover:text-blue-700 flex-shrink-0"
              >
                Mark read
              </button>
            )}
          </div>
          <p className="text-xs text-gray-500 mt-0.5">{alert.imei}</p>
          {alert.message && (
            <p className="text-xs text-gray-600 mt-1">{alert.message}</p>
          )}
          <div className="flex items-center gap-1 mt-1 text-xs text-gray-400">
            <Clock size={10} />
            <span>{formatDistanceToNow(new Date(alert.triggered_at), { addSuffix: true })}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
