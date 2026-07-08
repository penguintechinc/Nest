import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Plus, Trash2, RefreshCw, Shield, Info, RotateCcw } from 'lucide-react';
import api from '../services/api';

interface DataProtectionPolicy {
  name: string;
  snapshotSchedule?: string;
  backupSchedule?: string;
  destination: string;
  lastSnapshot?: string;
  lastBackup?: string;
}

interface DataResource {
  name: string;
  type: string;
}

export default function Backups() {
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const qc = useQueryClient();

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({
    name: '',
    snapshotSchedule: 'none',
    backupSchedule: 'none',
    destination: '',
  });
  const [restoreConfirm, setRestoreConfirm] = useState<string | null>(null);
  const [restoreSuccess, setRestoreSuccess] = useState(false);

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['protection-policies', tenant],
    queryFn: () => api.get(`/tenants/${tenant}/protection-policies`).then(r => r.data),
    enabled: !!tenant,
  });

  const { data: resourceData } = useQuery({
    queryKey: ['dataresources', tenant, 'object'],
    queryFn: () => api.get(`/tenants/${tenant}/dataresources?type=object`).then(r => r.data),
    enabled: !!tenant,
  });

  const createMutation = useMutation({
    mutationFn: (body: object) => api.post(`/tenants/${tenant}/protection-policies`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['protection-policies'] });
      setShowCreate(false);
      setForm({ name: '', snapshotSchedule: 'none', backupSchedule: 'none', destination: '' });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.delete(`/tenants/${tenant}/protection-policies/${name}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['protection-policies'] }),
  });

  const restoreMutation = useMutation({
    mutationFn: ({ backupName, policyName }: { backupName: string; policyName: string }) =>
      api.post(
        `/tenants/${tenant}/data-resources/${policyName}/restore`,
        { backup_name: backupName },
      ),
    onSuccess: () => {
      setRestoreSuccess(true);
      setRestoreConfirm(null);
      setTimeout(() => setRestoreSuccess(false), 3000);
    },
  });

  const policies: DataProtectionPolicy[] = data?.policies ?? [];
  const objectResources: DataResource[] = resourceData?.dataresources ?? [];

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return '—';
    try {
      return new Date(dateStr).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return dateStr;
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-[#fbbf24]">Backups</h2>
        <div className="flex gap-3">
          <button onClick={() => refetch()} className="p-2 rounded-lg bg-[#1e293b] border border-[#334155] text-slate-400 hover:text-slate-100"><RefreshCw size={16} /></button>
          <button onClick={() => setShowCreate(true)} className="flex items-center gap-2 px-4 py-2 bg-[#0ea5e9] hover:bg-[#0284c7] text-white rounded-lg text-sm font-medium">
            <Plus size={16} /> Create
          </button>
        </div>
      </div>

      <div className="mb-6 bg-[#334155]/30 rounded-lg p-4 border border-[#334155] flex gap-3">
        <Info size={16} className="text-[#0ea5e9] flex-shrink-0 mt-0.5" />
        <p className="text-sm text-slate-300">
          Backups are stored in your S3-compatible object DataResource via Velero. Snapshots are local VolumeSnapshots stored in Ceph.
        </p>
      </div>

      {isLoading ? (
        <p className="text-slate-400">Loading...</p>
      ) : policies.length === 0 ? (
        <div className="bg-[#1e293b] rounded-xl p-12 border border-[#334155] text-center">
          <p className="text-slate-400">No backup policies found. Create your first one.</p>
        </div>
      ) : (
        <div className="bg-[#1e293b] rounded-xl border border-[#334155] overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[#334155]/30">
              <tr className="text-left text-slate-400">
                {['Name', 'Snapshot Schedule', 'Backup Schedule', 'Destination', 'Last Snapshot', 'Last Backup', ''].map(h => (
                  <th key={h} className="px-6 py-3 font-medium">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {policies.map((p: DataProtectionPolicy) => (
                <tr key={p.name} className="border-t border-[#334155]/50 hover:bg-[#334155]/20">
                  <td className="px-6 py-4 font-mono text-slate-100">{p.name}</td>
                  <td className="px-6 py-4 text-slate-400">{p.snapshotSchedule ?? '—'}</td>
                  <td className="px-6 py-4 text-slate-400">{p.backupSchedule ?? '—'}</td>
                  <td className="px-6 py-4 text-slate-400">{p.destination}</td>
                  <td className="px-6 py-4 text-slate-400 text-xs">{formatDate(p.lastSnapshot)}</td>
                  <td className="px-6 py-4 text-slate-400 text-xs">{formatDate(p.lastBackup)}</td>
                  <td className="px-6 py-4 flex gap-2">
                    <button onClick={() => setRestoreConfirm(p.name)} className="p-1 text-slate-500 hover:text-amber-400" title="Restore"><RotateCcw size={14} /></button>
                    <button onClick={() => deleteMutation.mutate(p.name)} className="p-1 text-slate-500 hover:text-red-400" title="Delete"><Trash2 size={14} /></button>
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
            <h3 className="text-lg font-bold text-slate-100 mb-6">Create Backup Policy</h3>
            <form onSubmit={e => { e.preventDefault(); createMutation.mutate({ ...form, tenant }); }} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-400 mb-1">Policy Name</label>
                <input
                  value={form.name}
                  onChange={e => setForm(f => ({...f, name: e.target.value}))}
                  placeholder="e.g., daily-policy"
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]"
                />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Snapshot Schedule</label>
                <select
                  value={form.snapshotSchedule}
                  onChange={e => setForm(f => ({...f, snapshotSchedule: e.target.value}))}
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none"
                >
                  <option value="none">None</option>
                  <option value="@hourly">Hourly</option>
                  <option value="@daily">Daily</option>
                  <option value="@weekly">Weekly</option>
                </select>
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Backup Schedule</label>
                <select
                  value={form.backupSchedule}
                  onChange={e => setForm(f => ({...f, backupSchedule: e.target.value}))}
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none"
                >
                  <option value="none">None</option>
                  <option value="@daily">Daily</option>
                  <option value="@weekly">Weekly</option>
                  <option value="@monthly">Monthly</option>
                </select>
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Destination</label>
                <select
                  value={form.destination}
                  onChange={e => setForm(f => ({...f, destination: e.target.value}))}
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none"
                >
                  <option value="">Select destination</option>
                  {objectResources.map(res => (
                    <option key={res.name} value={res.name}>{res.name}</option>
                  ))}
                </select>
              </div>
              <div className="flex gap-3 pt-2">
                <button type="button" onClick={() => setShowCreate(false)} className="flex-1 py-2 rounded-lg border border-[#334155] text-slate-400 hover:text-slate-100">Cancel</button>
                <button type="submit" className="flex-1 py-2 rounded-lg bg-[#0ea5e9] text-white font-medium hover:bg-[#0284c7]">Create</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {restoreConfirm && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-[#1e293b] rounded-xl p-8 w-full max-w-md border border-[#334155]">
            <h3 className="text-lg font-bold text-slate-100 mb-4">Restore Backup</h3>
            <p className="text-slate-300 mb-6">
              Restore from backup <span className="font-mono text-amber-400">{restoreConfirm}</span>? This will restore resources to namespace <span className="font-mono text-amber-400">default</span>.
            </p>
            <div className="flex gap-3">
              <button
                onClick={() => setRestoreConfirm(null)}
                className="flex-1 py-2 rounded-lg border border-[#334155] text-slate-400 hover:text-slate-100"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  restoreMutation.mutate({ backupName: restoreConfirm, policyName: restoreConfirm });
                }}
                disabled={restoreMutation.isPending}
                className="flex-1 py-2 rounded-lg bg-amber-600 text-white font-medium hover:bg-amber-700 disabled:opacity-50"
              >
                {restoreMutation.isPending ? 'Restoring...' : 'Restore'}
              </button>
            </div>
          </div>
        </div>
      )}

      {restoreSuccess && (
        <div className="fixed bottom-4 right-4 bg-emerald-900/80 text-emerald-400 px-6 py-3 rounded-lg border border-emerald-700 z-50">
          Restore initiated.
        </div>
      )}
    </div>
  );
}
