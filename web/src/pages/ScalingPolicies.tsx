import React, { useState, useEffect } from 'react';
import scalingService from '../services/scaling';
import { ScalingPolicy } from '../types/server';

const ScalingPolicies: React.FC = () => {
  const [policies, setPolicies] = useState<ScalingPolicy[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadPolicies();
  }, []);

  const loadPolicies = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await scalingService.getPolicies();
      setPolicies(data.items || []);
    } catch {
      setError('Failed to load scaling policies');
      setPolicies([]);
    } finally {
      setLoading(false);
    }
  };

  const getStatusBadge = (status: string) => {
    const colors: Record<string, string> = {
      active: 'bg-emerald-400/20 text-emerald-400',
      inactive: 'bg-slate-500/20 text-slate-400',
      scaling: 'bg-sky-400/20 text-sky-400',
      error: 'bg-red-400/20 text-red-400',
    };
    const colorClass = colors[status] || colors.inactive;
    return (
      <span className={`px-2 py-1 rounded-full text-xs font-medium ${colorClass}`}>
        {status}
      </span>
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Scaling Policies</h1>
          <p className="text-slate-400 mt-1">Auto-scaling configuration</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Create Policy
        </button>
      </div>

      {error && (
        <div className="bg-red-400/10 border border-red-400/30 rounded-lg p-4 text-red-400">
          {error}
        </div>
      )}

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">
            <div className="inline-block w-6 h-6 border-2 border-slate-400 border-t-transparent rounded-full animate-spin mb-2" />
            <p>Loading scaling policies...</p>
          </div>
        ) : policies.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No scaling policies configured</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Type</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Thresholds</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Scale By</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Status</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {policies.map((policy) => (
                <tr key={policy.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{policy.name}</td>
                  <td className="p-4 text-slate-300">{policy.metric}</td>
                  <td className="p-4 text-slate-300">
                    <span className="text-sky-400">{policy.threshold_down}</span>
                    {' / '}
                    <span className="text-emerald-400">{policy.threshold_up}</span>
                  </td>
                  <td className="p-4 text-slate-300">
                    +{policy.scale_up_by} / -{policy.scale_down_by}
                  </td>
                  <td className="p-4">{getStatusBadge(policy.enabled ? 'active' : 'inactive')}</td>
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

export default ScalingPolicies;
