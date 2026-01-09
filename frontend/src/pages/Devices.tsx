import { useEffect, useState, useCallback } from 'react';
import { toast } from 'react-toastify';
import { api } from '../api';
import type { Device, DeviceInfo, FileEntry } from '../types';
import { Server, RefreshCw, Loader2, Plus, X, File, Upload, Download, ArrowLeft, FolderOpen, ChevronDown, ChevronUp } from 'lucide-react';

// ============================================================================
// Utility Functions
// ============================================================================

const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
};

// ============================================================================
// Connect Device Modal Component
// ============================================================================

interface ConnectDeviceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnect: (peerInfo: string) => Promise<void>;
  connecting: boolean;
}

function ConnectDeviceModal({ isOpen, onClose, onConnect, connecting }: ConnectDeviceModalProps) {
  const [peerInfo, setPeerInfo] = useState('');

  if (!isOpen) return null;

  const handleSubmit = async () => {
    if (!peerInfo.trim()) {
      toast.error('Peer info is required');
      return;
    }
    await onConnect(peerInfo.trim());
    setPeerInfo('');
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl p-6 w-full max-w-md">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-semibold text-gray-900">Connect to Device</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Peer Address
            </label>
            <input
              type="text"
              value={peerInfo}
              onChange={(e) => setPeerInfo(e.target.value)}
              placeholder="e.g., /ip4/127.0.0.1/tcp/1234/p2p/Qm..."
              className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm"
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !connecting) {
                  handleSubmit();
                }
              }}
            />
            <p className="mt-2 text-xs text-gray-500">
              Enter the peer multiaddr (e.g., /ip4/127.0.0.1/tcp/1234/p2p/Qm...)
            </p>
          </div>
          <div className="flex space-x-2 pt-2">
            <button
              onClick={handleSubmit}
              disabled={!peerInfo.trim() || connecting}
              className="flex-1 flex items-center justify-center px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
            >
              {connecting ? (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Connecting...
                </>
              ) : (
                <>
                  <Plus className="w-4 h-4 mr-2" />
                  Connect
                </>
              )}
            </button>
            <button
              onClick={() => {
                onClose();
                setPeerInfo('');
              }}
              className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ============================================================================
// Device List Component
// ============================================================================

interface DeviceListProps {
  devices: Device[];
  selectedDeviceId: string | null;
  onDeviceSelect: (deviceId: string) => void;
  onPing: (deviceId: string) => void;
}

function DeviceList({ devices, selectedDeviceId, onDeviceSelect, onPing }: DeviceListProps) {
  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200">
      <div className="p-6 border-b border-gray-200">
        <h2 className="text-lg font-semibold text-gray-900">
          Connected Devices ({devices.length})
        </h2>
      </div>
      <div className="divide-y divide-gray-200">
        {devices.length === 0 ? (
          <div className="p-6 text-center text-gray-500">
            No devices connected
          </div>
        ) : (
          devices.map((device) => (
            <div
              key={device.ID}
              className={`p-4 hover:bg-gray-50 cursor-pointer transition-colors ${
                selectedDeviceId === device.ID ? 'bg-primary-50' : ''
              }`}
              onClick={() => onDeviceSelect(device.ID)}
            >
              <div className="flex items-center">
                <div className="p-2 bg-primary-50 rounded-lg mr-3">
                  <Server className="w-5 h-5 text-primary-600" />
                </div>
                <p
                  className="font-medium text-gray-900 font-mono text-sm truncate grow"
                  style={{ minWidth: 0 }}
                  title={device.ID}
                >
                  {device.ID}
                </p>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onPing(device.ID);
                  }}
                  className="ml-4 px-3 py-1 text-sm bg-gray-100 text-gray-700 rounded hover:bg-gray-200 flex-shrink-0"
                >
                  Ping
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

// ============================================================================
// Device Details Component
// ============================================================================

interface DeviceDetailsProps {
  device: DeviceInfo | null;
}

function DeviceDetails({ device }: DeviceDetailsProps) {
  if (!device) {
    return (
      <div className="bg-white rounded-lg shadow-sm border border-gray-200">
        <div className="p-6 text-center text-gray-500">
          Select a device to view details
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200">
      <div className="p-6 border-b border-gray-200">
        <h2 className="text-lg font-semibold text-gray-900">Device Details</h2>
      </div>
      <div className="p-6 space-y-4">
        <div>
          <dt className="text-sm font-medium text-gray-500">Device ID</dt>
          <dd className="mt-1 text-sm text-gray-900 font-mono break-all">
            {device.ID}
          </dd>
        </div>
        {device.ReferenceToken && (
          <div>
            <dt className="text-sm font-medium text-gray-500">Reference Token</dt>
            <dd className="mt-1 text-sm text-gray-900 font-mono">
              {device.ReferenceToken}
            </dd>
          </div>
        )}
        {device.Status && (
          <div>
            <dt className="text-sm font-medium text-gray-500">Status</dt>
            <dd className="mt-1 text-sm text-gray-900">
              <span className={`px-2 py-1 text-xs font-medium rounded ${
                device.Status === 'healthy' 
                  ? 'bg-green-100 text-green-800' 
                  : 'bg-yellow-100 text-yellow-800'
              }`}>
                {device.Status}
              </span>
            </dd>
          </div>
        )}
        {device.SysInfo && (
          <div>
            <dt className="text-sm font-medium text-gray-500 mb-2">System Information</dt>
            <dd className="space-y-2 bg-gray-50 p-3 rounded">
              {device.SysInfo.Platform && (
                <div>
                  <span className="text-xs font-medium text-gray-500">Platform:</span>{' '}
                  <span className="text-sm text-gray-900">{device.SysInfo.Platform}</span>
                </div>
              )}
              {device.SysInfo.CPU && (
                <div>
                  <span className="text-xs font-medium text-gray-500">CPU:</span>{' '}
                  <span className="text-sm text-gray-900">{device.SysInfo.CPU}</span>
                </div>
              )}
              {device.SysInfo.RAM && (
                <div>
                  <span className="text-xs font-medium text-gray-500">RAM:</span>{' '}
                  <span className="text-sm text-gray-900">
                    {(device.SysInfo.RAM / 1024).toFixed(2)} GB
                  </span>
                </div>
              )}
              {device.SysInfo.GPU && device.SysInfo.GPU.length > 0 && (
                <div>
                  <span className="text-xs font-medium text-gray-500">GPU:</span>{' '}
                  <span className="text-sm text-gray-900">
                    {device.SysInfo.GPU.join(', ')}
                  </span>
                </div>
              )}
            </dd>
          </div>
        )}
        {device.Timestamp && (
          <div>
            <dt className="text-sm font-medium text-gray-500">Last Healthcheck</dt>
            <dd className="mt-1 text-sm text-gray-900">
              {new Date(device.Timestamp).toLocaleString()}
            </dd>
          </div>
        )}
        {device.Addresses && device.Addresses.length > 0 && (
          <div>
            <dt className="text-sm font-medium text-gray-500 mb-2">Addresses</dt>
            <dd className="space-y-1">
              {device.Addresses.map((addr, idx) => (
                <div key={idx} className="text-sm text-gray-900 font-mono bg-gray-50 p-2 rounded">
                  {String(addr)}
                </div>
              ))}
            </dd>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================================
// Upload File Modal Component
// ============================================================================

interface UploadFileModalProps {
  isOpen: boolean;
  onClose: () => void;
  onUpload: (file: File, remotePath: string) => Promise<void>;
  currentPath: string;
  uploading: boolean;
}

function UploadFileModal({ isOpen, onClose, onUpload, currentPath, uploading }: UploadFileModalProps) {
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [remotePath, setRemotePath] = useState('');

  if (!isOpen) return null;

  const handleSubmit = async () => {
    if (!selectedFile || !remotePath.trim()) {
      toast.warn('Please select a file and provide remote path');
      return;
    }
    await onUpload(selectedFile, remotePath.trim());
    setSelectedFile(null);
    setRemotePath('');
    // Reset file input
    const input = document.getElementById('file-upload-input') as HTMLInputElement;
    if (input) input.value = '';
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] || null;
    setSelectedFile(file);
    // Auto-fill remote path with filename if not set
    if (file && !remotePath.trim()) {
      const suggestedPath = currentPath === '/' 
        ? `/${file.name}` 
        : `${currentPath}/${file.name}`;
      setRemotePath(suggestedPath);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl p-6 w-full max-w-md">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-semibold text-gray-900">Upload File</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Select File
            </label>
            <input
              id="file-upload-input"
              type="file"
              onChange={handleFileChange}
              className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 text-sm file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-sm file:font-semibold file:bg-primary-50 file:text-primary-700 hover:file:bg-primary-100"
            />
            {selectedFile && (
              <p className="mt-2 text-sm text-gray-600">
                Selected: <span className="font-mono">{selectedFile.name}</span> ({(selectedFile.size / 1024).toFixed(2)} KB)
              </p>
            )}
            <p className="mt-1 text-xs text-gray-500">
              Select a file from your computer to upload
            </p>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Remote File Path
            </label>
            <input
              type="text"
              value={remotePath}
              onChange={(e) => setRemotePath(e.target.value)}
              placeholder={`e.g., ${currentPath}/file.txt`}
              className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm"
            />
            <p className="mt-1 text-xs text-gray-500">
              Destination path on the remote device
            </p>
          </div>
          <div className="flex space-x-2 pt-2">
            <button
              onClick={handleSubmit}
              disabled={!selectedFile || !remotePath.trim() || uploading}
              className="flex-1 flex items-center justify-center px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
            >
              {uploading ? (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Uploading...
                </>
              ) : (
                <>
                  <Upload className="w-4 h-4 mr-2" />
                  Upload
                </>
              )}
            </button>
            <button
              onClick={() => {
                onClose();
                setSelectedFile(null);
                setRemotePath('');
                const input = document.getElementById('file-upload-input') as HTMLInputElement;
                if (input) input.value = '';
              }}
              className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ============================================================================
// File Browser Component (Accordion)
// ============================================================================

interface FileBrowserProps {
  deviceId: string | null;
  isOpen: boolean;
  onToggle: () => void;
}

function FileBrowser({ deviceId, isOpen, onToggle }: FileBrowserProps) {
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [currentPath, setCurrentPath] = useState('/');
  const [loadingFiles, setLoadingFiles] = useState(false);
  const [showUpload, setShowUpload] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [downloadingFile, setDownloadingFile] = useState<string | null>(null);

  const loadFiles = useCallback(async (path: string) => {
    if (!deviceId) return;
    try {
      setLoadingFiles(true);
      const fileList = await api.listFiles(deviceId, path);
      setFiles(fileList);
      setCurrentPath(path);
    } catch (err) {
      toast.error(`Failed to load files: ${err instanceof Error ? err.message : 'Unknown error'}`);
    } finally {
      setLoadingFiles(false);
    }
  }, [deviceId]);

  // Reset file browser state when device changes
  useEffect(() => {
    setFiles([]);
    setCurrentPath('/');
    setShowUpload(false);
    setDownloadingFile(null);
  }, [deviceId]);

  // Load files when accordion is opened or device changes
  useEffect(() => {
    if (isOpen && deviceId) {
      loadFiles('/');
    }
  }, [isOpen, deviceId, loadFiles]);

  const handleFileClick = (file: FileEntry) => {
    if (!deviceId || !file.IsDir) return;
    const newPath = currentPath === '/' 
      ? `/${file.Filename}` 
      : `${currentPath}/${file.Filename}`;
    loadFiles(newPath);
  };

  const handleNavigateUp = () => {
    if (!deviceId || currentPath === '/') return;
    const pathParts = currentPath.split('/').filter(p => p);
    pathParts.pop();
    const newPath = pathParts.length === 0 ? '/' : '/' + pathParts.join('/');
    loadFiles(newPath);
  };

  const handleUpload = async (file: File, remotePath: string) => {
    if (!deviceId) return;
    try {
      setUploading(true);
      await api.uploadFileRaw(deviceId, file, remotePath);
      toast.success(`File ${file.name} uploaded successfully`);
      setShowUpload(false);
      await loadFiles(currentPath);
    } catch (err) {
      toast.error(`Failed to upload file: ${err instanceof Error ? err.message : 'Unknown error'}`);
    } finally {
      setUploading(false);
    }
  };

  const handleDownload = async (file: FileEntry) => {
    if (!deviceId || file.IsDir) return;
    
    // Construct remote path using DirectoryName from FileEntry for accuracy
    // DirectoryName might be relative (e.g., "test/MNIST/raw") or absolute (e.g., "/test/MNIST/raw")
    let remotePath: string;
    if (file.DirectoryName && file.DirectoryName !== '/' && file.DirectoryName !== '') {
      // Ensure DirectoryName starts with / for consistency
      const dirPath = file.DirectoryName.startsWith('/') 
        ? file.DirectoryName 
        : `/${file.DirectoryName}`;
      remotePath = `${dirPath}/${file.Filename}`;
    } else {
      // Fallback to currentPath if DirectoryName is not available
      remotePath = currentPath === '/' 
        ? `/${file.Filename}` 
        : `${currentPath}/${file.Filename}`;
    }

    try {
      setDownloadingFile(file.Filename);
      await api.downloadFileRaw(deviceId, remotePath, file.Filename);
      toast.success(`File ${file.Filename} downloaded successfully`);
    } catch (err) {
      toast.error(`Failed to download file: ${err instanceof Error ? err.message : 'Unknown error'}`);
    } finally {
      setDownloadingFile(null);
    }
  };

  if (!deviceId) return null;

  return (
    <>
      <div className="bg-white rounded-lg shadow-sm border border-gray-200">
        <button
          onClick={onToggle}
          className="w-full p-4 flex items-center justify-between hover:bg-gray-50 transition-colors"
        >
          <h2 className="text-lg font-semibold text-gray-900">File Browser</h2>
          {isOpen ? (
            <ChevronUp className="w-5 h-5 text-gray-500" />
          ) : (
            <ChevronDown className="w-5 h-5 text-gray-500" />
          )}
        </button>
        
        {isOpen && (
          <div className="border-t border-gray-200">
            <div className="p-4 border-b border-gray-200">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center space-x-2 flex-1">
                  {currentPath !== '/' && (
                    <button
                      onClick={handleNavigateUp}
                      className="p-1 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded"
                      title="Go up"
                    >
                      <ArrowLeft className="w-4 h-4" />
                    </button>
                  )}
                  <div className="flex-1 px-3 py-2 bg-gray-50 rounded font-mono text-sm text-gray-700">
                    {currentPath}
                  </div>
                  <button
                    onClick={() => loadFiles(currentPath)}
                    className="p-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded"
                    title="Refresh"
                  >
                    <RefreshCw className="w-4 h-4" />
                  </button>
                </div>
                <button
                  onClick={() => setShowUpload(true)}
                  className="ml-2 flex items-center px-3 py-1 text-sm bg-green-600 text-white rounded hover:bg-green-700"
                  title="Upload File"
                >
                  <Upload className="w-4 h-4 mr-1" />
                  Upload
                </button>
              </div>
            </div>
            <div className="p-4 max-h-96 overflow-y-auto">
              {loadingFiles ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="w-6 h-6 animate-spin text-primary-500" />
                </div>
              ) : files.length === 0 ? (
                <div className="text-center text-gray-500 py-8">No files found</div>
              ) : (
                <div className="space-y-1">
                  {files.map((file, idx) => (
                    <div
                      key={idx}
                      className={`flex items-center space-x-3 p-2 rounded transition-colors ${
                        file.IsDir 
                          ? 'hover:bg-blue-50 cursor-pointer' 
                          : 'hover:bg-gray-50'
                      }`}
                    >
                      <div
                        onClick={() => handleFileClick(file)}
                        className="flex items-center space-x-3 flex-1 min-w-0"
                      >
                        {file.IsDir ? (
                          <FolderOpen className="w-5 h-5 text-blue-600 flex-shrink-0" />
                        ) : (
                          <File className="w-5 h-5 text-gray-600 flex-shrink-0" />
                        )}
                        <div className="flex-1 min-w-0">
                          <p className="text-sm font-medium text-gray-900 truncate">
                            {file.Filename}
                          </p>
                          {!file.IsDir && (
                            <p className="text-xs text-gray-500">
                              {formatFileSize(file.Size)}
                            </p>
                          )}
                        </div>
                      </div>
                      {!file.IsDir && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDownload(file);
                          }}
                          disabled={downloadingFile === file.Filename}
                          className="p-1.5 text-blue-600 hover:text-blue-800 hover:bg-blue-50 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex-shrink-0"
                          title="Download file"
                        >
                          {downloadingFile === file.Filename ? (
                            <Loader2 className="w-4 h-4 animate-spin" />
                          ) : (
                            <Download className="w-4 h-4" />
                          )}
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      <UploadFileModal
        isOpen={showUpload}
        onClose={() => setShowUpload(false)}
        onUpload={handleUpload}
        currentPath={currentPath}
        uploading={uploading}
      />
    </>
  );
}

// ============================================================================
// Main Devices Page Component
// ============================================================================

export default function Devices() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [selectedDevice, setSelectedDevice] = useState<DeviceInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [showConnect, setShowConnect] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [fileBrowserOpen, setFileBrowserOpen] = useState(false);

  useEffect(() => {
    loadDevices();
  }, []);

  const loadDevices = async () => {
    try {
      const data = await api.getDevices();
      setDevices(data);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to load devices');
    } finally {
      setLoading(false);
    }
  };

  const handleDeviceSelect = async (deviceId: string) => {
    try {
      const info = await api.getDevice(deviceId);
      setSelectedDevice(info);
      // Reset file browser when selecting a new device
      setFileBrowserOpen(false);
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Failed to load device info';
      toast.error(errorMsg);
    }
  };

  const handlePing = async (deviceId: string) => {
    try {
      await api.pingDevice(deviceId);
      toast.success('Ping sent successfully');
    } catch (err) {
      toast.error(`Failed to ping device: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const handleConnect = async (peerInfo: string) => {
    try {
      setConnecting(true);
      const device = await api.connectToPeer(peerInfo);
      setShowConnect(false);
      toast.success('Successfully connected to device');
      await loadDevices();
      if (device && device.ID) {
        await handleDeviceSelect(device.ID);
      }
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Failed to connect to peer';
      toast.error(errorMsg);
    } finally {
      setConnecting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-primary-500" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Devices</h1>
          <p className="mt-2 text-gray-600">Manage connected P2P devices</p>
        </div>
        <div className="flex space-x-2">
          <button
            onClick={() => setShowConnect(true)}
            className="flex items-center px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors"
          >
            <Plus className="w-4 h-4 mr-2" />
            Connect Device
          </button>
          <button
            onClick={loadDevices}
            className="flex items-center px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors"
          >
            <RefreshCw className="w-4 h-4 mr-2" />
            Refresh
          </button>
        </div>
      </div>

      {/* Main Content: 2-Column Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Device List */}
        <DeviceList
          devices={devices}
          selectedDeviceId={selectedDevice?.ID || null}
          onDeviceSelect={handleDeviceSelect}
          onPing={handlePing}
        />

        {/* Right Column: Device Details + File Browser */}
        <div className="space-y-6">
          {/* Top: Device Details */}
          <DeviceDetails device={selectedDevice} />

          {/* Bottom: File Browser (Accordion) */}
          {selectedDevice && (
            <FileBrowser
              deviceId={selectedDevice.ID}
              isOpen={fileBrowserOpen}
              onToggle={() => setFileBrowserOpen(!fileBrowserOpen)}
            />
          )}
        </div>
      </div>

      {/* Modals */}
      <ConnectDeviceModal
        isOpen={showConnect}
        onClose={() => setShowConnect(false)}
        onConnect={handleConnect}
        connecting={connecting}
      />
    </div>
  );
}
