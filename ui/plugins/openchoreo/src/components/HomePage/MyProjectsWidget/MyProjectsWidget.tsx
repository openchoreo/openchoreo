import React, { useCallback, useEffect, useState } from 'react';
import {
  useApi,
  discoveryApiRef,
  identityApiRef,
} from '@backstage/core-plugin-api';
import { catalogApiRef } from '@backstage/plugin-catalog-react';
import { SummaryWidgetWrapper } from '../../SummaryWidgetWrapper';
import FolderIcon from '@material-ui/icons/Folder';
import { CHOREO_ANNOTATIONS } from '@openchoreo/backstage-plugin-api';
import { fetchTotalBindingsCount } from '../../../api/dashboard';

/**
 * A widget that displays project metrics for developers
 */
export const MyProjectsWidget: React.FC = () => {
  const [componentsCount, setComponentsCount] = useState<number>(0);
  const [projectsCount, setProjectsCount] = useState<number>(0);
  const [componentBindingsCount, setComponentBindingsCount] =
    useState<number>(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const catalogApi = useApi(catalogApiRef);
  const discoveryApi = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      // Fetch components and projects (systems) from catalog
      const [componentsResponse, systemsResponse] = await Promise.all([
        catalogApi.getEntities({
          filter: { kind: 'Component' },
        }),
        catalogApi.getEntities({
          filter: { kind: 'System' },
        }),
      ]);

      setComponentsCount(componentsResponse.items.length);
      setProjectsCount(systemsResponse.items.length);

      // Extract component info for fetching bindings
      const componentInfoList = componentsResponse.items
        .map(component => {
          const annotations = component.metadata.annotations || {};
          const orgName = annotations[CHOREO_ANNOTATIONS.ORGANIZATION];
          const projectName = annotations[CHOREO_ANNOTATIONS.PROJECT];
          const componentName = annotations[CHOREO_ANNOTATIONS.COMPONENT];

          if (orgName && projectName && componentName) {
            return { orgName, projectName, componentName };
          }
          return null;
        })
        .filter(
          (
            info,
          ): info is {
            orgName: string;
            projectName: string;
            componentName: string;
          } => info !== null,
        );

      // Fetch total bindings count from backend
      if (componentInfoList.length > 0) {
        const totalBindings = await fetchTotalBindingsCount(
          componentInfoList,
          discoveryApi,
          identityApi,
        );
        setComponentBindingsCount(totalBindings);
      } else {
        setComponentBindingsCount(0);
      }
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to fetch project data',
      );
      setComponentsCount(0);
      setProjectsCount(0);
      setComponentBindingsCount(0);
    } finally {
      setLoading(false);
    }
  }, [catalogApi, discoveryApi, identityApi]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return (
    <SummaryWidgetWrapper
      icon={<FolderIcon fontSize="inherit" />}
      title="My Projects"
      metrics={[
        { label: 'Projects:', value: projectsCount },
        { label: 'Components:', value: componentsCount },
        { label: 'Active Deployments:', value: componentBindingsCount },
      ]}
      loading={loading}
      errorMessage={error || undefined}
    />
  );
};
