import { useAuth } from '../../hooks/use-auth'
import { useNavigate } from 'react-router'

const roleBadgeColor: Record<string, string> = {
  admin: 'bg-purple-100 text-purple-800',
  analyst: 'bg-blue-100 text-blue-800',
  viewer: 'bg-gray-100 text-gray-600',
}

export function Topbar() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  return (
    <header className="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-3">
      <div />
      <div className="flex items-center gap-3">
        <span className="text-sm text-gray-600">{user?.email}</span>
        <span
          className={`rounded-full px-2 py-0.5 text-xs font-medium ${roleBadgeColor[user?.role ?? 'viewer']}`}
        >
          {user?.role}
        </span>
        <button
          onClick={handleLogout}
          className="rounded-md px-3 py-1.5 text-sm text-gray-500 hover:bg-gray-100 hover:text-gray-700"
        >
          Logout
        </button>
      </div>
    </header>
  )
}
