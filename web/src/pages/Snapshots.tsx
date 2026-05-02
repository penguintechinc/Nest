import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import axios from 'axios';
import { Plus, Trash2, RefreshCw, Camera, RotateCcw } from 'lucide-react';

interface VolumeSnapshot {
  name: string;
  sourcePVC: string;
  snapshotClass: string;
  readyToUse: boolean;
  creationTime: string;
  size?: string;
}

interface DataResource {
  name: string;
  type: string;
}

export default function Snapshots() {
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const token = localStorage.getItem('nest_token') ?? '';
  const headers = { Authorization: `Bearer ${token}` };
  const qc = useQueryClient();

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({ name: '', sourcePVC: '', snapshotClass: 'nest-rbd-snapshot' });
  const [restoreConfirm, setRestoreConfirm] = useState<string | null>(null);
  const [restoreSuccess, setRestoreSuccess] = useState(false);

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['snapshots', tenant],
    queryFn: () => axios.get(`/api/v1/tenants/${tenant}/snapshots`, { headers }).then(r => r.data),
    enabled: !!tenant,
  });

  const { data: pvcData } = useQuery({
    queryKey: ['dataresources', tenant],
    queryFn: () => axios.get(`/api/v1/tenants/${tenant}/dataresources`, { headers }).then(r => r.data),
    enabled: !!tenant,
  });

  const createMutation = useMutation({
    mutationFn: (body: object) => axios.post(`/api/v1/tenants/${tenant}/snapshots`, body, { headers }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['snapshots'] });
      setShowCreate(false);
      setForm({ name: '', sourcePVC: '', snapshotClass: 'nest-rbd-snapshot' });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => axios.delete(`/api/v1/tenants/${tenant}/snapshots/${name}`, { headers }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['snapshots'] }),
  });

  const restoreMutation = useMutation({
    mutationFn: ({ snapshotName, dataResourceName }: { snapshotName: string; dataResourceName: string }) =>
      axios.post(
        `/api/v1/tenants/${tenant}/data-resources/${dataResourceName}/restore`,
        { snapshot_name: snapshotName },
        { headers },
      ),
    onSuccess: () => {
      setRestoreSuccess(true);
      setRestoreConfirm(null);
      setTimeout(() => setRestoreSuccess(false), 3000);
    },
  });

  const snapshots: VolumeSnapshot[] = data?.snapshots ?? [];
  const pvcs: DataResource[] = pvcData?.dataresources ?? [];

  const getStatusColor = (readyToUse: boolean) => {
    return readyToUse
      ? 'bg-emerald-900/50 text-emerald-400'
      : 'bg-amber-900/50 text-amber-400';
  };

  const getStatusLabel = (readyToUse: boolean) => {
    return readyToUse ? 'Ready' : 'Pending';
  };

  const formatDate = (dateStr: string) => {
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
        <h2 className="text-2xl font-bold text-[#fbbf24]">Snapshots</h2>
        <div className="flex gap-3">
          <button onClick={() => refetch()} className="p-2 rounded-lg bg-[#1e293b] border border-[#334155] text-slate-400 hover:text-slate-100"><RefreshCw size={16} /></button>
          <button onClick={() => setShowCreate(true)} className="flex items-center gap-2 px-4 py-2 bg-[#0ea5e9] hover:bg-[#0284c7] text-white rounded-lg text-sm font-medium">
            <Plus size={16} /> Create
          </button>
        </div>
      </div>

      {isLoading ? (
        <p className="text-slate-400">Loading...</p>
      ) : snapshots.length === 0 ? (
        <div className="bg-[#1e293b] rounded-xl p-12 border border-[#334155] text-center">
          <p className="text-slate-400">No snapshots found. Create your first one.</p>
        </div>
      ) : (
        <div className="bg-[#1e293b] rounded-xl border border-[#334155] overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[#334155]/30">
              <tr className="text-left text-slate-400">
                {['Name', 'Source PVC', 'Status', 'Creation Time', 'Size', ''].map(h => (
                  <th key={h} className="px-6 py-3 font-medium">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {snapshots.map((s: VolumeSnapshot) => (
                <tr key={s.name} className="border-t border-[#334155]/50 hover:bg-[#334155]/20">
                  <td className="px-6 py-4 font-mono text-slate-100">{s.name}</td>
                  <td className="px-6 py-4 text-slate-400">{s.sourcePVC}</td>
                  <td className="px-6 py-4"><span className={`px-2 py-1 rounded text-xs ${getStatusColor(s.readyToUse)}`}>{getStatusLabel(s.readyToUse)}</span></td>
                  <td className="px-6 py-4 text-slate-400 text-xs">{formatDate(s.creationTime)}</td>
                  <td className="px-6 py-4 text-slate-500">{s.size ?? '—'}</td>
                  <td className="px-6 py-4 flex gap-2">
                    <button onClick={() => setRestoreConfirm(s.name)} className="p-1 text-slate-500 hover:text-amber-400" title="Restore"><RotateCcw size={14} /></button>
                    <button onClick={() => deleteMutation.mutate(s.name)} className="p-1 text-slate-500 hover:text-red-400" title="Delete"><Trash2 size={14} /></button>
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
            <h3 className="text-lg font-bold text-slate-100 mb-6">Create Snapshot</h3>
            <form onSubmit={e => { e.preventDefault(); createMutation.mutate({ ...form, tenant }); }} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-400 mb-1">Snapshot Name</label>
                <input
                  value={form.name}
                  onChange={e => setForm(f => ({...f, name: e.target.value}))}
                  placeholder="e.g., snapshot-1"
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]"
                />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Source PVC</label>
                <select
                  value={form.sourcePVC}
                  onChange={e => setForm(f => ({...f, sourcePVC: e.target.value}))}
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 focus:outline-none"
                >
                  <option value="">Select a PVC</option>
                  {pvcs.map(pvc => (
                    <option key={pvc.name} value={pvc.name}>{pvc.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">Snapshot Class</label>
                <input
                  value={form.snapshotClass}
                  readOnly
                  className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-500 focus:outline-none"
                />
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
            <h3 className="text-lg font-bold text-slate-100 mb-4">Restore Snapshot</h3>
            <p className="text-slate-300 mb-6">
              Restore from snapshot <span className="font-mono text-amber-400">{restoreConfirm}</span>? This will create a new PVC.
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
                  const snapshot = snapshots.find(s => s.name === restoreConfirm);
                  if (snapshot) {
                    const dataResourceName = snapshot.sourcePVC;
                    restoreMutation.mutate({ snapshotName: restoreConfirm, dataResourceName });
                  }
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
          Restore initiated. A new PVC will appear shortly.
        </div>
      )}
    </div>
  );
}
