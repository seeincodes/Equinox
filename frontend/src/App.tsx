import { BrowserRouter, Routes, Route, Navigate } from 'react-router'
import { AuthProvider, useAuth } from './hooks/use-auth'
import { AppLayout } from './components/layout/app-layout'
import { LoginPage } from './pages/login'
import { DashboardPage } from './pages/dashboard'
import { GroupDetailPage } from './pages/group-detail'
import { DecisionsPage } from './pages/decisions'
import { SettingsPage } from './pages/settings'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()
  if (loading)
    return (
      <div className="flex h-screen items-center justify-center text-sm text-gray-500">
        Loading...
      </div>
    )
  if (!user) return <Navigate to="/login" />
  return <AppLayout>{children}</AppLayout>
}

export function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <DashboardPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/groups/:id"
            element={
              <ProtectedRoute>
                <GroupDetailPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/decisions"
            element={
              <ProtectedRoute>
                <DecisionsPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/settings"
            element={
              <ProtectedRoute>
                <SettingsPage />
              </ProtectedRoute>
            }
          />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}
