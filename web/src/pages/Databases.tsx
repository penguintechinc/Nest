import { useQuery } from '@tanstack/react-query';
import axios from 'axios';
import { RefreshCw } from 'lucide-react';

export default function Databases() {
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const token = localStorage.getItem('nest_token') ?? '';
  const headers = { Authorization: `Bearer ${token}` };

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['databases', tenant],
    queryFn: () => axios.get(`/api/v1/tenants/${tenant}/databases`, { headers }).then(r => r.data),
    enabled: !!tenant,
  });

  const databases = data?.databases ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-[#fbbf24]">Databases</h2>
        <button onClick={() => refetch()} className="p-2 rounded-lg bg-[#1e293b] border border-[#334155] text-slate-400 hover:text-slate-100"><RefreshCw size={16} /></button>
      </div>
      {isLoading ? <p className="text-slate-400">Loading...</p> : (
        databases.length === 0 ? (
          <div className="bg-[#1e293b] rounded-xl p-12 border border-[#334155] text-center"><p className="text-slate-400">No databases provisioned yet.</p></div>
        ) : (
          <div className="bg-[#1e293b] rounded-xl border border-[#334155] overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-[#334155]/30"><tr className="text-left text-slate-400">{['Name','Type','Class','Status','Endpoint'].map(h=><th key={h} className="px-6 py-3 font-medium">{h}</th>)}</tr></thead>
              <tbody>
                {databases.map((db: any) => (
                  <tr key={db.name} className="border-t border-[#334155]/50">
                    <td className="px-6 py-4 font-mono text-slate-100">{db.name}</td>
                    <td className="px-6 py-4 text-slate-400">{db.type}</td>
                    <td className="px-6 py-4 text-slate-400">{db.class}</td>
                    <td className="px-6 py-4"><span className="px-2 py-1 rounded bg-emerald-900/50 text-emerald-400 text-xs">{db.status ?? 'active'}</span></td>
                    <td className="px-6 py-4 font-mono text-slate-500 text-xs">{db.endpoint ?? '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      )}
    </div>
  );
}
