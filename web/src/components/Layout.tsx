import { Outlet, NavLink, useNavigate } from 'react-router-dom';
import {
  LayoutDashboard, Database, HardDrive,
  ScrollText, Settings, LogOut, Box, Camera, Shield,
} from 'lucide-react';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/resources', label: 'Resources', icon: Box },
  { to: '/databases', label: 'Databases', icon: Database },
  { to: '/hardware', label: 'Hardware', icon: HardDrive },
  { to: '/snapshots', label: 'Snapshots', icon: Camera },
  { to: '/backups', label: 'Backups', icon: Shield },
  { to: '/audit', label: 'Audit Logs', icon: ScrollText },
  { to: '/settings', label: 'Settings', icon: Settings },
];

export default function Layout() {
  const navigate = useNavigate();

  function logout() {
    localStorage.removeItem('auth_token');
    localStorage.removeItem('nest_tenant');
    navigate('/login');
  }

  return (
    <div className="flex h-screen bg-[#0f172a]">
      {/* Sidebar */}
      <aside className="w-64 bg-[#1e293b] flex flex-col border-r border-[#334155]">
        <div className="p-6 border-b border-[#334155]">
          <img src="/nest-logo.png" alt="Nest" className="h-10 w-auto" />
        </div>
        <nav className="flex-1 p-4 space-y-1">
          {navItems.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
                  isActive
                    ? 'bg-[#0ea5e9]/20 text-[#0ea5e9]'
                    : 'text-slate-400 hover:text-[#fbbf24] hover:bg-[#334155]'
                }`
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="p-4 border-t border-[#334155]">
          <button
            onClick={logout}
            className="flex items-center gap-3 px-3 py-2 w-full rounded-lg text-sm text-slate-400 hover:text-red-400 hover:bg-[#334155] transition-colors"
          >
            <LogOut size={16} />
            Logout
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto p-8">
        <Outlet />
      </main>
    </div>
  );
}
