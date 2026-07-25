import { useQuery } from '@tanstack/react-query';
import api from '../services/api';

interface HardwareNode {
  name: string;
  class: string;
}

export default function Hardware() {
  const { data, isLoading } = useQuery({
    queryKey: ['hardware'],
    queryFn: () => api.get('/hardware/inventory').then(r => r.data),
  });

  return (
    <div>
      <h2 className="text-2xl font-bold text-[#fbbf24] mb-6">Hardware</h2>
      {isLoading ? <p className="text-slate-400">Loading...</p> : (
        <div className="grid gap-4">
          {(data?.nodes ?? []).length === 0 ? (
            <div className="bg-[#1e293b] rounded-xl p-12 border border-[#334155] text-center"><p className="text-slate-400">No hardware inventory data available.</p></div>
          ) : (data?.nodes as HardwareNode[] ?? []).map((n) => (
            <div key={n.name} className="bg-[#1e293b] rounded-xl p-6 border border-[#334155]">
              <div className="flex items-center justify-between">
                <h3 className="font-mono text-slate-100">{n.name}</h3>
                <span className="text-xs px-2 py-1 rounded bg-slate-700 text-slate-300">{n.class}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
