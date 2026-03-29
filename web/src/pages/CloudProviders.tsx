import React, { useState, useEffect } from 'react';
import cloudService from '../services/cloud';
import { CloudProvider } from '../types/server';

const CloudProviders: React.FC = () => {
  const [providers, setProviders] = useState<CloudProvider[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadProviders();
  }, []);

  const loadProviders = async () => {
    try {
      setLoading(true);
      const data = await cloudService.getProviders();
      setProviders(data.items || []);
    } catch {
      setProviders([]);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'connected': return 'bg-emerald-500/20 text-emerald-400';
      case 'disconnected': return 'bg-slate-500/20 text-slate-400';
      case 'error': return 'bg-red-500/20 text-red-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Cloud Providers</h1>
          <p className="text-slate-400 mt-1">Manage cloud provider connections</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Add Provider
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading cloud providers...</div>
        ) : providers.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No cloud providers configured</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Type</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Region</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Status</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {providers.map((provider) => (
                <tr key={provider.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{provider.name}</td>
                  <td className="p-4 text-slate-300">{provider.provider_type}</td>
                  <td className="p-4 text-slate-300">{provider.region}</td>
                  <td className="p-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${statusColor(provider.status)}`}>
                      {provider.status}
                    </span>
                  </td>
                  <td className="p-4">
                    <button className="text-sky-400 hover:text-sky-300 mr-3 text-sm">Test Connection</button>
                    <button className="text-red-400 hover:text-red-300 text-sm">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};

export default CloudProviders;
