/**
 * Resources page
 * Full resource management interface
 */

import React, { useState } from 'react';
import ResourceList from '../components/resources/ResourceList';
import ResourceWizard from '../components/resources/ResourceWizard';
import { Resource } from '../types/resource';

const Resources: React.FC = () => {
  const [showWizard, setShowWizard] = useState(false);
  const [selectedResource, setSelectedResource] = useState<Resource | null>(null);

  const handleResourceCreated = () => {
    setShowWizard(false);
    // Optionally refresh the list here
  };

  if (showWizard) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Create Resource</h1>
          <p className="text-slate-400 mt-2">Add a new cloud resource to NEST</p>
        </div>
        <ResourceWizard onSuccess={handleResourceCreated} onCancel={() => setShowWizard(false)} />
      </div>
    );
  }

  if (selectedResource) {
    return (
      <div className="space-y-6">
        <button
          onClick={() => setSelectedResource(null)}
          className="text-sky-400 hover:text-sky-300 font-medium"
        >
          ← Back to Resources
        </button>

        <div>
          <h1 className="text-3xl font-bold text-amber-400">{selectedResource.name}</h1>
          <p className="text-slate-400 mt-2">{selectedResource.type}</p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Resource Details */}
          <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
            <h2 className="text-lg font-semibold text-amber-400 mb-4">Details</h2>
            <dl className="space-y-4">
              <div>
                <dt className="text-sm font-medium text-slate-400">Status</dt>
                <dd className="text-lg text-gray-200 capitalize">{selectedResource.status}</dd>
              </div>
              <div>
                <dt className="text-sm font-medium text-slate-400">Cloud Provider</dt>
                <dd className="text-lg text-gray-200">{selectedResource.cloud_provider}</dd>
              </div>
              {selectedResource.region && (
                <div>
                  <dt className="text-sm font-medium text-slate-400">Region</dt>
                  <dd className="text-lg text-gray-200">{selectedResource.region}</dd>
                </div>
              )}
              <div>
                <dt className="text-sm font-medium text-slate-400">Created</dt>
                <dd className="text-lg text-gray-200">
                  {new Date(selectedResource.created_at).toLocaleDateString()}
                </dd>
              </div>
            </dl>
          </div>

          {/* Actions */}
          <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
            <h2 className="text-lg font-semibold text-amber-400 mb-4">Actions</h2>
            <div className="space-y-2">
              <button className="w-full px-4 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium">
                Edit Resource
              </button>
              <button className="w-full px-4 py-2 bg-slate-700 text-gray-200 rounded-lg hover:bg-slate-600 transition font-medium">
                View Metrics
              </button>
              <button className="w-full px-4 py-2 bg-slate-700 text-gray-200 rounded-lg hover:bg-slate-600 transition font-medium">
                View Logs
              </button>
              <button className="w-full px-4 py-2 bg-red-900/50 text-red-400 rounded-lg hover:bg-red-900/70 transition font-medium">
                Delete Resource
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-amber-400">Resources</h1>
          <p className="text-slate-400 mt-2">Manage your cloud resources</p>
        </div>
        <button
          onClick={() => setShowWizard(true)}
          className="px-6 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition font-medium"
        >
          + Create Resource
        </button>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-lg p-6">
        <ResourceList onSelectResource={setSelectedResource} />
      </div>
    </div>
  );
};

export default Resources;
