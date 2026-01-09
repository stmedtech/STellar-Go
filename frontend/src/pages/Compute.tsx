import { useEffect, useState, useRef, useCallback } from 'react';
import { toast } from 'react-toastify';
import { api } from '../api';
import type { Device, ComputeRun, LogEntry } from '../types';
import { Terminal, X, RefreshCw, Trash2, FileText } from 'lucide-react';

// ============================================================================
// LogsView Component - Reusable component for displaying merged logs with color coding
// ============================================================================

interface LogsViewProps {
  entries: LogEntry[];
  className?: string;
  emptyMessage?: string;
}

function LogsView({ entries, className = '', emptyMessage }: LogsViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const prevEntriesLengthRef = useRef(0);

  // Auto-scroll to bottom when content changes
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const hasNewEntries = entries.length > prevEntriesLengthRef.current;
    prevEntriesLengthRef.current = entries.length;

    if (hasNewEntries) {
      // Use requestAnimationFrame to ensure DOM has updated before scrolling
      requestAnimationFrame(() => {
        if (container) {
          container.scrollTop = container.scrollHeight;
        }
      });
    }
  }, [entries]);

  const hasContent = entries.length > 0;

  if (!hasContent && emptyMessage) {
    return (
      <div className={`bg-gray-900 text-gray-500 font-mono text-sm p-4 ${className}`}>
        {emptyMessage}
      </div>
    );
  }

  const getColorClass = (type: string) => {
    switch (type) {
      case 'stdout':
        return 'text-green-400';
      case 'stderr':
        return 'text-red-400';
      case 'error':
        return 'text-yellow-400';
      default:
        return 'text-gray-400';
    }
  };

  return (
    <div
      ref={containerRef}
      className={`bg-gray-900 font-mono text-sm p-4 overflow-y-auto whitespace-pre-wrap break-words ${className}`}
      style={{ fontFamily: 'monospace' }}
    >
      {entries.map((entry, index) => (
        <span key={index} className={getColorClass(entry.type)}>
          {entry.data}
        </span>
      ))}
      <div ref={endRef} />
    </div>
  );
}

// Command line parser (similar to utils.ParseCommandLine)
// Handles quoted strings (single and double quotes) and escaped characters
function parseCommandLine(cmdLine: string): { command: string; args: string[] } {
  const trimmed = cmdLine.trim();
  if (!trimmed) {
    return { command: '', args: [] };
  }

  const tokens: string[] = [];
  let current = '';
  let inSingleQuote = false;
  let inDoubleQuote = false;
  let escapeNext = false;

  for (let i = 0; i < trimmed.length; i++) {
    const char = trimmed[i];

    if (escapeNext) {
      escapeNext = false;
      if (inSingleQuote) {
        // In single quotes, only \' and \\ are special
        if (char === "'" || char === '\\') {
          current += char;
        } else {
          current += '\\' + char;
        }
      } else {
        // Handle escape sequences
        switch (char) {
          case 'n':
            current += '\n';
            break;
          case 't':
            current += '\t';
            break;
          case 'r':
            current += '\r';
            break;
          case '\\':
            current += '\\';
            break;
          case '"':
            current += '"';
            break;
          case "'":
            current += "'";
            break;
          default:
            current += char;
        }
      }
      continue;
    }

    switch (char) {
      case '\\':
        if (inSingleQuote) {
          // Check if next char is a single quote or backslash
          if (i + 1 < trimmed.length) {
            const next = trimmed[i + 1];
            if (next === "'" || next === '\\') {
              escapeNext = true;
              continue;
            }
          }
          current += char;
        } else {
          escapeNext = true;
        }
        break;

      case "'":
        if (inDoubleQuote) {
          current += char;
        } else if (inSingleQuote) {
          inSingleQuote = false;
          if (current) {
            tokens.push(current);
            current = '';
          }
        } else {
          inSingleQuote = true;
        }
        break;

      case '"':
        if (inSingleQuote) {
          current += char;
        } else if (inDoubleQuote) {
          inDoubleQuote = false;
          if (current) {
            tokens.push(current);
            current = '';
          }
        } else {
          inDoubleQuote = true;
        }
        break;

      case ' ':
      case '\t':
        if (inSingleQuote || inDoubleQuote) {
          current += char;
        } else {
          if (current) {
            tokens.push(current);
            current = '';
          }
        }
        break;

      default:
        current += char;
    }
  }

  // Add the last token
  if (current || inSingleQuote || inDoubleQuote) {
    tokens.push(current);
  }

  if (tokens.length === 0) {
    return { command: '', args: [] };
  }

  return {
    command: tokens[0],
    args: tokens.slice(1),
  };
}

export default function Compute() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [selectedDevice, setSelectedDevice] = useState<string>('');
  const [hostNodeID, setHostNodeID] = useState<string | null>(null);
  const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
  const [commandInput, setCommandInput] = useState('');
  const [status, setStatus] = useState<string>('Ready');
  const [currentRunId, setCurrentRunId] = useState<string | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [computeRuns, setComputeRuns] = useState<ComputeRun[]>([]);
  const [loadingRuns, setLoadingRuns] = useState(false);
  const [viewingLogsFor, setViewingLogsFor] = useState<string | null>(null);
  const [viewLogsEntries, setViewLogsEntries] = useState<LogEntry[]>([]);
  const [loadingLogs, setLoadingLogs] = useState(false);

  useEffect(() => {
    loadDevices();
    loadNodeInfo();
  }, []);

  useEffect(() => {
    if (selectedDevice) {
      setLogEntries([]);
      setStatus('Ready');
      setCurrentRunId(null);
      setIsRunning(false);
      loadComputeRuns();
      // Poll for run updates every 2 seconds
      const interval = setInterval(() => {
        loadComputeRuns();
      }, 2000);
      return () => clearInterval(interval);
    }
  }, [selectedDevice]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadDevices = async () => {
    try {
      const data = await api.getDevices();
      setDevices(data);
      if (data.length > 0 && !selectedDevice) {
        setSelectedDevice(data[0].ID);
      }
    } catch (err) {
      toast.error(`Failed to load devices: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const loadNodeInfo = async () => {
    try {
      const nodeInfo = await api.getNodeInfo();
      setHostNodeID(nodeInfo.NodeID);
    } catch (err) {
      // Silently fail - host node indicator is optional
    }
  };

  const loadComputeRuns = async () => {
    if (!selectedDevice) return;
    try {
      setLoadingRuns(true);
      const runs = await api.listComputeRuns(selectedDevice);
      // Sort by created time (newest first)
      runs.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime());
      setComputeRuns(runs);
    } catch (err) {
      // Silently fail for polling, only show error on manual refresh
    } finally {
      setLoadingRuns(false);
    }
  };

  const appendLogEntries = useCallback((newEntries: LogEntry[]) => {
    setLogEntries(prev => {
      // Merge new entries, avoiding duplicates based on timestamp and data
      const existing = new Map<string, LogEntry>();
      prev.forEach(entry => {
        const key = `${entry.time}-${entry.type}-${entry.data.substring(0, 50)}`;
        existing.set(key, entry);
      });
      newEntries.forEach(entry => {
        const key = `${entry.time}-${entry.type}-${entry.data.substring(0, 50)}`;
        if (!existing.has(key)) {
          existing.set(key, entry);
        }
      });
      // Sort by timestamp
      return Array.from(existing.values()).sort((a, b) => 
        new Date(a.time).getTime() - new Date(b.time).getTime()
      );
    });
  }, []);

  const streamOutput = useCallback(async (runId: string) => {
    if (!selectedDevice) return;

    const pollInterval = 500; // Poll every 500ms for updates
    let lastEntryCount = 0;

    const poll = async () => {
      try {
        // Get current run status
        const run = await api.getComputeRun(selectedDevice, runId);
        
        // Fetch logs (merged stdout/stderr with timestamps)
        try {
          const entries = await api.streamLogs(selectedDevice, runId);
          if (entries.length > lastEntryCount) {
            const newEntries = entries.slice(lastEntryCount);
            appendLogEntries(newEntries);
            lastEntryCount = entries.length;
          }
        } catch (err) {
          // Ignore errors, might be empty
        }

        if (run.status === 'running') {
          // Continue polling
          setTimeout(poll, pollInterval);
        } else {
          // Command finished, get final logs
          try {
            const entries = await api.streamLogs(selectedDevice, runId);
            if (entries.length > lastEntryCount) {
              const newEntries = entries.slice(lastEntryCount);
              appendLogEntries(newEntries);
            }
          } catch (err) {
            // Ignore
          }

          // Show exit code as a log entry
          if (run.exit_code !== undefined) {
            const exitEntry: LogEntry = {
              run_id: runId,
              type: run.error ? 'error' : 'stdout',
              data: run.error ? `\n[exit] error: ${run.error}\n` : `\n[exit] code=${run.exit_code}\n`,
              time: new Date().toISOString(),
            };
            appendLogEntries([exitEntry]);
          }

          setIsRunning(false);
          setCurrentRunId(null);
          setStatus('Ready');
        }
      } catch (err) {
        const errorEntry: LogEntry = {
          run_id: runId,
          type: 'error',
          data: `\n[error] Failed to get run status: ${err instanceof Error ? err.message : 'Unknown error'}\n`,
          time: new Date().toISOString(),
        };
        appendLogEntries([errorEntry]);
        setIsRunning(false);
        setCurrentRunId(null);
        setStatus('Ready');
      }
    };

    // Start polling
    poll();
  }, [selectedDevice, appendLogEntries]);

  const executeCommand = useCallback(async (cmdLine: string) => {
    if (!selectedDevice || !cmdLine.trim()) return;

    const { command, args } = parseCommandLine(cmdLine);
    if (!command) {
      toast.warn('Empty command');
      return;
    }

    try {
      setStatus('Executing...');
      setIsRunning(true);
      
      // Show command being executed
      const cmdEntry: LogEntry = {
        run_id: '',
        type: 'stdout',
        data: `$ ${cmdLine}\n`,
        time: new Date().toISOString(),
      };
      appendLogEntries([cmdEntry]);

      // Execute command
      const run = await api.runCompute(selectedDevice, command, args);
      setCurrentRunId(run.id);
      setStatus('Running');

      // Start streaming output
      streamOutput(run.id);
      
      // Refresh runs list to show the new run
      await loadComputeRuns();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Unknown error';
      const errorEntry: LogEntry = {
        run_id: currentRunId || '',
        type: 'error',
        data: `\n[error] Failed to execute command: ${errorMsg}\n`,
        time: new Date().toISOString(),
      };
      appendLogEntries([errorEntry]);
      setIsRunning(false);
      setCurrentRunId(null);
      setStatus('Ready');
      toast.error(`Failed to execute command: ${errorMsg}`);
    }
  }, [selectedDevice, appendLogEntries, streamOutput, currentRunId]);

  const handleInputSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const cmdLine = commandInput.trim();
    if (!cmdLine) return;

    // If a command is running, send input to stdin
    if (isRunning && currentRunId) {
      try {
        await api.sendStdin(selectedDevice, currentRunId, cmdLine + '\n');
        const stdinEntry: LogEntry = {
          run_id: currentRunId,
          type: 'stdout',
          data: cmdLine + '\n',
          time: new Date().toISOString(),
        };
        appendLogEntries([stdinEntry]);
        setCommandInput('');
        return;
      } catch (err) {
        toast.error(`Failed to send input: ${err instanceof Error ? err.message : 'Unknown error'}`);
        return;
      }
    }

    // Execute new command
    setCommandInput('');
    await executeCommand(cmdLine);
  };

  const handleCancel = async () => {
    if (!selectedDevice || !currentRunId || !isRunning) return;
    await handleCancelRun(currentRunId);
  };

  const handleCancelRun = async (runId: string) => {
    if (!selectedDevice) return;

    try {
      await api.cancelComputeRun(selectedDevice, runId);
      
      // If this is the currently running command, update terminal state
      if (runId === currentRunId) {
        const cancelEntry: LogEntry = {
          run_id: runId,
          type: 'error',
          data: '\n[cancelled]\n',
          time: new Date().toISOString(),
        };
        appendLogEntries([cancelEntry]);
        setIsRunning(false);
        setCurrentRunId(null);
        setStatus('Ready');
      }
      
      // Refresh runs list to show updated status
      await loadComputeRuns();
      toast.success('Command cancelled');
    } catch (err) {
      toast.error(`Failed to cancel: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const handleClear = () => {
    setLogEntries([]);
  };

  const handleDeleteRun = async (runId: string) => {
    if (!selectedDevice) return;
    try {
      await api.deleteComputeRun(selectedDevice, runId);
      await loadComputeRuns();
      toast.success('Run deleted');
    } catch (err) {
      toast.error(`Failed to delete run: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running':
        return 'bg-blue-100 text-blue-800';
      case 'completed':
        return 'bg-green-100 text-green-800';
      case 'failed':
        return 'bg-red-100 text-red-800';
      case 'cancelled':
        return 'bg-gray-100 text-gray-800';
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  const handleViewLogs = async (runId: string) => {
    if (!selectedDevice) return;
    setViewingLogsFor(runId);
    setViewLogsEntries([]);
    setLoadingLogs(true);
    try {
      // Fetch logs (merged stdout/stderr with timestamps)
      const entries = await api.streamLogs(selectedDevice, runId);
      setViewLogsEntries(entries);
    } catch (err) {
      toast.error(`Failed to load logs: ${err instanceof Error ? err.message : 'Unknown error'}`);
      const errorEntry: LogEntry = {
        run_id: runId,
        type: 'error',
        data: `Error: ${err instanceof Error ? err.message : 'Unknown error'}`,
        time: new Date().toISOString(),
      };
      setViewLogsEntries([errorEntry]);
    } finally {
      setLoadingLogs(false);
    }
  };

  const handleCloseLogs = () => {
    setViewingLogsFor(null);
    setViewLogsEntries([]);
  };

  // Poll for log updates when viewing logs
  useEffect(() => {
    if (!viewingLogsFor || !selectedDevice) return;

    let lastEntryCount = 0;
    let isPolling = true;
    let timeoutId: ReturnType<typeof setTimeout> | null = null;

    const pollLogs = async () => {
      if (!isPolling || !viewingLogsFor || !selectedDevice) return;

      try {
        // Get current run status to check if it's still running
        const run = await api.getComputeRun(selectedDevice, viewingLogsFor).catch(() => null);
        
        // Fetch logs
        const entries = await api.streamLogs(selectedDevice, viewingLogsFor);
        
        // Update entries incrementally (merge new entries)
        setViewLogsEntries(prev => {
          // Calculate new entries based on the difference
          const newEntries = entries.slice(lastEntryCount);
          
          if (newEntries.length === 0 && entries.length === prev.length) {
            return prev; // No new entries and counts match
          }

          // Merge all entries, avoiding duplicates
          const existing = new Map<string, LogEntry>();
          prev.forEach(entry => {
            const key = `${entry.time}-${entry.type}-${entry.data.substring(0, 50)}`;
            existing.set(key, entry);
          });
          entries.forEach(entry => {
            const key = `${entry.time}-${entry.type}-${entry.data.substring(0, 50)}`;
            if (!existing.has(key)) {
              existing.set(key, entry);
            }
          });
          // Sort by timestamp
          const merged = Array.from(existing.values()).sort((a, b) => 
            new Date(a.time).getTime() - new Date(b.time).getTime()
          );
          lastEntryCount = merged.length;
          return merged;
        });

        // Continue polling if run is still running
        if (run && run.status === 'running') {
          timeoutId = setTimeout(pollLogs, 2000); // Poll every 2s
        } else {
          isPolling = false;
        }
      } catch (err) {
        // Silently fail, don't spam errors
        if (isPolling) {
          timeoutId = setTimeout(pollLogs, 2000); // Retry after 2 seconds on error
        }
      }
    };

    // Start polling after initial load completes
    // Wait a bit to let the initial load finish, then initialize lastEntryCount
    timeoutId = setTimeout(() => {
      setViewLogsEntries(prev => {
        lastEntryCount = prev.length;
        return prev;
      });
      pollLogs();
    }, 500);

    return () => {  
      isPolling = false;
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, [viewingLogsFor, selectedDevice]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">Terminal</h1>
        <p className="mt-2 text-gray-600">Interactive terminal for remote devices</p>
      </div>

      {/* Device Selection */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-4">
        <div className="flex items-center space-x-4">
          <label className="text-sm font-medium text-gray-700 whitespace-nowrap">
            Device:
          </label>
          <div className="flex-1 flex items-center space-x-2">
            <select
              value={selectedDevice}
              onChange={(e) => setSelectedDevice(e.target.value)}
              className="flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
            >
              <option value="">Select a device...</option>
              {[{ID: hostNodeID || ''}, ...devices].filter(Boolean).map((device) => (
                <option key={device.ID} value={device.ID} title={device.ID}>
                  {device.ID}
                </option>
              ))}
            </select>
            {selectedDevice && hostNodeID && selectedDevice === hostNodeID && (
              <span className="px-2 py-1 text-xs font-medium bg-blue-100 text-blue-800 rounded">
                This Node
              </span>
            )}
          </div>
          <div className="text-sm text-gray-600">
            {status}
          </div>
        </div>
      </div>

      {/* Terminal Output */}
      {selectedDevice && (
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
          <div className="bg-gray-800 text-gray-200 px-4 py-2 flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <Terminal className="w-4 h-4" />
              <span className="text-sm font-medium">
                Terminal - {selectedDevice.substring(0, 20)}...
                {hostNodeID && selectedDevice === hostNodeID && (
                  <span className="ml-2 px-2 py-0.5 text-xs font-medium bg-blue-100 text-blue-800 rounded">
                    Local
                  </span>
                )}
              </span>
            </div>
            <button
              onClick={handleClear}
              className="text-xs text-gray-400 hover:text-gray-200 px-2 py-1 rounded hover:bg-gray-700"
            >
              Clear
            </button>
          </div>
          <LogsView
            entries={logEntries}
            className="h-96"
            emptyMessage="Type a command and press Enter to execute.\nIf a command is running, type input and press Enter to send to stdin."
          />
          
          {/* Command Input */}
          <form onSubmit={handleInputSubmit} className="border-t border-gray-200 bg-gray-50">
            <div className="flex items-center p-2">
              <span className="text-gray-600 font-mono text-sm mr-2">$</span>
              <input
                type="text"
                value={commandInput}
                onChange={(e) => setCommandInput(e.target.value)}
                placeholder={isRunning ? "Send input to running command..." : "Type command and press Enter..."}
                className="flex-1 bg-transparent border-0 focus:ring-0 focus:outline-none font-mono text-sm"
                disabled={!selectedDevice}
              />
              {isRunning && (
                <button
                  type="button"
                  onClick={handleCancel}
                  className="ml-2 px-3 py-1 text-sm bg-red-100 text-red-700 rounded hover:bg-red-200 flex items-center space-x-1"
                >
                  <X className="w-4 h-4" />
                  <span>Cancel</span>
                </button>
              )}
            </div>
          </form>
        </div>
      )}

      {!selectedDevice && (
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6 text-center text-gray-500">
          Please select a device to start using the terminal
        </div>
      )}

      {/* Compute Runs List */}
      {selectedDevice && (
        <div className="bg-white rounded-lg shadow-sm border border-gray-200">
          <div className="p-4 border-b border-gray-200 flex items-center justify-between">
            <h2 className="text-lg font-semibold text-gray-900">Command History</h2>
            <button
              onClick={loadComputeRuns}
              disabled={loadingRuns}
              className="flex items-center px-3 py-1 text-sm bg-gray-100 text-gray-700 rounded hover:bg-gray-200 disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 mr-2 ${loadingRuns ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
          {computeRuns.length === 0 ? (
            <div className="p-6 text-center text-gray-500">No commands executed yet</div>
          ) : (
            <div className="divide-y divide-gray-200 max-h-96 overflow-y-auto">
              {computeRuns.map((run: ComputeRun) => (
                <div key={run.id} className="p-4 hover:bg-gray-50">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center space-x-3 flex-1 min-w-0">
                      <Terminal className="w-4 h-4 text-gray-400 flex-shrink-0" />
                      <div className="flex-1 min-w-0">
                        <p className="font-medium text-gray-900 font-mono text-sm truncate">
                          {run.command} {run.args?.join(' ') || ''}
                        </p>
                        <p className="text-xs text-gray-500 truncate" title={run.id}>
                          ID: {run.id}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center space-x-2">
                      <button
                        onClick={() => handleViewLogs(run.id)}
                        className="p-1 text-gray-400 hover:text-blue-600 hover:bg-blue-50 rounded"
                        title="View Logs"
                      >
                        <FileText className="w-4 h-4" />
                      </button>
                      <span
                        className={`px-2 py-1 text-xs font-medium rounded ${getStatusColor(run.status)}`}
                      >
                        {run.status}
                      </span>
                      {run.status === 'running' && (
                        <button
                          onClick={() => handleCancelRun(run.id)}
                          className="p-1 text-red-600 hover:bg-red-50 rounded"
                          title="Cancel"
                        >
                          <X className="w-4 h-4" />
                        </button>
                      )}
                      {run.status !== 'running' && (
                        <button
                          onClick={() => handleDeleteRun(run.id)}
                          className="p-1 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded"
                          title="Delete"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                    </div>
                  </div>
                  <div className="text-xs text-gray-500 space-y-1 mt-2 ml-7">
                    <p>Created: {new Date(run.created).toLocaleString()}</p>
                    {run.started && <p>Started: {new Date(run.started).toLocaleString()}</p>}
                    {run.finished && <p>Finished: {new Date(run.finished).toLocaleString()}</p>}
                    {run.exit_code !== undefined && <p>Exit Code: {run.exit_code}</p>}
                    {run.error && <p className="text-red-600">Error: {run.error}</p>}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Logs Modal */}
      {viewingLogsFor && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-xl w-full max-w-4xl max-h-[80vh] flex flex-col">
            <div className="p-4 border-b border-gray-200 flex items-center justify-between">
              <h2 className="text-xl font-semibold text-gray-900">Logs</h2>
              <button
                onClick={handleCloseLogs}
                className="text-gray-400 hover:text-gray-600"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="flex-1 overflow-auto p-4">
              {loadingLogs ? (
                <div className="text-center text-gray-500 py-8">Loading logs...</div>
              ) : (
                <LogsView
                  entries={viewLogsEntries}
                  className="rounded"
                  emptyMessage="No logs available"
                />
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
