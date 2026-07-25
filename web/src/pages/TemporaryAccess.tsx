import React, { useState, useEffect } from 'react';
import accessService from '../services/access';
import { TemporaryAccess as TemporaryAccessType } from '../types/server';

const TemporaryAccess: React.FC = () => {
  const [grants, setGrants] = useState<TemporaryAccessType[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadGrants();
  }, []);

  const loadGrants = async () => {
    try {
      setLoading(true);
      const data = await accessService.getTemporaryAccess();
      setGrants(data.items || []);
    } catch {
      setGrants([]);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'active': return 'bg-emerald-500/20 text-emerald-400';
      case 'expired': return 'bg-slate-500/20 text-slate-400';
      case 'revoked': return 'bg-red-500/20 text-red-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Temporary Access</h1>
          <p className="text-slate-400 mt-1">Time-limited database access grants</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Grant Access
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading access grants...</div>
        ) : grants.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No access grants found</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">User</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Database</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Permission</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Reason</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Granted</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Expires</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Status</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {grants.map((grant) => (
                <tr key={grant.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{grant.user_email}</td>
                  <td className="p-4 text-slate-300">{grant.database_name}</td>
                  <td className="p-4 text-slate-300">{grant.permission_level}</td>
                  <td className="p-4 text-slate-300">{grant.reason}</td>
                  <td className="p-4 text-slate-300">{new Date(grant.granted_at).toLocaleDateString()}</td>
                  <td className="p-4 text-slate-300">{new Date(grant.expires_at).toLocaleDateString()}</td>
                  <td className="p-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${statusColor(grant.status)}`}>
                      {grant.status}
                    </span>
                  </td>
                  <td className="p-4">
                    {grant.status === 'active' && (
                      <button className="text-red-400 hover:text-red-300 text-sm">Revoke</button>
                    )}
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

export default TemporaryAccess;
