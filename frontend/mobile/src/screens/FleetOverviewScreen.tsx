import React, { useEffect, useState } from "react"
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
  Platform,
  RefreshControl,
} from "react-native"
import type { NativeStackNavigationProp } from "@react-navigation/native-stack"

interface Device {
  id: string
  imei: string
  vehicle_reg: string
  speed: number
  ignition: boolean
  lat: number
  lng: number
  last_seen: string
  is_online: boolean
}

interface Props {
  navigation: NativeStackNavigationProp<any>
}

export function FleetOverviewScreen({ navigation }: Props) {
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  const fetchDevices = async () => {
    try {
      // In production: call fleetApi.liveAll() merged with devicesApi.list()
      // Using mock data here for illustration
      await new Promise((r) => setTimeout(r, 500))
      setDevices([
        { id: "1", imei: "868523040000001", vehicle_reg: "KA-01-AB-1234", speed: 45, ignition: true, lat: 12.97, lng: 77.59, last_seen: new Date().toISOString(), is_online: true },
        { id: "2", imei: "868523040000002", vehicle_reg: "KA-01-CD-5678", speed: 0, ignition: false, lat: 12.95, lng: 77.60, last_seen: new Date(Date.now() - 300000).toISOString(), is_online: false },
        { id: "3", imei: "868523040000003", vehicle_reg: "KA-01-EF-9012", speed: 72, ignition: true, lat: 12.98, lng: 77.57, last_seen: new Date().toISOString(), is_online: true },
      ])
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => { fetchDevices() }, [])

  const renderDevice = ({ item }: { item: Device }) => (
    <TouchableOpacity
      style={styles.deviceCard}
      onPress={() => navigation.navigate("Tracking", {
        deviceId: item.id,
        imei: item.imei,
        vehicleReg: item.vehicle_reg,
      })}
    >
      <View style={styles.cardLeft}>
        <View style={[styles.statusDot, item.is_online ? styles.dotOnline : styles.dotOffline]} />
        <View>
          <Text style={styles.regNumber}>{item.vehicle_reg}</Text>
          <Text style={styles.imeiText}>{item.imei}</Text>
        </View>
      </View>

      <View style={styles.cardRight}>
        {item.is_online ? (
          <>
            <Text style={[styles.speedText, { color: item.speed > 60 ? "#EF4444" : "#10B981" }]}>
              {item.speed.toFixed(0)} km/h
            </Text>
            <Text style={[styles.ignitionText, { color: item.ignition ? "#10B981" : "#6B7280" }]}>
              {item.ignition ? "IGN ON" : "IGN OFF"}
            </Text>
          </>
        ) : (
          <Text style={styles.offlineText}>Offline</Text>
        )}
      </View>
    </TouchableOpacity>
  )

  if (loading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#3B82F6" />
      </View>
    )
  }

  const onlineCount = devices.filter((d) => d.is_online).length
  const movingCount = devices.filter((d) => d.speed > 2).length

  return (
    <View style={styles.container}>
      {/* Summary header */}
      <View style={styles.summaryBar}>
        <SummaryPill label="Total" value={devices.length} color="#3B82F6" />
        <SummaryPill label="Online" value={onlineCount} color="#10B981" />
        <SummaryPill label="Moving" value={movingCount} color="#F59E0B" />
        <SummaryPill label="Offline" value={devices.length - onlineCount} color="#6B7280" />
      </View>

      <FlatList
        data={devices}
        keyExtractor={(d) => d.id}
        renderItem={renderDevice}
        contentContainerStyle={styles.listContent}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => { setRefreshing(true); fetchDevices() }}
            tintColor="#3B82F6"
          />
        }
        ItemSeparatorComponent={() => <View style={styles.separator} />}
      />
    </View>
  )
}

function SummaryPill({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <View style={[styles.pill, { borderColor: color }]}>
      <Text style={[styles.pillValue, { color }]}>{value}</Text>
      <Text style={styles.pillLabel}>{label}</Text>
    </View>
  )
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#F8FAFC" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },

  summaryBar: {
    flexDirection: "row",
    justifyContent: "space-around",
    backgroundColor: "#1E293B",
    paddingVertical: 16,
    paddingTop: Platform.OS === "ios" ? 60 : 16,
  },
  pill: {
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 12,
    borderWidth: 1,
    backgroundColor: "#0F172A",
    minWidth: 64,
  },
  pillValue: { fontSize: 22, fontWeight: "800" },
  pillLabel: { color: "#94A3B8", fontSize: 11, marginTop: 2 },

  listContent: { paddingVertical: 8 },
  separator: { height: 1, backgroundColor: "#E2E8F0", marginHorizontal: 16 },

  deviceCard: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    backgroundColor: "#FFF",
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  cardLeft: { flexDirection: "row", alignItems: "center", gap: 12 },
  cardRight: { alignItems: "flex-end" },

  statusDot: { width: 10, height: 10, borderRadius: 5 },
  dotOnline: { backgroundColor: "#10B981" },
  dotOffline: { backgroundColor: "#D1D5DB" },

  regNumber: { fontSize: 15, fontWeight: "700", color: "#111827" },
  imeiText: { fontSize: 11, color: "#9CA3AF", marginTop: 2 },

  speedText: { fontSize: 17, fontWeight: "700" },
  ignitionText: { fontSize: 11, fontWeight: "600", marginTop: 2 },
  offlineText: { fontSize: 13, color: "#9CA3AF" },
})
