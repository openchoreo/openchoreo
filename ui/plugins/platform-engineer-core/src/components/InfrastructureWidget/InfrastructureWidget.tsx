import React, { useCallback, useEffect, useState } from 'react';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { catalogApiRef } from '@backstage/plugin-catalog-react';
import { fetchDataplanesWithEnvironmentsAndComponents } from '../../api/dataplanesWithEnvironmentsAndComponents';
import { DataPlaneWithEnvironments } from '../../types';
import { SummaryWidgetWrapper } from '../SummaryWidgetWrapper';
import InfrastructureIcon from '@material-ui/icons/Storage';

/**
 * A standalone infrastructure widget for the homepage that handles its own data fetching
 */
export const InfrastructureWidget: React.FC = () => {
  const [dataplanesWithEnvironments, setDataplanesWithEnvironments] = useState<
    DataPlaneWithEnvironments[]
  >([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const discovery = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);
  const catalogApi = useApi(catalogApiRef);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      const dataplanesData = await fetchDataplanesWithEnvironmentsAndComponents(
        discovery,
        identityApi,
        catalogApi,
      );

      setDataplanesWithEnvironments(dataplanesData);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to fetch infrastructure data',
      );
      setDataplanesWithEnvironments([]);
    } finally {
      setLoading(false);
    }
  }, [discovery, identityApi, catalogApi]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Calculate metrics from the data
  const totalDataplanes = dataplanesWithEnvironments.length;
  const totalEnvironments = dataplanesWithEnvironments.reduce(
    (total, dp) => total + dp.environments.length,
    0,
  );
  const healthyComponents = dataplanesWithEnvironments.reduce(
    (total, dp) =>
      total +
      dp.environments.reduce((envTotal, env) => {
        return envTotal + (env.componentCount ?? 0);
      }, 0),
    0,
  );

  return (
    <SummaryWidgetWrapper
      icon={<InfrastructureIcon fontSize="inherit" />}
      title="Infrastructure"
      metrics={[
        { label: 'Data planes connected:', value: totalDataplanes },
        { label: 'Environments:', value: totalEnvironments },
        { label: 'Healthy workloads:', value: healthyComponents },
      ]}
      loading={loading}
      errorMessage={error || undefined}
    />
  );
};
