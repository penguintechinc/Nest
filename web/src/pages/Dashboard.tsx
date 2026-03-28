/**
 * Dashboard page
 * Main dashboard with resource overview and statistics
 */

import React from 'react';
import ResourceStats from '../components/stats/ResourceStats';
import ResourceList from '../components/resources/ResourceList';

const Dashboard: React.FC = () => {
  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-amber-400">Dashboard</h1>
        <p className="text-slate-400 mt-2">Welcome to NEST - Cloud Resource Management</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        {/* Statistics Sidebar */}
        <div className="lg:col-span-1">
          <ResourceStats />
        </div>

        {/* Resource List */}
        <div className="lg:col-span-2">
          <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
            <h2 className="text-2xl font-bold text-amber-400 mb-6">Resources</h2>
            <ResourceList compact={true} />
          </div>
        </div>
      </div>

      {/* Quick Links */}
      <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-amber-400 mb-4">Quick Actions</h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
            Create Resource
          </button>
          <button className="px-4 py-2 bg-slate-700 text-sky-400 border border-sky-600 rounded-lg hover:bg-slate-600 transition font-medium">
            View All Resources
          </button>
          <button className="px-4 py-2 bg-slate-700 text-sky-400 border border-sky-600 rounded-lg hover:bg-slate-600 transition font-medium">
            Manage Teams
          </button>
        </div>
      </div>
    </div>
  );
};

export default Dashboard;
