import React, { useState, useEffect } from 'react';
import serverService from '../services/servers';
import { DatabaseServer } from '../types/server';

const Servers: React.FC = () => {
  const [servers, setServers] = useState<DatabaseServer[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadServers();
  }, []);

  const loadServers = async () => {
    try {
      setLoading(true);
      const data = await serverService.getServers();
      setServers(data.items || []);
    } catch {
      setServers([]);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'active': return 'bg-emerald-500/20 text-emerald-400';
      case 'maintenance': return 'bg-amber-500/20 text-amber-400';
      case 'error': return 'bg-red-500/20 text-red-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Database Servers</h1>
          <p className="text-slate-400 mt-1">Manage database server instances</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Add Server
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading servers...</div>
        ) : servers.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No servers configured</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Host</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Type</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Status</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Connections</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {servers.map((server) => (
                <tr key={server.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{server.name}</td>
                  <td className="p-4 text-slate-300">{server.hostname}:{server.port}</td>
                  <td className="p-4 text-slate-300">{server.server_type} {server.version}</td>
                  <td className="p-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${statusColor(server.status)}`}>
                      {server.status}
                    </span>
                  </td>
                  <td className="p-4 text-slate-300">{server.current_connections}/{server.max_connections}</td>
                  <td className="p-4">
                    <button className="text-sky-400 hover:text-sky-300 mr-3 text-sm">Edit</button>
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

export default Servers;
