export interface LogEntry {
  timestamp: string;
  log: string;
  logLevel: 'ERROR' | 'WARN' | 'INFO' | 'DEBUG';
  componentId: string;
  environmentId: string;
  projectId: string;
  version: string;
  versionId: string;
  namespace: string;
  podId: string;
  containerName: string;
  labels: Record<string, string>;
}

export interface LogsResponse {
  logs: LogEntry[];
  totalCount: number;
  tookMs: number;
}

export interface Environment {
  id: string;
  name: string;
}

export interface RuntimeLogsFilters {
  logLevel: string[];
  environmentId: string;
  timeRange: string;
}

export interface RuntimeLogsPagination {
  hasMore: boolean;
  offset: number;
  limit: number;
}

export interface RuntimeLogsState {
  logs: LogEntry[];
  loading: boolean;
  error: string | null;
  filters: RuntimeLogsFilters;
  pagination: RuntimeLogsPagination;
}

export interface RuntimeLogsParams {
  environmentId: string;
  logLevels: string[];
  startTime: string;
  endTime: string;
  limit?: number;
  offset?: number;
}

export type LogLevel = 'ERROR' | 'WARN' | 'INFO' | 'DEBUG';

export const TIME_RANGE_OPTIONS = [
  { value: '10m', label: 'Last 10 minutes' },
  { value: '30m', label: 'Last 30 minutes' },
  { value: '1h', label: 'Last 1 hour' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '14d', label: 'Last 14 days' },
] as const;

export const LOG_LEVELS: LogLevel[] = ['ERROR', 'WARN', 'INFO', 'DEBUG'];
