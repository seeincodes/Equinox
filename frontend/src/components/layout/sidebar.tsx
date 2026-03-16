import { NavLink } from 'react-router'
import { useAuth } from '../../hooks/use-auth'

const navItems = [
  { to: '/', label: 'Dashboard', icon: '◈' },
  { to: '/decisions', label: 'Decisions', icon: '◎' },
]

export function Sidebar() {
  const { user } = useAuth()

  return (
    <aside className="flex h-screen w-56 flex-col border-r border-gray-200 bg-white">
      <div className="flex items-center gap-2 border-b border-gray-200 px-4 py-4">
        <span className="text-lg font-bold text-blue-600">Equinox</span>
      </div>
      <nav className="flex-1 space-y-1 px-2 py-3">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? 'bg-blue-50 text-blue-700'
                  : 'text-gray-700 hover:bg-gray-100'
              }`
            }
          >
            <span>{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
        {user?.role === 'admin' && (
          <NavLink
            to="/settings"
            className={({ isActive }) =>
              `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? 'bg-blue-50 text-blue-700'
                  : 'text-gray-700 hover:bg-gray-100'
              }`
            }
          >
            <span>⚙</span>
            Settings
          </NavLink>
        )}
      </nav>
    </aside>
  )
}
