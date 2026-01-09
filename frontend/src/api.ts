import type { NodeInfo, Device, DeviceInfo, ComputeRun, Proxy, Policy, HealthStatus, FileEntry } from './types';

const API_BASE = '';

/**
 * Handles error responses from the API, with special handling for 403 status codes.
 * 403 indicates that the destination node rejected the protocol because policy is enabled
 * and the current node ID is not in the whitelist.
 */
function handleErrorResponse(response: Response, responseText?: string): Error {
  // Extract error message from response body first
  let errorMessage = `API error: ${response.statusText}`;
  if (responseText) {
    try {
      const errorData = JSON.parse(responseText);
      if (errorData && typeof errorData === 'object') {
        if ('error' in errorData) {
          errorMessage = String(errorData.error);
        } else if ('message' in errorData) {
          errorMessage = String(errorData.message);
        }
      }
    } catch {
      // If JSON parsing fails, check if responseText itself contains error info
      if (responseText && responseText.trim()) {
        errorMessage = responseText.trim();
      }
    }
  }

  // Special handling for 403 Forbidden (check both status code and error message)
  const is403Error = response.status === 403 || 
                     errorMessage.includes('error code: 403') ||
                     errorMessage.includes('code: 0x193') ||
                     (errorMessage.includes('stream reset') && errorMessage.includes('403'));

  if (is403Error) {
    return new Error(
      'Access denied: The destination node rejected this protocol request because ' +
      'policy is enabled and your node ID is not in the whitelist. ' +
      'Please ask the destination node administrator to add your node to the whitelist.'
    );
  }

  return new Error(errorMessage);
}

async function fetchAPI<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });

  if (!response.ok) {
    const text = await response.text();
    throw handleErrorResponse(response, text);
  }

  // Check if response has content
  const contentType = response.headers.get('content-type');
  const text = await response.text();
  
  // If response is empty, return empty object or null based on expected type
  if (!text || text.trim() === '') {
    return {} as T;
  }

  // Only parse as JSON if content-type indicates JSON or if text looks like JSON
  if (contentType && contentType.includes('application/json')) {
    try {
      return JSON.parse(text);
    } catch {
      // If JSON parsing fails but content-type says JSON, return the text as-is
      return text as unknown as T;
    }
  }

  // Try to parse as JSON anyway (some endpoints might not set content-type correctly)
  try {
    return JSON.parse(text);
  } catch {
    // If it's not JSON, return the text as-is
    return text as unknown as T;
  }
}

export const api = {
  // Health & Node
  getHealth: (): Promise<HealthStatus> => fetchAPI('/health'),
  getNodeInfo: (): Promise<NodeInfo> => fetchAPI('/node'),

  // Devices
  getDevices: async (): Promise<Device[]> => {
    const devicesMap = await fetchAPI<Record<string, Device>>('/devices');
    // Convert map to array
    return Object.values(devicesMap || {});
  },
  getDevice: (deviceId: string): Promise<DeviceInfo> => fetchAPI(`/devices/${deviceId}`),
  getDeviceTree: (deviceId: string): Promise<any> => fetchAPI(`/devices/${deviceId}/tree`),
  pingDevice: (deviceId: string): Promise<any> => 
    fetchAPI(`/devices/${deviceId}/ping`, { method: 'POST' }),

  // Files
  listFiles: (deviceId: string, path?: string): Promise<FileEntry[]> => {
    const url = path ? `/devices/${deviceId}/files?path=${encodeURIComponent(path)}` : `/devices/${deviceId}/files`;
    return fetchAPI(url);
  },
  downloadFile: async (deviceId: string, remotePath: string, destPath: string): Promise<any> => {
    const url = `/devices/${deviceId}/files/download?remotePath=${encodeURIComponent(remotePath)}&destPath=${encodeURIComponent(destPath)}`;
    return fetchAPI(url);
  },
  uploadFile: async (deviceId: string, localPath: string, remotePath: string): Promise<any> => {
    // UploadFile expects form data
    const formData = new URLSearchParams();
    formData.append('localPath', localPath);
    formData.append('remotePath', remotePath);
    const response = await fetch(`${API_BASE}/devices/${deviceId}/files/upload`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: formData.toString(),
    });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    const text = await response.text();
    if (!text || text.trim() === '') {
      return { success: true };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { success: true };
    }
  },
  // Raw file upload (from browser file input)
  uploadFileRaw: async (deviceId: string, file: File, remotePath: string): Promise<any> => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('remotePath', remotePath);
    
    const response = await fetch(`${API_BASE}/devices/${deviceId}/files/upload/raw`, {
      method: 'POST',
      body: formData,
    });
    
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    
    const text = await response.text();
    if (!text || text.trim() === '') {
      return { success: true };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { success: true };
    }
  },
  // Raw file download (streams to browser)
  downloadFileRaw: async (deviceId: string, remotePath: string, filename?: string): Promise<void> => {
    const url = `/devices/${deviceId}/files/download/raw?remotePath=${encodeURIComponent(remotePath)}`;
    const response = await fetch(`${API_BASE}${url}`, {
      method: 'GET',
    });
    
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    
    // Get filename from Content-Disposition header or use provided/default
    let downloadFilename = filename;
    const contentDisposition = response.headers.get('Content-Disposition');
    if (!downloadFilename && contentDisposition) {
      const filenameMatch = contentDisposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/);
      if (filenameMatch && filenameMatch[1]) {
        downloadFilename = filenameMatch[1].replace(/['"]/g, '');
      }
    }
    if (!downloadFilename) {
      downloadFilename = 'download';
    }
    
    // Stream download for large files instead of loading entire blob into memory at once
    const contentLength = response.headers.get('Content-Length');
    const totalBytes = contentLength ? parseInt(contentLength, 10) : 0;
    
    // Use ReadableStream for efficient chunked reading
    if (!response.body) {
      throw new Error('Response body is not available');
    }
    
    const reader = response.body.getReader();
    const chunks: Uint8Array[] = [];
    let receivedBytes = 0;
    let lastProgressLog = 0;
    
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        
        if (value) {
          chunks.push(value);
          receivedBytes += value.length;
          
          // Log progress for large files (every 5MB or at completion)
          if (totalBytes > 0) {
            const progressMB = Math.floor(receivedBytes / (5 * 1024 * 1024));
            if (progressMB > lastProgressLog || receivedBytes >= totalBytes) {
              const progress = ((receivedBytes / totalBytes) * 100).toFixed(1);
              console.log(`Download progress: ${progress}% (${(receivedBytes / 1024 / 1024).toFixed(2)} MB / ${(totalBytes / 1024 / 1024).toFixed(2)} MB)`);
              lastProgressLog = progressMB;
            }
          }
        }
      }
      
      // Combine chunks into a single blob
      // Note: For very large files (>100MB), this still uses memory, but chunked reading
      // is more efficient than loading everything at once
      const blob = new Blob(chunks as BlobPart[], { type: response.headers.get('Content-Type') || 'application/octet-stream' });
      
      // Trigger download
      const url_obj = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url_obj;
      a.download = downloadFilename;
      document.body.appendChild(a);
      a.click();
      // Small delay before cleanup to ensure download starts
      setTimeout(() => {
        document.body.removeChild(a);
        window.URL.revokeObjectURL(url_obj);
      }, 100);
    } catch (err) {
      reader.cancel();
      throw err;
    }
  },

  // Compute
  listComputeRuns: (deviceId: string): Promise<ComputeRun[]> => 
    fetchAPI(`/devices/${deviceId}/compute/runs`),
  getComputeRun: (deviceId: string, runId: string): Promise<ComputeRun> => 
    fetchAPI(`/devices/${deviceId}/compute/runs/${runId}`),
  runCompute: (deviceId: string, command: string, args: string[]): Promise<ComputeRun> =>
    fetchAPI(`/devices/${deviceId}/compute/execute`, {
      method: 'POST',
      body: JSON.stringify({ command, args }),
    }),
  cancelComputeRun: (deviceId: string, runId: string): Promise<any> =>
    fetchAPI(`/devices/${deviceId}/compute/runs/${runId}/cancel`, { method: 'POST' }),
  deleteComputeRun: (deviceId: string, runId: string): Promise<any> =>
    fetchAPI(`/devices/${deviceId}/compute/runs/${runId}`, { method: 'DELETE' }),
  // Stream output endpoints
  streamStdout: async (deviceId: string, runId: string, follow?: boolean): Promise<string> => {
    const url = `/devices/${deviceId}/compute/runs/${runId}/stdout${follow ? '?follow=true' : ''}`;
    const response = await fetch(`${API_BASE}${url}`, { method: 'GET' });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    return await response.text();
  },
  streamStderr: async (deviceId: string, runId: string, follow?: boolean): Promise<string> => {
    const url = `/devices/${deviceId}/compute/runs/${runId}/stderr${follow ? '?follow=true' : ''}`;
    const response = await fetch(`${API_BASE}${url}`, { method: 'GET' });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    return await response.text();
  },
  streamLogs: async (deviceId: string, runId: string, follow?: boolean): Promise<import('./types').LogEntry[]> => {
    const url = `/devices/${deviceId}/compute/runs/${runId}/logs${follow ? '?follow=true' : ''}`;
    const response = await fetch(`${API_BASE}${url}`, { method: 'GET' });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    const text = await response.text();
    // Parse NDJSON (newline-delimited JSON)
    const entries: import('./types').LogEntry[] = [];
    for (const line of text.split('\n')) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      try {
        const entry = JSON.parse(trimmed);
        entries.push(entry);
      } catch (err) {
        // Skip invalid JSON lines
        console.warn('Failed to parse log entry:', trimmed);
      }
    }
    return entries;
  },
  sendStdin: async (deviceId: string, runId: string, input: string): Promise<any> => {
    const response = await fetch(`${API_BASE}/devices/${deviceId}/compute/runs/${runId}/stdin`, {
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
      },
      body: input,
    });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    const text = await response.text();
    if (!text || text.trim() === '') {
      return { success: true };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { success: true };
    }
  },

  // Proxy
  listProxies: (): Promise<Proxy[]> => fetchAPI('/proxy'),
  createProxy: (deviceId: string, localPort: number, remoteHost: string, remotePort: number): Promise<any> =>
    fetchAPI('/proxy', {
      method: 'POST',
      body: JSON.stringify({ 
        device_id: deviceId,
        local_port: localPort,
        remote_host: remoteHost,
        remote_port: remotePort,
      }),
    }),
  closeProxy: (port: number): Promise<any> =>
    fetchAPI(`/proxy/${port}`, { method: 'DELETE' }),

  // Policy
  getPolicy: (): Promise<Policy> => fetchAPI('/policy'),
  setPolicy: async (enable: boolean): Promise<any> => {
    // SetPolicy expects form data, not JSON
    const formData = new URLSearchParams();
    formData.append('enable', String(enable));
    const response = await fetch(`${API_BASE}/policy`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: formData.toString(),
    });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    // SetPolicy returns empty response, so just return success
    return { success: true };
  },
  getWhitelist: (): Promise<string[]> => fetchAPI('/policy/whitelist'),
  addToWhitelist: async (deviceId: string): Promise<any> => {
    // AddPolicyWhiteList expects form data
    const formData = new URLSearchParams();
    formData.append('deviceId', deviceId);
    const response = await fetch(`${API_BASE}/policy/whitelist`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: formData.toString(),
    });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    const text = await response.text();
    if (!text || text.trim() === '') {
      return { success: true };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { success: true };
    }
  },
  removeFromWhitelist: async (deviceId: string): Promise<any> => {
    // RemovePolicyWhiteList accepts query parameter or form data
    const response = await fetch(`${API_BASE}/policy/whitelist?deviceId=${encodeURIComponent(deviceId)}`, {
      method: 'DELETE',
    });
    if (!response.ok) {
      const text = await response.text();
      throw handleErrorResponse(response, text);
    }
    const text = await response.text();
    if (!text || text.trim() === '') {
      return { success: true };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { success: true };
    }
  },

  // Connect
  connectToPeer: (peerInfo: string): Promise<any> =>
    fetchAPI('/connect', {
      method: 'POST',
      body: JSON.stringify({ peer_info: peerInfo }),
    }),
};
