import { useEffect, useState } from 'react';
import { toast } from 'react-toastify';
import { api } from '../api';
import type { Policy } from '../types';
import { Shield, Plus, X, Loader2, RefreshCw, CheckCircle } from 'lucide-react';

export default function PolicyPage() {
  const [policy, setPolicy] = useState<Policy | null>(null);
  const [whitelist, setWhitelist] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [showAdd, setShowAdd] = useState(false);
  const [newDeviceId, setNewDeviceId] = useState('');

  useEffect(() => {
    loadPolicy();
  }, []);

  const loadPolicy = async () => {
    try {
      setLoading(true);
      const [policyData, whitelistData] = await Promise.all([
        api.getPolicy(),
        api.getWhitelist(),
      ]);
      setPolicy(policyData);
      setWhitelist(whitelistData);
    } catch (err) {
      toast.error(`Failed to load policy: ${err instanceof Error ? err.message : 'Unknown error'}`);
    } finally {
      setLoading(false);
    }
  };

  const handleTogglePolicy = async () => {
    if (!policy) return;
    try {
      await api.setPolicy(!policy.Enable);
      toast.success(`Policy ${!policy.Enable ? 'enabled' : 'disabled'} successfully`);
      await loadPolicy();
    } catch (err) {
      toast.error(`Failed to update policy: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const handleAddToWhitelist = async () => {
    if (!newDeviceId.trim()) return;
    try {
      await api.addToWhitelist(newDeviceId.trim());
      setNewDeviceId('');
      setShowAdd(false);
      toast.success('Device added to whitelist successfully');
      await loadPolicy();
    } catch (err) {
      toast.error(`Failed to add to whitelist: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const handleRemoveFromWhitelist = async (deviceId: string) => {
    try {
      await api.removeFromWhitelist(deviceId);
      toast.success('Device removed from whitelist successfully');
      await loadPolicy();
    } catch (err) {
      toast.error(`Failed to remove from whitelist: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Policy Management</h1>
          <p className="mt-2 text-gray-600">Configure security policies and whitelist</p>
        </div>
        <button
          onClick={loadPolicy}
          className="flex items-center px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200"
        >
          <RefreshCw className="w-4 h-4 mr-2" />
          Refresh
        </button>
      </div>

      {loading ? (
        <div className="flex items-center justify-center h-64">
          <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
        </div>
      ) : (
        <>
          {/* Policy Status */}
          <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-3">
                <div className={`p-3 rounded-lg ${policy?.Enable ? 'bg-green-50' : 'bg-gray-50'}`}>
                  <Shield className={`w-6 h-6 ${policy?.Enable ? 'text-green-600' : 'text-gray-400'}`} />
                </div>
                <div>
                  <h2 className="text-lg font-semibold text-gray-900">Policy Status</h2>
                  <p className="text-sm text-gray-600">
                    {policy?.Enable ? 'Policy is enabled' : 'Policy is disabled'}
                  </p>
                </div>
              </div>
              <label className="relative inline-flex items-center cursor-pointer">
                <input
                  type="checkbox"
                  checked={policy?.Enable || false}
                  onChange={handleTogglePolicy}
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              </label>
            </div>
          </div>

          {/* Whitelist */}
          <div className="bg-white rounded-lg shadow-sm border border-gray-200">
            <div className="p-6 border-b border-gray-200 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-gray-900">
                Whitelist ({whitelist.length})
              </h2>
              <button
                onClick={() => setShowAdd(!showAdd)}
                className="flex items-center px-3 py-1 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700"
              >
                <Plus className="w-4 h-4 mr-1" />
                Add Device
              </button>
            </div>

            {/* Add Device Form */}
            {showAdd && (
              <div className="p-6 border-b border-gray-200 bg-gray-50">
                <div className="flex space-x-2">
                  <input
                    type="text"
                    value={newDeviceId}
                    onChange={(e) => setNewDeviceId(e.target.value)}
                    placeholder="Enter device ID"
                    className="flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                  />
                  <button
                    onClick={handleAddToWhitelist}
                    disabled={!newDeviceId.trim()}
                    className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
                  >
                    Add
                  </button>
                  <button
                    onClick={() => {
                      setShowAdd(false);
                      setNewDeviceId('');
                    }}
                    className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {/* Whitelist Items */}
            {whitelist.length === 0 ? (
              <div className="p-6 text-center text-gray-500">Whitelist is empty</div>
            ) : (
              <div className="divide-y divide-gray-200">
                {whitelist.map((deviceId) => (
                  <div key={deviceId} className="p-6">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-3">
                        <CheckCircle className="w-5 h-5 text-green-500" />
                        <code className="text-sm text-gray-900 font-mono break-all">
                          {deviceId}
                        </code>
                      </div>
                      <button
                        onClick={() => handleRemoveFromWhitelist(deviceId)}
                        className="flex items-center px-3 py-1 text-sm bg-red-50 text-red-600 rounded hover:bg-red-100"
                      >
                        <X className="w-4 h-4 mr-1" />
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
