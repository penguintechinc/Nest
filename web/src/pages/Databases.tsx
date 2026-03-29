import React, { useState, useEffect } from 'react';
import databaseService from '../services/databases';
import { ManagedDatabase } from '../types/server';

const Databases: React.FC = () => {
  const [databases, setDatabases] = useState<ManagedDatabase[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadDatabases();
  }, []);

  const loadDatabases = async () => {
    try {
      setLoading(true);
      const data = await databaseService.getDatabases();
      setDatabases(data.items || []);
    } catch {
      setDatabases([]);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'active': return 'bg-emerald-500/20 text-emerald-400';
      case 'suspended': return 'bg-amber-500/20 text-amber-400';
      case 'archived': return 'bg-slate-500/20 text-slate-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  const formatSize = (sizeMb: number) => {
    if (sizeMb >= 1024) {
      return `${(sizeMb / 1024).toFixed(1)} GB`;
    }
    return `${sizeMb} MB`;
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Managed Databases</h1>
          <p className="text-slate-400 mt-1">View and manage database instances</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Create Database
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading databases...</div>
        ) : databases.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No databases found</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Server</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Size</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Charset</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Status</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {databases.map((db) => (
                <tr key={db.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{db.name}</td>
                  <td className="p-4 text-slate-300">{db.server_name}</td>
                  <td className="p-4 text-slate-300">{formatSize(db.size_mb)}</td>
                  <td className="p-4 text-slate-300">{db.charset}</td>
                  <td className="p-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${statusColor(db.status)}`}>
                      {db.status}
                    </span>
                  </td>
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

export default Databases;
