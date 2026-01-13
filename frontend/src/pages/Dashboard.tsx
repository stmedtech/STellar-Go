import { useEffect, useState } from 'react';
import { api } from '../api';
import type { NodeInfo, HealthStatus } from '../types';
import { 
  Workflow,
  Network,
  Shield,
  Router,
  CheckCircle,
  XCircle,
  Loader2
} from 'lucide-react';

export default function Dashboard() {
  const [nodeInfo, setNodeInfo] = useState<NodeInfo | null>(null);
  const [health, setHealth] = useState<HealthStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 5000); // Refresh every 5 seconds
    return () => clearInterval(interval);
  }, []);

  const loadData = async () => {
    try {
      setError(null);
      const [healthData, nodeData] = await Promise.all([
        api.getHealth(),
        api.getNodeInfo(),
      ]);
      setHealth(healthData);
      setNodeInfo(nodeData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <div className="flex items-center">
          <XCircle className="w-5 h-5 text-red-600 mr-2" />
          <span className="text-red-800">{error}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">Overview</h1>
        <p className="mt-2 text-gray-600">Stellar Node Dashboard</p>
      </div>

      {/* Status Card */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            {health?.status === 'healthy' ? (
              <CheckCircle className="w-8 h-8 text-green-500" />
            ) : (
              <XCircle className="w-8 h-8 text-red-500" />
            )}
            <div>
              <h3 className="text-lg font-semibold text-gray-900">System Status</h3>
              <p className="text-sm text-gray-600 capitalize">{health?.status || 'Unknown'}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <StatCard
          icon={Shield}
          title="Policy"
          value={nodeInfo?.Policy?.Enable ? 'Enabled' : 'Disabled'}
          subtitle="Security policy"
          color="orange"
        />
        <StatCard
          icon={Router}
          title="Bootstrapper"
          value={nodeInfo?.Bootstrapper ? 'Yes' : 'No'}
          subtitle="Node type"
          color="green"
        />
        <StatCard
          icon={Network}
          title="Addresses"
          value={nodeInfo?.Addresses?.length || 0}
          subtitle="Network addresses"
          color="purple"
        />
        <StatCard
          icon={Workflow}
          title="Devices"
          value={nodeInfo?.DevicesCount || 0}
          subtitle="Connected devices"
          color="blue"
        />
      </div>

      {/* Node Information */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Node Information</h2>
        <dl className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <dt className="text-sm font-medium text-gray-500">Node ID</dt>
            <dd className="mt-1 text-sm text-gray-900 font-mono break-all">
              {nodeInfo?.NodeID || 'N/A'}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-medium text-gray-500">Relay Node</dt>
            <dd className="mt-1 text-sm text-gray-900">
              {nodeInfo?.RelayNode ? 'Yes' : 'No'}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-medium text-gray-500">Reference Token</dt>
            <dd className="mt-1 text-sm text-gray-900 font-mono">
              {nodeInfo?.ReferenceToken || 'N/A'}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-medium text-gray-500">Whitelist Entries</dt>
            <dd className="mt-1 text-sm text-gray-900">
              {nodeInfo?.Policy?.WhiteList?.length || 0}
            </dd>
          </div>
        </dl>

        {nodeInfo?.Addresses && nodeInfo.Addresses.length > 0 && (
          <div className="mt-4">
            <dt className="text-sm font-medium text-gray-500 mb-2">Addresses</dt>
            <dd className="space-y-1">
              {nodeInfo.Addresses.map((addr, idx) => (
                <div key={idx} className="text-sm text-gray-900 font-mono bg-gray-50 p-2 rounded">
                  {addr}
                </div>
              ))}
            </dd>
          </div>
        )}
      </div>
    </div>
  );
}

interface StatCardProps {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  value: string | number;
  subtitle: string;
  color: 'blue' | 'purple' | 'green' | 'orange';
}

function StatCard({ icon: Icon, title, value, subtitle, color }: StatCardProps) {
  const colorClasses = {
    blue: 'bg-blue-50 text-blue-600',
    purple: 'bg-purple-50 text-purple-600',
    green: 'bg-green-50 text-green-600',
    orange: 'bg-orange-50 text-orange-600',
  };

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
      <div className="flex items-center">
        <div className={`p-3 rounded-lg ${colorClasses[color]}`}>
          <Icon className="w-6 h-6" />
        </div>
        <div className="ml-4">
          <p className="text-sm font-medium text-gray-600">{title}</p>
          <p className="text-2xl font-bold text-gray-900">{value}</p>
          <p className="text-xs text-gray-500 mt-1">{subtitle}</p>
        </div>
      </div>
    </div>
  );
}
