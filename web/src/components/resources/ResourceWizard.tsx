/**
 * Resource creation wizard component
 * Multi-step form for creating new resources
 */

import React, { useState } from 'react';
import { CreateResourceInput, LifecycleMode } from '../../types/resource';
import { lifecycleColors } from '../../theme/lifecycleColors';
import resourceService from '../../services/resources';

interface ResourceWizardProps {
  onSuccess?: () => void;
  onCancel?: () => void;
}

const ResourceWizard: React.FC<ResourceWizardProps> = ({ onSuccess, onCancel }) => {
  const [step, setStep] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [formData, setFormData] = useState<CreateResourceInput>({
    name: '',
    type: '',
    lifecycle_mode: LifecycleMode.FULL,
    cloud_provider: '',
    region: '',
    tags: {},
  });

  const handleInputChange = (field: keyof CreateResourceInput, value: CreateResourceInput[typeof field]) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleSubmit = async () => {
    if (!formData.name || !formData.type || !formData.cloud_provider) {
      setError('Please fill in all required fields');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await resourceService.createResource(formData);
      setStep(1);
      setFormData({
        name: '',
        type: '',
        lifecycle_mode: LifecycleMode.FULL,
        cloud_provider: '',
        region: '',
        tags: {},
      });
      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create resource');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto bg-white rounded-lg shadow-md p-8">
      <h2 className="text-2xl font-bold mb-8">Create New Resource</h2>

      {/* Progress indicator */}
      <div className="flex items-center justify-between mb-8">
        {[1, 2, 3].map((s) => (
          <div key={s} className="flex items-center">
            <div
              className={`w-8 h-8 rounded-full flex items-center justify-center font-medium ${
                step >= s ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600'
              }`}
            >
              {s}
            </div>
            {s < 3 && (
              <div
                className={`flex-1 h-1 mx-2 ${step > s ? 'bg-blue-600' : 'bg-gray-200'}`}
              ></div>
            )}
          </div>
        ))}
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg text-red-700">
          {error}
        </div>
      )}

      {/* Step 1: Basic Information */}
      {step === 1 && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold mb-4">Basic Information</h3>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Resource Name *
            </label>
            <input
              type="text"
              value={formData.name}
              onChange={(e) => handleInputChange('name', e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
              placeholder="e.g., production-db-01"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Resource Type *
            </label>
            <select
              value={formData.type}
              onChange={(e) => handleInputChange('type', e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
            >
              <option value="">Select a type...</option>
              <option value="database">Database</option>
              <option value="compute">Compute Instance</option>
              <option value="storage">Storage Bucket</option>
              <option value="network">Network</option>
              <option value="container">Container Registry</option>
              <option value="other">Other</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Cloud Provider *
            </label>
            <select
              value={formData.cloud_provider}
              onChange={(e) => handleInputChange('cloud_provider', e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
            >
              <option value="">Select a provider...</option>
              <option value="aws">Amazon Web Services</option>
              <option value="gcp">Google Cloud Platform</option>
              <option value="azure">Microsoft Azure</option>
              <option value="other">Other</option>
            </select>
          </div>
        </div>
      )}

      {/* Step 2: Lifecycle Configuration */}
      {step === 2 && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold mb-4">Lifecycle Configuration</h3>

          <label className="block text-sm font-medium text-gray-700 mb-4">
            Select how NEST manages this resource
          </label>

          <div className="space-y-3">
            {Object.entries(lifecycleColors).map(([key, color]) => (
              <label
                key={key}
                className="flex items-start p-4 border-2 rounded-lg cursor-pointer transition"
                style={{
                  borderColor:
                    formData.lifecycle_mode === key ? color.primary : '#e5e7eb',
                  backgroundColor: formData.lifecycle_mode === key ? color.light : 'white',
                }}
              >
                <input
                  type="radio"
                  name="lifecycle"
                  value={key}
                  checked={formData.lifecycle_mode === key}
                  onChange={(e) =>
                    handleInputChange('lifecycle_mode', e.target.value as LifecycleMode)
                  }
                  className="mt-1 mr-4"
                />
                <div>
                  <p className="font-semibold" style={{ color: color.primary }}>
                    {color.label}
                  </p>
                  <p className="text-sm text-gray-600 mt-1">{color.description}</p>
                </div>
              </label>
            ))}
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Region (Optional)
            </label>
            <input
              type="text"
              value={formData.region || ''}
              onChange={(e) => handleInputChange('region', e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500"
              placeholder="e.g., us-east-1"
            />
          </div>
        </div>
      )}

      {/* Step 3: Review & Submit */}
      {step === 3 && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold mb-4">Review</h3>

          <div className="bg-gray-50 rounded-lg p-4 space-y-3">
            <div>
              <p className="text-sm text-gray-600">Name</p>
              <p className="font-medium">{formData.name}</p>
            </div>
            <div>
              <p className="text-sm text-gray-600">Type</p>
              <p className="font-medium">{formData.type}</p>
            </div>
            <div>
              <p className="text-sm text-gray-600">Cloud Provider</p>
              <p className="font-medium">{formData.cloud_provider}</p>
            </div>
            <div>
              <p className="text-sm text-gray-600">Lifecycle</p>
              <p className="font-medium">{lifecycleColors[formData.lifecycle_mode].label}</p>
            </div>
            {formData.region && (
              <div>
                <p className="text-sm text-gray-600">Region</p>
                <p className="font-medium">{formData.region}</p>
              </div>
            )}
          </div>

          <button
            onClick={handleSubmit}
            disabled={loading}
            className="w-full py-2 px-4 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:bg-gray-400 font-medium"
          >
            {loading ? 'Creating...' : 'Create Resource'}
          </button>
        </div>
      )}

      {/* Navigation */}
      <div className="flex justify-between mt-8 pt-6 border-t border-gray-200">
        <button
          onClick={onCancel}
          className="px-4 py-2 text-gray-700 border border-gray-300 rounded-md hover:bg-gray-50"
        >
          Cancel
        </button>

        <div className="flex space-x-2">
          <button
            onClick={() => setStep(Math.max(1, step - 1))}
            disabled={step === 1}
            className="px-4 py-2 text-gray-700 border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
          >
            Previous
          </button>

          <button
            onClick={() => {
              if (step < 3) {
                setStep(step + 1);
              } else {
                handleSubmit();
              }
            }}
            disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:bg-gray-400 font-medium"
          >
            {step === 3 ? 'Create' : 'Next'}
          </button>
        </div>
      </div>
    </div>
  );
};

export default ResourceWizard;
