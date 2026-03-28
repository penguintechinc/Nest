/**
 * Teams page
 * Team management interface
 */

import React from 'react';
import TeamList from '../components/teams/TeamList';
import { Team } from '../types/team';

const Teams: React.FC = () => {
  const [selectedTeam, setSelectedTeam] = React.useState<Team | null>(null);

  if (selectedTeam) {
    return (
      <div className="space-y-6">
        <button
          onClick={() => setSelectedTeam(null)}
          className="text-sky-400 hover:text-sky-300 font-medium"
        >
          ← Back to Teams
        </button>

        <div>
          <h1 className="text-3xl font-bold text-amber-400">{selectedTeam.name}</h1>
          {selectedTeam.description && (
            <p className="text-slate-400 mt-2">{selectedTeam.description}</p>
          )}
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
            <h3 className="text-lg font-semibold text-amber-400 mb-2">Members</h3>
            <p className="text-3xl font-bold text-sky-400">{selectedTeam.members_count}</p>
          </div>

          <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
            <h3 className="text-lg font-semibold text-amber-400 mb-2">Created</h3>
            <p className="text-lg text-gray-200">
              {new Date(selectedTeam.created_at).toLocaleDateString()}
            </p>
          </div>

          <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
            <h3 className="text-lg font-semibold text-amber-400 mb-2">Updated</h3>
            <p className="text-lg text-gray-200">
              {new Date(selectedTeam.updated_at).toLocaleDateString()}
            </p>
          </div>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-amber-400 mb-4">Team Actions</h2>
          <div className="space-y-2">
            <button className="w-full px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
              Manage Members
            </button>
            <button className="w-full px-4 py-2 bg-slate-700 text-gray-200 rounded-lg hover:bg-slate-600 transition font-medium">
              Edit Team
            </button>
            <button className="w-full px-4 py-2 bg-red-900/50 text-red-400 rounded-lg hover:bg-red-900/70 transition font-medium">
              Delete Team
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Teams</h1>
          <p className="text-slate-400 mt-2">Manage team access and permissions</p>
        </div>
        <button className="px-6 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
          + Create Team
        </button>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
        <TeamList onSelectTeam={setSelectedTeam} />
      </div>
    </div>
  );
};

export default Teams;
