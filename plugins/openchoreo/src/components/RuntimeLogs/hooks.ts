import { useCallback, useEffect, useRef, useState } from 'react';
import { useEntity } from '@backstage/plugin-catalog-react';
import { discoveryApiRef, identityApiRef } from '@backstage/core-plugin-api';
import { useApi } from '@backstage/core-plugin-api';
import {
  getRuntimeLogs,
  getEnvironments,
  calculateTimeRange,
} from '../../api/runtimeLogs';
import {
  LogEntry,
  Environment,
  RuntimeLogsFilters,
  RuntimeLogsPagination,
} from './types';

export function useEnvironments() {
  const { entity } = useEntity();
  const discovery = useApi(discoveryApiRef);
  const identity = useApi(identityApiRef);
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchEnvironments = async () => {
      try {
        setLoading(true);
        setError(null);
        const envs = await getEnvironments(entity, discovery, identity);
        setEnvironments(envs);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to fetch environments',
        );
      } finally {
        setLoading(false);
      }
    };

    fetchEnvironments();
  }, [entity, discovery, identity]);

  return { environments, loading, error };
}

export function useRuntimeLogs(
  filters: RuntimeLogsFilters,
  pagination: RuntimeLogsPagination,
) {
  const { entity } = useEntity();
  const discovery = useApi(discoveryApiRef);
  const identity = useApi(identityApiRef);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [totalCount, setTotalCount] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const loadingRef = useRef(false);

  const fetchLogs = useCallback(
    async (reset: boolean = false) => {
      if (loadingRef.current || !filters.environmentId) {
        return;
      }

      try {
        loadingRef.current = true;
        setLoading(true);
        setError(null);

        const { startTime, endTime } = calculateTimeRange(filters.timeRange);
        const currentOffset = reset ? 0 : pagination.offset;

        const response = await getRuntimeLogs(entity, discovery, identity, {
          environmentId:
            typeof filters.environmentId === 'string'
              ? filters.environmentId.toLowerCase()
              : '',
          logLevels: filters.logLevel,
          startTime,
          endTime,
          limit: pagination.limit,
          offset: currentOffset,
        });

        if (reset) {
          setLogs(response.logs);
        } else {
          setLogs(prev => [...prev, ...response.logs]);
        }

        setTotalCount(response.totalCount);
        setHasMore(response.logs.length === pagination.limit);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch logs');
      } finally {
        loadingRef.current = false;
        setLoading(false)
      }
    },
    [entity, discovery, identity, filters, pagination], // TODO: Verify this array
  );

  const loadMore = useCallback(() => {
    if (!loading && hasMore) {
      fetchLogs(false);
    }
  }, [fetchLogs, hasMore, loading]);

  const refresh = useCallback(() => {
    setLogs([]);
    fetchLogs(true);
  }, [fetchLogs]);

  return {
    logs,
    loading,
    error,
    totalCount,
    hasMore,
    fetchLogs,
    loadMore,
    refresh,
  };
}

export function useInfiniteScroll(
  callback: () => void,
  hasMore: boolean,
  loading: boolean,
) {
  const [isFetching, setIsFetching] = useState(false);
  const observerRef = useRef<IntersectionObserver | null>(null);
  const loadingRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (loading || !hasMore) {
      return () => {};
    }

    if (observerRef.current) {
      observerRef.current.disconnect();
    }

    observerRef.current = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting && !isFetching) {
          setIsFetching(true);
          callback();
        }
      },
      {
        threshold: 0.1,
        rootMargin: '200px',
      },
    );

    if (loadingRef.current) {
      observerRef.current.observe(loadingRef.current);
    }

    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect();
      }
    };
  }, [callback, hasMore, loading, isFetching]);

  useEffect(() => {
    if (!loading) {
      setIsFetching(false);
    }
  }, [loading]);

  return { loadingRef };
}

export function useFilters() {
  const [filters, setFilters] = useState<RuntimeLogsFilters>({
    logLevel: [],
    environmentId: '',
    timeRange: '1h',
  });

  const updateFilters = useCallback(
    (newFilters: Partial<RuntimeLogsFilters>) => {
      setFilters(prev => ({ ...prev, ...newFilters }));
    },
    [],
  );

  const resetFilters = useCallback(() => {
    setFilters({
      logLevel: [],
      environmentId: '',
      timeRange: '1h',
    });
  }, []);

  return { filters, updateFilters, resetFilters };
}
