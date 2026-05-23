import { useState, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';

export default function Login() {
  const navigate = useNavigate();
  const [tenant, setTenant] = useState('');
  const [token, setToken] = useState('');
  const [error, setError] = useState('');

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!tenant.trim() || !token.trim()) {
      setError('Tenant and token are required');
      return;
    }
    localStorage.setItem('nest_token', token.trim());
    localStorage.setItem('nest_tenant', tenant.trim());
    navigate('/dashboard');
  }

  return (
    <div className="min-h-screen bg-[#0f172a] flex items-center justify-center">
      <div className="bg-[#1e293b] rounded-xl p-8 w-full max-w-md border border-[#334155]">
        <h1 className="text-2xl font-bold text-[#fbbf24] mb-2">Nest Admin</h1>
        <p className="text-slate-400 text-sm mb-8">Sign in to manage your storage platform</p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="tenant" className="block text-sm font-medium text-slate-300 mb-1">Tenant</label>
            <input
              id="tenant"
              type="text"
              value={tenant}
              onChange={e => setTenant(e.target.value)}
              placeholder="acme"
              className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]"
            />
          </div>
          <div>
            <label htmlFor="token" className="block text-sm font-medium text-slate-300 mb-1">API Token</label>
            <input
              id="token"
              type="password"
              value={token}
              onChange={e => setToken(e.target.value)}
              placeholder="sub:tenant:scope"
              className="w-full bg-[#0f172a] border border-[#334155] rounded-lg px-4 py-2 text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-[#0ea5e9]"
            />
          </div>
          {error && <p className="text-red-400 text-sm">{error}</p>}
          <button
            type="submit"
            className="w-full bg-[#0ea5e9] hover:bg-[#0284c7] text-white font-semibold py-2 rounded-lg transition-colors"
          >
            Sign In
          </button>
        </form>
      </div>
    </div>
  );
}
