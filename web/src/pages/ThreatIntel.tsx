import React, { useState, useEffect } from 'react';
import securityService from '../services/security';
import { ThreatIntelEntry } from '../types/server';

const ThreatIntel: React.FC = () => {
  const [entries, setEntries] = useState<ThreatIntelEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadEntries();
  }, []);

  const loadEntries = async () => {
    try {
      setLoading(true);
      const data = await securityService.getThreatIntel();
      setEntries(data.items || []);
    } catch {
      setEntries([]);
    } finally {
      setLoading(false);
    }
  };

  const severityColor = (severity: string) => {
    switch (severity) {
      case 'critical': return 'bg-red-500/20 text-red-400';
      case 'high': return 'bg-orange-500/20 text-orange-400';
      case 'medium': return 'bg-amber-500/20 text-amber-400';
      case 'low': return 'bg-slate-500/20 text-slate-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-amber-400">Threat Intelligence</h1>
        <p className="text-slate-400 mt-1">Monitor threat indicators and alerts</p>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading threat intelligence...</div>
        ) : entries.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No threat intelligence entries</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Source</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Type</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Indicator</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Severity</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">First Seen</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Active</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{entry.source}</td>
                  <td className="p-4 text-slate-300">{entry.threat_type}</td>
                  <td className="p-4 text-slate-300 font-mono text-sm">{entry.indicator}</td>
                  <td className="p-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${severityColor(entry.severity)}`}>
                      {entry.severity}
                    </span>
                  </td>
                  <td className="p-4 text-slate-300">{new Date(entry.first_seen).toLocaleDateString()}</td>
                  <td className="p-4">
                    <span className={`inline-block w-2.5 h-2.5 rounded-full ${entry.active ? 'bg-emerald-400' : 'bg-slate-500'}`} />
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

export default ThreatIntel;
