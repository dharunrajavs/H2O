'use client'

import { useSelector } from 'react-redux'
import { RootState } from '@/store'
import { LoginPage } from '@/pages/LoginPage'
import { DashboardPage } from '@/pages/DashboardPage'

export default function Home() {
  const token = useSelector((s: RootState) => s.auth.accessToken)
  return token ? <DashboardPage /> : <LoginPage />
}
