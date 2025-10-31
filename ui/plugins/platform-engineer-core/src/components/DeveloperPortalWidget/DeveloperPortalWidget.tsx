import React, { useCallback, useEffect, useState } from 'react';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { catalogApiRef } from '@backstage/plugin-catalog-react';
import { fetchDistinctDeployedComponentsCount } from '../../api/distinctDeployedComponents';
import { SummaryWidgetWrapper } from '../SummaryWidgetWrapper';
import DeveloperModeIcon from '@material-ui/icons/DeveloperMode';

/**
 * A standalone developer portal widget for the homepage that handles its own data fetching
 */
export const DeveloperPortalWidget: React.FC = () => {
  const [distinctDeployedComponentsCount, setDistinctDeployedComponentsCount] =
    useState<number>(0);
  const [projectsCount, setProjectsCount] = useState<number>(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const discovery = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);
  const catalogApi = useApi(catalogApiRef);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      // Fetch distinct deployed components count and projects (systems)
      const [distinctCount, systemsResponse] = await Promise.all([
        fetchDistinctDeployedComponentsCount(
          discovery,
          identityApi,
          catalogApi,
        ),
        // Projects map to System in Backstage catalog
        catalogApi.getEntities({
          filter: { kind: 'System' },
        }),
      ]);

      setDistinctDeployedComponentsCount(distinctCount);
      setProjectsCount(systemsResponse.items.length);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to fetch developer portal data',
      );
      setDistinctDeployedComponentsCount(0);
      setProjectsCount(0);
    } finally {
      setLoading(false);
    }
  }, [discovery, identityApi, catalogApi]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return (
    <SummaryWidgetWrapper
      icon={<DeveloperModeIcon fontSize="inherit" />}
      title="Developer Portal"
      metrics={[
        { label: 'Projects created:', value: projectsCount },
        {
          label: 'Components deployed:',
          value: distinctDeployedComponentsCount,
        },
      ]}
      loading={loading}
      errorMessage={error || undefined}
    />
  );
};
