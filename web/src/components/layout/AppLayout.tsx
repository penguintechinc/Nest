/**
 * Main application layout wrapper
 * Combines header, sidebar, and main content
 */

import React, { useState } from 'react';
import Header from './Header';
import Sidebar from './Sidebar';

interface AppLayoutProps {
  children: React.ReactNode;
}

const AppLayout: React.FC<AppLayoutProps> = ({ children }) => {
  const [sidebarOpen, setSidebarOpen] = useState(false);

  return (
    <div className="flex h-screen bg-slate-900 text-gray-200">
      <Sidebar isOpen={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      <div className="flex-1 flex flex-col overflow-hidden">
        <Header onMenuToggle={() => setSidebarOpen(!sidebarOpen)} />

        <main className="flex-1 overflow-auto">
          <div className="max-w-7xl mx-auto py-6 sm:px-6 md:px-8">
            {children}
          </div>
        </main>
      </div>
    </div>
  );
};

export default AppLayout;
