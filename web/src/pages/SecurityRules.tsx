import React, { useState, useEffect } from 'react';
import securityService from '../services/security';
import { SecurityRule } from '../types/server';

const SecurityRules: React.FC = () => {
  const [rules, setRules] = useState<SecurityRule[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadRules();
  }, []);

  const loadRules = async () => {
    try {
      setLoading(true);
      const data = await securityService.getSecurityRules();
      setRules(data.items || []);
    } catch {
      setRules([]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Security Rules</h1>
          <p className="text-slate-400 mt-1">Manage firewall and access control rules</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Add Rule
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading security rules...</div>
        ) : rules.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No security rules configured</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Type</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Action</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Priority</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Enabled</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr key={rule.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{rule.name}</td>
                  <td className="p-4 text-slate-300">{rule.rule_type}</td>
                  <td className="p-4 text-slate-300">{rule.action}</td>
                  <td className="p-4 text-slate-300">{rule.priority}</td>
                  <td className="p-4">
                    <span className={`inline-block w-2.5 h-2.5 rounded-full ${rule.enabled ? 'bg-emerald-400' : 'bg-slate-500'}`} />
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

export default SecurityRules;
