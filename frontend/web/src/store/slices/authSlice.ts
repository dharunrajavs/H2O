import { createSlice, PayloadAction } from "@reduxjs/toolkit"
import type { User } from "@/types"

interface AuthState {
  user: User | null
  accessToken: string | null
  refreshToken: string | null
  isAuthenticated: boolean
}

const initialState: AuthState = {
  user: null,
  accessToken: typeof window !== "undefined" ? localStorage.getItem("access_token") : null,
  refreshToken: typeof window !== "undefined" ? localStorage.getItem("refresh_token") : null,
  isAuthenticated: typeof window !== "undefined" ? !!localStorage.getItem("access_token") : false,
}

const authSlice = createSlice({
  name: "auth",
  initialState,
  reducers: {
    setCredentials(
      state,
      action: PayloadAction<{ user: User; accessToken: string; refreshToken: string }>
    ) {
      state.user = action.payload.user
      state.accessToken = action.payload.accessToken
      state.refreshToken = action.payload.refreshToken
      state.isAuthenticated = true

      localStorage.setItem("access_token", action.payload.accessToken)
      localStorage.setItem("refresh_token", action.payload.refreshToken)
    },

    updateAccessToken(state, action: PayloadAction<string>) {
      state.accessToken = action.payload
      localStorage.setItem("access_token", action.payload)
    },

    logout(state) {
      state.user = null
      state.accessToken = null
      state.refreshToken = null
      state.isAuthenticated = false
      localStorage.clear()
    },
  },
})

export const { setCredentials, updateAccessToken, logout } = authSlice.actions
export default authSlice.reducer
