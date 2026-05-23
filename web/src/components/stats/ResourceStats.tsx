/**
 * Resource statistics component
 * Displays summary statistics about resources
 */

import React, { useState, useEffect } from 'react';
import resourceService from '../../services/resources';
import { lifecycleColors } from '../../theme/lifecycleColors';

const ResourceStats: React.FC = () => {
  const [stats, setStats] = useState<{
    total: number;
    by_lifecycle: Record<string, number>;
    by_provider: Record<string, number>;
    by_status: Record<string, number>;
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchStats();
  }, []);

  const fetchStats = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await resourceService.getResourceStats();
      setStats(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch statistics');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return <div className="animate-pulse h-32 bg-gray-200 rounded-lg"></div>;
  }

  if (error || !stats) {
    return (
      <div className="text-center py-8">
        <p className="text-red-600">{error || 'Failed to load statistics'}</p>
        <button
          onClick={fetchStats}
          className="mt-2 text-blue-600 hover:underline"
        >
          Retry
        </button>
      </div>
    );
  }

  const lifecycleEntries = Object.entries(stats.by_lifecycle || {});
  const providerEntries = Object.entries(stats.by_provider || {});
  const statusEntries = Object.entries(stats.by_status || {});

  return (
    <div className="space-y-6">
      {/* Total Resources Card */}
      <div className="bg-gradient-to-br from-blue-500 to-blue-600 rounded-lg shadow-md p-6 text-white">
        <h3 className="text-sm font-medium opacity-90">Total Resources</h3>
        <p className="text-4xl font-bold mt-2">{stats.total}</p>
      </div>

      {/* Lifecycle Distribution */}
      {lifecycleEntries.length > 0 && (
        <div className="bg-white rounded-lg shadow-md p-6">
          <h3 className="text-lg font-semibold mb-4 text-gray-900">By Lifecycle</h3>
          <div className="space-y-3">
            {lifecycleEntries.map(([lifecycle, count]) => {
              const color =
                lifecycleColors[lifecycle as keyof typeof lifecycleColors];
              return (
                <div key={lifecycle}>
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-sm font-medium text-gray-700">
                      {color?.label || lifecycle}
                    </span>
                    <span className="text-sm font-semibold text-gray-900">{count}</span>
                  </div>
                  <div className="w-full bg-gray-200 rounded-full h-2">
                    <div
                      className="h-2 rounded-full transition-all duration-300"
                      style={{
                        width: `${(count / stats.total) * 100}%`,
                        backgroundColor: color?.primary || '#666',
                      }}
                    ></div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Provider Distribution */}
      {providerEntries.length > 0 && (
        <div className="bg-white rounded-lg shadow-md p-6">
          <h3 className="text-lg font-semibold mb-4 text-gray-900">By Cloud Provider</h3>
          <div className="space-y-2">
            {providerEntries.map(([provider, count]) => (
              <div key={provider} className="flex items-center justify-between py-2">
                <span className="text-sm text-gray-700 capitalize">{provider}</span>
                <span className="text-sm font-semibold text-gray-900">{count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Status Distribution */}
      {statusEntries.length > 0 && (
        <div className="bg-white rounded-lg shadow-md p-6">
          <h3 className="text-lg font-semibold mb-4 text-gray-900">By Status</h3>
          <div className="space-y-2">
            {statusEntries.map(([status, count]) => {
              const statusColorMap: Record<string, string> = {
                active: 'text-green-600 bg-green-50',
                inactive: 'text-gray-600 bg-gray-50',
                error: 'text-red-600 bg-red-50',
              };
              const statusColor = statusColorMap[status] ?? 'text-gray-600 bg-gray-50';

              return (
                <div
                  key={status}
                  className="flex items-center justify-between py-2 px-3 rounded-lg"
                >
                  <span className={`text-sm font-medium capitalize ${statusColor}`}>
                    {status}
                  </span>
                  <span className="text-sm font-semibold text-gray-900">{count}</span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default ResourceStats;
