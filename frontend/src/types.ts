export interface NodeInfo {
  NodeID: string;
  Addresses: string[];
  Bootstrapper: boolean;
  RelayNode: boolean;
  ReferenceToken: string;
  DevicesCount: number;
  Policy: Policy;
}

export interface Policy {
  Enable: boolean;
  WhiteList: string[];
}

export interface Device {
  ID: string;
  Addresses: string[];
  LastSeen?: string;
}

export interface SystemInformation {
  Platform: string;
  CPU: string;
  GPU: string[];
  RAM: number;
}

export interface DeviceInfo {
  ID: string;
  ReferenceToken?: string;
  Status?: string;
  SysInfo?: SystemInformation;
  Timestamp?: string;
  Addresses?: string[];
  [key: string]: any;
}

export interface ComputeRun {
  id: string;
  command: string;
  args?: string[];
  status: 'running' | 'completed' | 'failed' | 'cancelled' | 'removed';
  created: string;
  started?: string;
  finished?: string;
  exit_code?: number;
  error?: string;
  endpoints?: {
    stdout: string;
    stderr: string;
    logs: string;
    stdin: string;
  };
}

export interface Proxy {
  local_port: number;
  remote_addr: string;
  device_id: string;
}

export interface HealthStatus {
  status: string;
}

export interface FileEntry {
  DirectoryName: string;
  Filename: string;
  Size: number;
  IsDir: boolean;
  Children?: FileEntry[];
}

export interface LogEntry {
  run_id: string;
  type: 'stdout' | 'stderr' | 'error';
  data: string;
  time: string;
}
