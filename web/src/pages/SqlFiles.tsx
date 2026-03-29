import React, { useState, useEffect } from 'react';
import sqlFileService from '../services/sqlFiles';
import { SqlFile } from '../types/server';

const SqlFiles: React.FC = () => {
  const [sqlFiles, setSqlFiles] = useState<SqlFile[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadSqlFiles();
  }, []);

  const loadSqlFiles = async () => {
    try {
      setLoading(true);
      const data = await sqlFileService.getSqlFiles();
      setSqlFiles(data.items || []);
    } catch {
      setSqlFiles([]);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'pending': return 'bg-amber-500/20 text-amber-400';
      case 'approved': return 'bg-sky-500/20 text-sky-400';
      case 'rejected': return 'bg-red-500/20 text-red-400';
      case 'executed': return 'bg-emerald-500/20 text-emerald-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">SQL Files</h1>
          <p className="text-slate-400 mt-1">Review and execute SQL scripts</p>
        </div>
        <button className="px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          Upload SQL
        </button>
      </div>

      <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
        {loading ? (
          <div className="p-8 text-center text-slate-400">Loading SQL files...</div>
        ) : sqlFiles.length === 0 ? (
          <div className="p-8 text-center text-slate-400">No SQL files found</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="bg-slate-800/50">
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Name</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Description</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Status</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Submitted</th>
                <th className="text-left p-4 text-amber-400/80 text-sm uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody>
              {sqlFiles.map((file) => (
                <tr key={file.id} className="border-t border-slate-700 hover:bg-slate-700/50 transition">
                  <td className="p-4 font-medium text-gray-200">{file.name}</td>
                  <td className="p-4 text-slate-300">{file.description}</td>
                  <td className="p-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${statusColor(file.status)}`}>
                      {file.status}
                    </span>
                  </td>
                  <td className="p-4 text-slate-300">{new Date(file.submitted_at).toLocaleDateString()}</td>
                  <td className="p-4">
                    {file.status === 'pending' && (
                      <button className="text-sky-400 hover:text-sky-300 mr-3 text-sm">Review</button>
                    )}
                    {file.status === 'approved' && (
                      <button className="text-emerald-400 hover:text-emerald-300 mr-3 text-sm">Execute</button>
                    )}
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

export default SqlFiles;
