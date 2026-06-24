import React, { useEffect, useState, useCallback, useRef } from "react"
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  StatusBar,
  Platform,
  Dimensions,
} from "react-native"
import MapView, { Marker, Polyline, PROVIDER_GOOGLE } from "react-native-maps"
import type { Region } from "react-native-maps"
import type { LiveLocationEvent } from "../services/websocket"

interface Props {
  deviceId: string
  imei: string
  vehicleReg: string
  wsClient: any  // MobileGPSWebSocket
}

interface RoutePoint {
  latitude: number
  longitude: number
}

const { height } = Dimensions.get("window")

export function TrackingScreen({ deviceId, imei, vehicleReg, wsClient }: Props) {
  const mapRef = useRef<MapView>(null)
  const [position, setPosition] = useState<LiveLocationEvent | null>(null)
  const [route, setRoute] = useState<RoutePoint[]>([])
  const [isFollowing, setIsFollowing] = useState(true)
  const [isConnected, setIsConnected] = useState(false)

  useEffect(() => {
    if (!wsClient) return

    wsClient.subscribe([deviceId])

    const unsubLoc = wsClient.onLocation((data: LiveLocationEvent) => {
      if (data.device_id !== deviceId) return

      setPosition(data)
      setRoute((prev) => {
        const newRoute = [...prev, { latitude: data.lat, longitude: data.lng }]
        return newRoute.slice(-500) // keep last 500 points
      })

      if (isFollowing && mapRef.current) {
        mapRef.current.animateToRegion(
          {
            latitude: data.lat,
            longitude: data.lng,
            latitudeDelta: 0.005,
            longitudeDelta: 0.005,
          },
          500
        )
      }
    })

    const unsubConnect = wsClient.onConnect(() => setIsConnected(true))
    const unsubDisconnect = wsClient.onDisconnect(() => setIsConnected(false))
    setIsConnected(wsClient.isConnected)

    return () => {
      unsubLoc()
      unsubConnect()
      unsubDisconnect()
    }
  }, [deviceId, wsClient, isFollowing])

  const initialRegion: Region = {
    latitude: position?.lat ?? 12.9716,
    longitude: position?.lng ?? 77.5946,
    latitudeDelta: 0.02,
    longitudeDelta: 0.02,
  }

  return (
    <View style={styles.container}>
      <StatusBar barStyle="light-content" backgroundColor="#1E293B" />

      {/* Header */}
      <View style={styles.header}>
        <View>
          <Text style={styles.headerTitle}>{vehicleReg}</Text>
          <Text style={styles.headerSub}>{imei}</Text>
        </View>
        <View style={[styles.statusBadge, isConnected ? styles.statusOnline : styles.statusOffline]}>
          <Text style={styles.statusText}>{isConnected ? "LIVE" : "OFFLINE"}</Text>
        </View>
      </View>

      {/* Map */}
      <MapView
        ref={mapRef}
        provider={PROVIDER_GOOGLE}
        style={styles.map}
        initialRegion={initialRegion}
        showsUserLocation={false}
        showsTraffic={false}
        onPanDrag={() => setIsFollowing(false)}
      >
        {/* Route trail */}
        {route.length > 1 && (
          <Polyline
            coordinates={route}
            strokeColor="#3B82F6"
            strokeWidth={3}
            lineDashPattern={[1]}
          />
        )}

        {/* Vehicle marker */}
        {position && (
          <Marker
            coordinate={{ latitude: position.lat, longitude: position.lng }}
            title={vehicleReg}
            description={`${position.speed.toFixed(0)} km/h`}
            rotation={position.heading}
            anchor={{ x: 0.5, y: 0.5 }}
          />
        )}
      </MapView>

      {/* Follow button */}
      {!isFollowing && (
        <TouchableOpacity
          style={styles.followBtn}
          onPress={() => setIsFollowing(true)}
        >
          <Text style={styles.followBtnText}>📍 Follow Vehicle</Text>
        </TouchableOpacity>
      )}

      {/* Stats panel */}
      {position && (
        <View style={styles.statsPanel}>
          <StatItem label="Speed" value={`${position.speed.toFixed(0)} km/h`} />
          <StatItem label="Heading" value={`${position.heading.toFixed(0)}°`} />
          <StatItem
            label="Ignition"
            value={position.ignition ? "ON" : "OFF"}
            valueColor={position.ignition ? "#10B981" : "#EF4444"}
          />
          <StatItem
            label="GPS"
            value={position.gps_fixed ? "Fixed" : "LBS"}
            valueColor={position.gps_fixed ? "#10B981" : "#F59E0B"}
          />
        </View>
      )}
    </View>
  )
}

function StatItem({
  label,
  value,
  valueColor = "#111827",
}: {
  label: string
  value: string
  valueColor?: string
}) {
  return (
    <View style={styles.statItem}>
      <Text style={styles.statLabel}>{label}</Text>
      <Text style={[styles.statValue, { color: valueColor }]}>{value}</Text>
    </View>
  )
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#0F172A" },

  header: {
    backgroundColor: "#1E293B",
    paddingTop: Platform.OS === "ios" ? 50 : 16,
    paddingBottom: 16,
    paddingHorizontal: 20,
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  headerTitle: { color: "#F1F5F9", fontSize: 18, fontWeight: "700" },
  headerSub: { color: "#94A3B8", fontSize: 12, marginTop: 2 },

  statusBadge: {
    paddingHorizontal: 10,
    paddingVertical: 4,
    borderRadius: 20,
  },
  statusOnline: { backgroundColor: "#10B981" },
  statusOffline: { backgroundColor: "#6B7280" },
  statusText: { color: "#FFF", fontSize: 11, fontWeight: "700" },

  map: { flex: 1 },

  followBtn: {
    position: "absolute",
    bottom: 200,
    right: 16,
    backgroundColor: "#3B82F6",
    borderRadius: 24,
    paddingHorizontal: 16,
    paddingVertical: 10,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.3,
    shadowRadius: 4,
    elevation: 4,
  },
  followBtnText: { color: "#FFF", fontWeight: "600", fontSize: 13 },

  statsPanel: {
    backgroundColor: "#1E293B",
    flexDirection: "row",
    paddingVertical: 16,
    paddingHorizontal: 20,
    borderTopWidth: 1,
    borderTopColor: "#334155",
  },
  statItem: { flex: 1, alignItems: "center" },
  statLabel: { color: "#64748B", fontSize: 11, marginBottom: 4 },
  statValue: { fontSize: 16, fontWeight: "700" },
})
