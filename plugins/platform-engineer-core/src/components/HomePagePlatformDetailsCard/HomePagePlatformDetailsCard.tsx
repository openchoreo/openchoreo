import React, { useCallback, useEffect, useState } from 'react';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { catalogApiRef } from '@backstage/plugin-catalog-react';
import { PlatformDetailsCard } from '../PlatformDetailsCard';
import { fetchDataplanesWithEnvironmentsAndComponents } from '../../api/dataplanesWithEnvironmentsAndComponents';
import { DataPlaneWithEnvironments } from '../../types';
import { Box, CircularProgress, Typography } from '@material-ui/core';

/**
 * A standalone platform details card for the homepage that handles its own data fetching
 */
export const HomePagePlatformDetailsCard: React.FC = () => {
  const [dataplanesWithEnvironments, setDataplanesWithEnvironments] = useState<
    DataPlaneWithEnvironments[]
  >([]);
  const [expandedDataplanes, setExpandedDataplanes] = useState<Set<string>>(
    new Set(),
  );
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
      setExpandedDataplanes(
        dataplanesData.length === 1
          ? new Set([dataplanesData[0].name])
          : new Set(),
      );
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to fetch platform details',
      );
      setDataplanesWithEnvironments([]);
    } finally {
      setLoading(false);
    }
  }, [discovery, identityApi, catalogApi]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const toggleDataplaneExpansion = (dataplaneName: string) => {
    setExpandedDataplanes(prev => {
      const newSet = new Set(prev);
      if (newSet.has(dataplaneName)) {
        newSet.delete(dataplaneName);
      } else {
        newSet.add(dataplaneName);
      }
      return newSet;
    });
  };

  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight={120}
      >
        <CircularProgress size={24} />
      </Box>
    );
  }

  if (error) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight={120}
      >
        <Typography variant="body2" color="error">
          Failed to load platform details
        </Typography>
      </Box>
    );
  }

  return (
    <PlatformDetailsCard
      dataplanesWithEnvironments={dataplanesWithEnvironments}
      expandedDataplanes={expandedDataplanes}
      onToggleDataplaneExpansion={toggleDataplaneExpansion}
    />
  );
};
