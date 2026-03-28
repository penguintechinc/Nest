/**
 * Application sidebar component
 * Primary navigation menu with categorized items
 */

import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import {
  LayoutDashboard, Server, Database, FileCode, Shield,
  AlertTriangle, Ban, Cloud, TrendingUp, Clock, Users, HardDrive,
} from 'lucide-react';

interface NavItem {
  path: string;
  label: string;
  icon: React.ElementType;
}

interface NavCategory {
  header: string;
  items: NavItem[];
}

interface SidebarProps {
  isOpen?: boolean;
  onClose?: () => void;
}

const navCategories: NavCategory[] = [
  {
    header: 'Overview',
    items: [
      { path: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
      { path: '/resources', label: 'Resources', icon: HardDrive },
      { path: '/teams', label: 'Teams', icon: Users },
    ],
  },
  {
    header: 'Data Management',
    items: [
      { path: '/servers', label: 'Servers', icon: Server },
      { path: '/databases', label: 'Databases', icon: Database },
      { path: '/sql-files', label: 'SQL Files', icon: FileCode },
    ],
  },
  {
    header: 'Security',
    items: [
      { path: '/security-rules', label: 'Security Rules', icon: Shield },
      { path: '/threat-intel', label: 'Threat Intel', icon: AlertTriangle },
      { path: '/blocked-databases', label: 'Blocked DBs', icon: Ban },
    ],
  },
  {
    header: 'Infrastructure',
    items: [
      { path: '/cloud-providers', label: 'Cloud Providers', icon: Cloud },
      { path: '/scaling', label: 'Scaling', icon: TrendingUp },
      { path: '/temporary-access', label: 'Temp Access', icon: Clock },
    ],
  },
];

const Sidebar: React.FC<SidebarProps> = ({ isOpen = true, onClose }) => {
  const location = useLocation();

  const isActive = (path: string) => location.pathname === path;

  return (
    <>
      {/* Mobile overlay */}
      {isOpen && (
        <div
          className="fixed inset-0 bg-black/50 md:hidden z-30"
          onClick={onClose}
        ></div>
      )}

      {/* Sidebar */}
      <nav
        className={`fixed md:relative w-64 h-screen bg-gray-900 text-white transition-transform duration-300 ease-in-out z-40 overflow-y-auto ${
          isOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'
        }`}
      >
        <div className="p-6">
          <div className="flex items-center justify-between mb-6">
            <span className="text-2xl font-bold text-amber-400">NEST</span>
            <button
              onClick={onClose}
              className="md:hidden text-gray-400 hover:text-white"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>

          <div className="space-y-6">
            {navCategories.map((category) => (
              <div key={category.header}>
                <p className="text-xs uppercase tracking-wider text-slate-500 font-semibold mb-2 px-4">
                  {category.header}
                </p>
                <ul className="space-y-1">
                  {category.items.map((item) => {
                    const Icon = item.icon;
                    return (
                      <li key={item.path}>
                        <Link
                          to={item.path}
                          onClick={onClose}
                          className={`flex items-center space-x-3 px-4 py-2.5 rounded-lg transition ${
                            isActive(item.path)
                              ? 'bg-sky-600 text-white'
                              : 'text-slate-300 hover:bg-slate-800'
                          }`}
                        >
                          <Icon className="w-5 h-5" />
                          <span className="text-sm">{item.label}</span>
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </div>
            ))}
          </div>
        </div>

        {/* Footer */}
        <div className="absolute bottom-0 left-0 right-0 p-6 border-t border-gray-800">
          <div className="text-xs text-gray-400">
            <p className="mb-1">v1.0.0</p>
            <p>&copy; 2024 NEST</p>
          </div>
        </div>
      </nav>
    </>
  );
};

export default Sidebar;
