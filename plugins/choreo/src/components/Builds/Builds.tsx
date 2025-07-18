import React, { useEffect, useState } from 'react';
import { useApi, discoveryApiRef, identityApiRef } from '@backstage/core-plugin-api';
import { useEntity } from '@backstage/plugin-catalog-react';
import { catalogApiRef } from '@backstage/plugin-catalog-react';
import {
  Progress,
  ResponseErrorPanel,
  Table,
  TableColumn,
  StatusOK,
  StatusError,
  StatusPending,
  StatusRunning,
} from '@backstage/core-components';
import { Chip, Typography } from '@material-ui/core';
import type { ModelsBuild } from '@internal/plugin-openchoreo-api';

const BuildStatusComponent = ({ status }: { status?: string }) => {
  if (!status) {
    return <StatusPending>Unknown</StatusPending>;
  }

  const normalizedStatus = status.toLowerCase();
  
  if (normalizedStatus.includes('succeed') || normalizedStatus.includes('success')) {
    return <StatusOK>Success</StatusOK>;
  }
  
  if (normalizedStatus.includes('fail') || normalizedStatus.includes('error')) {
    return <StatusError>Failed</StatusError>;
  }
  
  if (normalizedStatus.includes('running') || normalizedStatus.includes('progress')) {
    return <StatusRunning>Running</StatusRunning>;
  }
  
  return <StatusPending>{status}</StatusPending>;
};

const formatDate = (dateString: string) => {
  return new Date(dateString).toLocaleString();
};

const formatDuration = (buildStatus?: any) => {
  if (!buildStatus?.startTime) return 'N/A';
  
  const startTime = new Date(buildStatus.startTime);
  const endTime = buildStatus.completionTime ? new Date(buildStatus.completionTime) : new Date();
  
  const durationMs = endTime.getTime() - startTime.getTime();
  const durationMinutes = Math.floor(durationMs / 60000);
  const durationSeconds = Math.floor((durationMs % 60000) / 1000);
  
  if (durationMinutes > 0) {
    return `${durationMinutes}m ${durationSeconds}s`;
  }
  return `${durationSeconds}s`;
};

export const Builds = () => {
  const { entity } = useEntity();
  const discoveryApi = useApi(discoveryApiRef);
  const catalogApi = useApi(catalogApiRef);
  const identityApi = useApi(identityApiRef);
  const [builds, setBuilds] = useState<ModelsBuild[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchBuilds = async () => {
      if (!entity.metadata.name) {
        setError(new Error('Component name not found'));
        setLoading(false);
        return;
      }

      const componentName = entity.metadata.name;
      
      // Get project name from spec.system
      const systemValue = entity.spec?.system;
      if (!systemValue) {
        setError(new Error('Project name not found in spec.system'));
        setLoading(false);
        return;
      }

      // Convert system value to string (it could be string or object)
      const projectName = typeof systemValue === 'string' ? systemValue : String(systemValue);

      try {
        // Fetch the project entity to get the organization
        const projectEntityRef = `system:default/${projectName}`;
        const projectEntity = await catalogApi.getEntityByRef(projectEntityRef);
        
        if (!projectEntity) {
          throw new Error(`Project entity not found: ${projectEntityRef}`);
        }

        // Get organization from the project entity's spec.domain or annotations
        let organizationValue = projectEntity.spec?.domain;
        if (!organizationValue) {
          organizationValue = projectEntity.metadata.annotations?.['openchoreo.io/organization'];
        }
        
        if (!organizationValue) {
          throw new Error(`Organization name not found in project entity: ${projectEntityRef}`);
        }

        // Convert organization value to string (it could be string or object)
        const organizationName = typeof organizationValue === 'string' ? organizationValue : String(organizationValue);

        // Get authentication token
        const { token } = await identityApi.getCredentials();

        // Now fetch the builds
        const baseUrl = await discoveryApi.getBaseUrl('choreo');
        const response = await fetch(
          `${baseUrl}/builds?componentName=${encodeURIComponent(componentName)}&projectName=${encodeURIComponent(projectName)}&organizationName=${encodeURIComponent(organizationName)}`,
          {
            headers: {
              Authorization: `Bearer ${token}`,
            },
          }
        );

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const buildsData = await response.json();
        setBuilds(buildsData);
      } catch (err) {
        setError(err as Error);
      } finally {
        setLoading(false);
      }
    };

    fetchBuilds();
  }, [entity, discoveryApi, catalogApi, identityApi]);

  if (loading) {
    return <Progress />;
  }

  if (error) {
    return <ResponseErrorPanel error={error} />;
  }

  const columns: TableColumn[] = [
    {
      title: 'Build Name',
      field: 'name',
      highlight: true,
    },
    {
      title: 'Status',
      field: 'status',
      render: (row: any) => <BuildStatusComponent status={(row as ModelsBuild).status} />,
    },
    {
      title: 'Branch',
      field: 'branch',
      render: (row: any) => {
        const build = row as ModelsBuild;
        return build.branch ? <Chip label={build.branch} size="small" /> : 'N/A';
      },
    },
    {
      title: 'Commit',
      field: 'commit',
      render: (row: any) => {
        const build = row as ModelsBuild;
        return build.commit ? (
          <Typography variant="body2" style={{ fontFamily: 'monospace' }}>
            {build.commit.substring(0, 8)}
          </Typography>
        ) : 'N/A';
      },
    },
    {
      title: 'Image',
      field: 'image',
      render: (row: any) => {
        const build = row as ModelsBuild;
        return build.image ? (
          <Typography variant="body2" style={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
            {build.image}
          </Typography>
        ) : 'N/A';
      },
    },
    {
      title: 'Duration',
      field: 'duration',
      render: (row: any) => formatDuration((row as ModelsBuild).buildStatus),
    },
    {
      title: 'Created',
      field: 'createdAt',
      render: (row: any) => formatDate((row as ModelsBuild).createdAt),
    },
  ];

  return (
    <Table
      title="Component Builds"
      options={{ search: true, paging: true }}
      columns={columns}
      data={builds}
      emptyContent={
        <Typography variant="body1">
          No builds found for this component.
        </Typography>
      }
    />
  );
};