import React, { useState, useEffect } from 'react';
import securityService from '../services/security';
import { BlockedDatabase } from '../types/server';

const BlockedDatabases: React.FC = () => {
  const [blocked, setBlocked] = useState<BlockedDatabase[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadBlocked();
  }, []);

  const loadBlocked = async () => {
    try {
      setLoading(true);
      const data = await securityService.getBlockedDatabases();
      setBlocked(data.items || []);
    } catch {
      setBlocked([]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Blocked Databases</h1>
          <p className="text-slate-400 mt-1">Databases blocked from access</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Block Database
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading blocked databases...</div>
        ) : blocked.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No blocked databases</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Reason</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Blocked At</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Expires</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {blocked.map((db) => (
                <tr key={db.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{db.name}</td>
                  <td className="p-4 text-slate-300">{db.reason}</td>
                  <td className="p-4 text-slate-300">{new Date(db.blocked_at).toLocaleDateString()}</td>
                  <td className="p-4 text-slate-300">{db.expires_at ? new Date(db.expires_at).toLocaleDateString() : 'Never'}</td>
                  <td className="p-4">
                    <button className="text-amber-400 hover:text-amber-300 text-sm">Unblock</button>
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

export default BlockedDatabases;
