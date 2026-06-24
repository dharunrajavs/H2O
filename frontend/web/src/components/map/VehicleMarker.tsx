"use client"

interface VehicleMarkerProps {
  heading: number
  speed: number
  ignition: boolean
  isSelected: boolean
}

export function VehicleMarker({ heading, speed, ignition, isSelected }: VehicleMarkerProps) {
  const isMoving = speed > 2

  const color = isSelected
    ? "#F59E0B"           // amber for selected
    : ignition && isMoving
    ? "#10B981"           // green for moving
    : ignition
    ? "#3B82F6"           // blue for ignition-on but idle
    : "#6B7280"           // gray for off

  return (
    <div
      className="relative flex items-center justify-center cursor-pointer transition-transform hover:scale-110"
      style={{ transform: `rotate(${heading}deg)` }}
    >
      {/* Pulsing ring for moving vehicles */}
      {isMoving && (
        <div
          className="absolute rounded-full animate-ping opacity-30"
          style={{
            width: 28,
            height: 28,
            backgroundColor: color,
          }}
        />
      )}

      {/* Car icon SVG */}
      <svg
        width={24}
        height={24}
        viewBox="0 0 24 24"
        fill={color}
        style={{
          filter: isSelected ? `drop-shadow(0 0 4px ${color})` : undefined,
        }}
      >
        <path d="M5 11L6.5 6.5A2 2 0 018.4 5h7.2a2 2 0 011.9 1.5L19 11M5 11v5h1v2a1 1 0 002 0v-2h8v2a1 1 0 002 0v-2h1v-5M5 11h14M7.5 15.5a1 1 0 100-2 1 1 0 000 2zm9 0a1 1 0 100-2 1 1 0 000 2z" />
      </svg>
    </div>
  )
}
