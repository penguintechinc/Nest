import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Plus, Trash2, RefreshCw } from 'lucide-react';
import api from '../services/api';

export default function Resources() {
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const qc = useQueryClient();

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({ name: '', type: 'postgres', class: 'postgres-standard' });
  const [typeFilter, setTypeFilter] = useState('');

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['dataresources', tenant, typeFilter],
    queryFn: () => api.get(`/tenants/${tenant}/dataresources${typeFilter ? `?type=${typeFilter}` : ''}`).then(r => r.data),
    enabled: !!tenant,
  });

  const createMutation = useMutation({
    mutationFn: (body: object) => api.post(`/tenants/${tenant}/dataresources`, body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['dataresources'] }); setShowCreate(false); },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.delete(`/tenants/${tenant}/dataresources/${name}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['dataresources'] }),
  });

  const resources = data?.dataresources ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-[#fbbf24]">Data Resources</h2>
        <div className="flex gap-3">
          <button onClick={() => refetch()} className="p-2 rounded-lg bg-[#1e293b] border border-[#334155] text-slate-400 hover:text-slate-100"><RefreshCw size={16} /></button>
          <button onClick={() => setShowCreate(true)} className="flex items-center gap-2 px-4 py-2 bg-[#0ea5e9] hover:bg-[#0284c7] text-white rounded-lg text-sm font-medium">
            <Plus size={16} /> Create
          </button>
        </div>
      </div>

      <div className="flex gap-3 mb-6">
        {['', 'postgres', 'mysql', 'kafka', 's3', 'valkey'].map(t => (
          <button key={t} onClick={() => setTypeFilter(t)}
            className={`px-3 py-1 rounded-full text-xs font-medium transition-colors ${typeFilter === t ? 'bg-[#0ea5e9] text-white' : 'bg-[#1e293b] text-slate-400 hover:text-slate-100 border border-[#334155]'}`}>
            {t || 'All'}
          </button>
        ))}
      </div>

      {isLoading ? (
        <p className="text-slate-400">Loading...</p>
      ) : resources.length === 0 ? (
        <div className="bg-[#1e293b] rounded-xl p-12 border border-[#334155] text-center">
          <p className="text-slate-400">No resources found. Create your first one.</p>
        </div>
      ) : (
        <div className="bg-[#1e293b] rounded-xl border border-[#334155] overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[#334155]/30">
              <tr className="text-left text-slate-400">
                {['Name', 'Type', 'Class', 'Status', 'Endpoint', ''].map(h => (
                  <th key={h} className="px-6 py-3 font-medium">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {resources.map((r: any) => (
                <tr key={r.name} className="border-t border-[#334155]/50 hover:bg-[#334155]/20">
                  <td className="px-6 py-4 font-mono text-slate-100">{r.name}</td>
                  <td className="px-6 py-4 text-slate-400">{r.type}</td>
                  <td className="px-6 py-4 text-slate-400">{r.class}</td>
                  <td className="px-6 py-4"><span className="px-2 py-1 rounded bg-emerald-900/50 text-emerald-400 text-xs">{r.status ?? 'active'}</span></td>
                  <td className="px-6 py-4 font-mono text-slate-500 text-xs">{r.endpoint ?? '—'}</td>
                  <td className="px-6 py-4">
                    <button onClick={() => deleteMutation.mutate(r.name)} className="p-1 text-slate-500 hover:text-red-400"><Trash2 size={14} /></button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showCreate && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-[#1e293b] rounded-xl p-8 w-full max-w-md border border-[#334155]">
            <h3 className="text-lg font-bold text-slate-100 mb-6">Create Data Resource</h3>
            <form onSubmit={e => { e.preventDefault(); createMutation.mutate({ ...form, tenant }); }} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-400 mb-1">Name</label>
                <input value={form.name} onChange={e => setForm(f => ({...f, name: e.target.value}))} className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Type</label>
                <select value={form.type} onChange={e => setForm(f => ({...f, type: e.target.value}))} className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none">
                  {['postgres', 'mysql', 'mariadb', 'valkey', 'kafka', 's3', 'pvc/block'].map(t => <option key={t}>{t}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Class</label>
                <input value={form.class} onChange={e => setForm(f => ({...f, class: e.target.value}))} className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]" />
              </div>
              <div className="flex gap-3 pt-2">
                <button type="button" onClick={() => setShowCreate(false)} className="flex-1 py-2 rounded-lg border border-[#334155] text-slate-400 hover:text-slate-100">Cancel</button>
                <button type="submit" className="flex-1 py-2 rounded-lg bg-[#0ea5e9] text-white font-medium hover:bg-[#0284c7]">Create</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
