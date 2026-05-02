import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import axios from 'axios';
import { Search } from 'lucide-react';

export default function AuditLogs() {
  const token = localStorage.getItem('nest_token') ?? '';
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const headers = { Authorization: `Bearer ${token}` };
  const [search, setSearch] = useState('');

  const { data, isLoading } = useQuery({
    queryKey: ['audit', tenant],
    queryFn: () => axios.get(`/api/v1/audit/events?tenant=${tenant}&limit=50`, { headers }).then(r => r.data),
    enabled: !!tenant,
  });

  const events = (data?.events ?? []).filter((e: any) =>
    !search || e.action?.includes(search) || e.resource?.includes(search) || e.actor?.includes(search)
  );

  return (
    <div>
      <h2 className="text-2xl font-bold text-[#fbbf24] mb-6">Audit Logs</h2>
      <div className="relative mb-6">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
        <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search events..." className="w-full bg-[#1e293b] border border-[#334155] rounded-lg pl-10 pr-4 py-2 text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]" />
      </div>
      {isLoading ? <p className="text-slate-400">Loading...</p> : events.length === 0 ? (
        <div className="bg-[#1e293b] rounded-xl p-12 border border-[#334155] text-center"><p className="text-slate-400">No audit events found.</p></div>
      ) : (
        <div className="bg-[#1e293b] rounded-xl border border-[#334155] overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[#334155]/30"><tr className="text-left text-slate-400">{['Time','Actor','Action','Resource','Outcome'].map(h=><th key={h} className="px-6 py-3 font-medium">{h}</th>)}</tr></thead>
            <tbody>
              {events.map((e: any) => (
                <tr key={e.id} className="border-t border-[#334155]/50">
                  <td className="px-6 py-3 text-slate-500 text-xs">{new Date(e.timestamp).toLocaleString()}</td>
                  <td className="px-6 py-3 font-mono text-slate-300 text-xs">{e.actor}</td>
                  <td className="px-6 py-3 text-slate-400">{e.action}</td>
                  <td className="px-6 py-3 font-mono text-slate-400 text-xs">{e.resource}</td>
                  <td className="px-6 py-3"><span className={`px-2 py-1 rounded text-xs ${e.outcome === 'success' ? 'bg-emerald-900/50 text-emerald-400' : 'bg-red-900/50 text-red-400'}`}>{e.outcome}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
