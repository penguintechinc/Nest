export default function Settings() {
  const tenant = localStorage.getItem('nest_tenant') ?? '';
  const endpoint = window.location.origin;

  return (
    <div>
      <h2 className="text-2xl font-bold text-[#fbbf24] mb-6">Settings</h2>
      <div className="bg-[#1e293b] rounded-xl p-6 border border-[#334155] max-w-lg">
        <h3 className="font-semibold text-slate-100 mb-4">Connection</h3>
        <div className="space-y-3 text-sm">
          <div className="flex justify-between">
            <span className="text-slate-400">Tenant</span>
            <span className="font-mono text-slate-100">{tenant}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-400">API Endpoint</span>
            <span className="font-mono text-slate-100 text-xs">{endpoint}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
