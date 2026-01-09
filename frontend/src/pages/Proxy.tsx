import { useEffect, useState } from 'react';
import { toast } from 'react-toastify';
import { api } from '../api';
import type { Proxy, Device } from '../types';
import { Network, Plus, X, Loader2, RefreshCw } from 'lucide-react';

export default function ProxyPage() {
  const [proxies, setProxies] = useState<Proxy[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [deviceId, setDeviceId] = useState('');
  const [localPort, setLocalPort] = useState('');
  const [remoteHost, setRemoteHost] = useState('');
  const [remotePort, setRemotePort] = useState('');

  useEffect(() => {
    loadProxies();
    loadDevices();
  }, []);

  const loadDevices = async () => {
    try {
      const data = await api.getDevices();
      setDevices(data);
    } catch (err) {
      toast.error(`Failed to load devices: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const loadProxies = async () => {
    try {
      setLoading(true);
      const data = await api.listProxies();
      setProxies(data);
    } catch (err) {
      toast.error(`Failed to load proxies: ${err instanceof Error ? err.message : 'Unknown error'}`);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!deviceId || !localPort || !remoteHost || !remotePort) return;
    try {
      const localPortNum = parseInt(localPort);
      const remotePortNum = parseInt(remotePort);
      if (isNaN(localPortNum) || isNaN(remotePortNum)) {
        toast.warn('Ports must be valid numbers');
        return;
      }
      await api.createProxy(deviceId, localPortNum, remoteHost, remotePortNum);
      setDeviceId('');
      setLocalPort('');
      setRemoteHost('');
      setRemotePort('');
      setShowCreate(false);
      toast.success('Proxy created successfully');
      await loadProxies();
    } catch (err) {
      toast.error(`Failed to create proxy: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const handleClose = async (proxyPort: number) => {
    try {
      await api.closeProxy(proxyPort);
      toast.success('Proxy closed successfully');
      await loadProxies();
    } catch (err) {
      toast.error(`Failed to close proxy: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Proxy Management</h1>
          <p className="mt-2 text-gray-600">Manage network proxies</p>
        </div>
        <div className="flex space-x-2">
          <button
            onClick={loadProxies}
            className="flex items-center px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200"
          >
            <RefreshCw className="w-4 h-4 mr-2" />
            Refresh
          </button>
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="flex items-center px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700"
          >
            <Plus className="w-4 h-4 mr-2" />
            Create Proxy
          </button>
        </div>
      </div>

      {/* Create Proxy Form */}
      {showCreate && (
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">Create New Proxy</h2>
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Device (required)
              </label>
              <select
                value={deviceId}
                onChange={(e) => setDeviceId(e.target.value)}
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
              >
                <option value="">Select a device...</option>
                {devices.map((device) => (
                  <option key={device.ID} value={device.ID} title={device.ID}>
                    {device.ID}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Local Port (required)
              </label>
              <input
                type="number"
                value={localPort}
                onChange={(e) => setLocalPort(e.target.value)}
                placeholder="e.g., 8080"
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Remote Host (required)
              </label>
              <input
                type="text"
                value={remoteHost}
                onChange={(e) => setRemoteHost(e.target.value)}
                placeholder="e.g., 127.0.0.1"
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Remote Port (required)
              </label>
              <input
                type="number"
                value={remotePort}
                onChange={(e) => setRemotePort(e.target.value)}
                placeholder="e.g., 3000"
                className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
              />
            </div>
            <div className="flex space-x-2">
              <button
                onClick={handleCreate}
                disabled={!deviceId || !localPort || !remoteHost || !remotePort}
                className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
              >
                Create
              </button>
              <button
                onClick={() => {
                  setShowCreate(false);
                  setDeviceId('');
                  setLocalPort('');
                  setRemoteHost('');
                  setRemotePort('');
                }}
                className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Proxies List */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200">
        <div className="p-6 border-b border-gray-200">
          <h2 className="text-lg font-semibold text-gray-900">
            Active Proxies ({proxies.length})
          </h2>
        </div>
        {loading ? (
          <div className="p-6 text-center">
            <Loader2 className="w-6 h-6 animate-spin text-primary-500 mx-auto" />
          </div>
        ) : proxies.length === 0 ? (
          <div className="p-6 text-center text-gray-500">No active proxies</div>
        ) : (
          <div className="divide-y divide-gray-200">
            {proxies.map((proxy) => (
              <div key={proxy.local_port} className="p-6">
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-3">
                    <div className="p-2 bg-primary-50 rounded-lg">
                      <Network className="w-5 h-5 text-primary-600" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-gray-900">
                        Port {proxy.local_port} → {proxy.remote_addr}
                      </p>
                      <p className="text-sm text-gray-500 font-mono truncate" title={proxy.device_id}>
                        Device: {proxy.device_id}
                      </p>
                    </div>
                  </div>
                  <button
                    onClick={() => handleClose(proxy.local_port)}
                    className="flex items-center px-3 py-1 text-sm bg-red-50 text-red-600 rounded hover:bg-red-100"
                  >
                    <X className="w-4 h-4 mr-1" />
                    Close
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
