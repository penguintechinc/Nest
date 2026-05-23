import { useQuery } from '@tanstack/react-query';
import axios from 'axios';
import { Database, Box, HardDrive, Activity } from 'lucide-react';

function StatCard({ icon: Icon, label, value, color }: {
  icon: typeof Database; label: string; value: string | number; color: string;
}) {
  return (
    <div className="bg-[#1e293b] rounded-xl p-6 border border-[#334155]">
      <div className="flex items-center gap-4">
        <div className={`p-3 rounded-lg ${color}`}>
          <Icon size={20} className="text-white" />
        </div>
        <div>
          <p className="text-slate-400 text-sm">{label}</p>
          <p className="text-2xl font-bold text-slate-100">{value}</p>
        </div>
      </div>
    </div>
  );
}

export default function Dashboard() {
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const token = localStorage.getItem('nest_token') ?? '';
  const headers = { Authorization: `Bearer ${token}` };

  const { data: resources } = useQuery({
    queryKey: ['resources', tenant],
    queryFn: () => axios.get(`/api/v1/tenants/${tenant}/dataresources`, { headers }).then(r => r.data),
    enabled: !!tenant,
  });

  const { data: databases } = useQuery({
    queryKey: ['databases', tenant],
    queryFn: () => axios.get(`/api/v1/tenants/${tenant}/databases`, { headers }).then(r => r.data),
    enabled: !!tenant,
  });

  return (
    <div>
      <h2 className="text-2xl font-bold text-[#fbbf24] mb-6">Dashboard</h2>
      <p className="text-slate-400 mb-8">Tenant: <span className="text-slate-100 font-mono">{tenant}</span></p>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatCard icon={Box} label="Data Resources" value={resources?.dataresources?.length ?? 0} color="bg-[#0ea5e9]" />
        <StatCard icon={Database} label="Databases" value={databases?.databases?.length ?? 0} color="bg-purple-600" />
        <StatCard icon={HardDrive} label="Storage" value="—" color="bg-emerald-600" />
        <StatCard icon={Activity} label="Status" value="Healthy" color="bg-amber-600" />
      </div>

      <div className="bg-[#1e293b] rounded-xl p-6 border border-[#334155]">
        <h3 className="text-lg font-semibold text-slate-100 mb-4">Recent Resources</h3>
        {resources?.dataresources?.length > 0 ? (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-slate-400 border-b border-[#334155]">
                <th className="pb-3">Name</th><th className="pb-3">Type</th><th className="pb-3">Status</th>
              </tr>
            </thead>
            <tbody>
              {resources.dataresources.slice(0, 5).map((r: any) => (
                <tr key={r.name} className="border-b border-[#334155]/50">
                  <td className="py-3 font-mono text-slate-100">{r.name}</td>
                  <td className="py-3 text-slate-400">{r.type}</td>
                  <td className="py-3"><span className="px-2 py-1 rounded bg-emerald-900/50 text-emerald-400 text-xs">{r.status ?? 'active'}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p className="text-slate-500 text-sm">No resources yet. Create your first resource.</p>
        )}
      </div>
    </div>
  );
}
