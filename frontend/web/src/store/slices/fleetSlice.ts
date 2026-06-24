import { createSlice, PayloadAction } from "@reduxjs/toolkit"
import type { LivePosition, FleetSummary } from "@/types"

interface FleetState {
  positions: Record<string, LivePosition>   // deviceID → latest position
  summary: FleetSummary | null
  selectedDeviceId: string | null
  isLoading: boolean
}

const initialState: FleetState = {
  positions: {},
  summary: null,
  selectedDeviceId: null,
  isLoading: false,
}

const fleetSlice = createSlice({
  name: "fleet",
  initialState,
  reducers: {
    updatePosition(state, action: PayloadAction<{ deviceId: string; position: LivePosition }>) {
      state.positions[action.payload.deviceId] = action.payload.position
    },

    setPositions(state, action: PayloadAction<LivePosition[]>) {
      action.payload.forEach((pos) => {
        // Use IMEI as key for positions map
        state.positions[pos.imei] = pos
      })
    },

    setSummary(state, action: PayloadAction<FleetSummary>) {
      state.summary = action.payload
    },

    selectDevice(state, action: PayloadAction<string | null>) {
      state.selectedDeviceId = action.payload
    },

    setLoading(state, action: PayloadAction<boolean>) {
      state.isLoading = action.payload
    },
  },
})

export const { updatePosition, setPositions, setSummary, selectDevice, setLoading } =
  fleetSlice.actions

export default fleetSlice.reducer
