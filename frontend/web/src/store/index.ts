import { configureStore } from "@reduxjs/toolkit"
import authReducer from "./slices/authSlice"
import fleetReducer from "./slices/fleetSlice"

export const store = configureStore({
  reducer: {
    auth: authReducer,
    fleet: fleetReducer,
  },
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware({
      serializableCheck: {
        ignoredActionPaths: ["payload.recorded_at", "payload.updated_at"],
      },
    }),
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
