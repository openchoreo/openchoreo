import { useEffect, useState, useCallback } from 'react';
import {
  useApi,
  discoveryApiRef,
  identityApiRef,
} from '@backstage/core-plugin-api';
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
import {
  Typography,
  Button,
  Box,
  Paper,
  Link,
  IconButton,
} from '@material-ui/core';
import GitHub from '@material-ui/icons/GitHub';
import CallSplit from '@material-ui/icons/CallSplit';
import FileCopy from '@material-ui/icons/FileCopy';
import Refresh from '@material-ui/icons/Refresh';
import { BuildLogs } from './BuildLogs';
import type {
  ModelsBuild,
  ModelsCompleteComponent,
} from '@openchoreo/backstage-plugin-api';
import { formatRelativeTime } from '../../utils/timeUtils';

const BuildStatusComponent = ({ status }: { status?: string }) => {
  if (!status) {
    return <StatusPending>Unknown</StatusPending>;
  }

  const normalizedStatus = status.toLowerCase();

  if (
    normalizedStatus.includes('succeed') ||
    normalizedStatus.includes('success')
  ) {
    return <StatusOK>Success</StatusOK>;
  }

  if (normalizedStatus.includes('fail') || normalizedStatus.includes('error')) {
    return <StatusError>Failed</StatusError>;
  }

  if (
    normalizedStatus.includes('running') ||
    normalizedStatus.includes('progress')
  ) {
    return <StatusRunning>Running</StatusRunning>;
  }

  return <StatusPending>{status}</StatusPending>;
};

export const Builds = () => {
  const { entity } = useEntity();
  const discoveryApi = useApi(discoveryApiRef);
  const catalogApi = useApi(catalogApiRef);
  const identityApi = useApi(identityApiRef);
  const [builds, setBuilds] = useState<ModelsBuild[]>([]);
  const [componentDetails, setComponentDetails] =
    useState<ModelsCompleteComponent | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [triggeringBuild, setTriggeringBuild] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [selectedBuild, setSelectedBuild] = useState<ModelsBuild | null>(null);

  const getEntityDetails = useCallback(async () => {
    if (!entity.metadata.name) {
      throw new Error('Component name not found');
    }

    const componentName = entity.metadata.name;

    // Get project name from spec.system
    const systemValue = entity.spec?.system;
    if (!systemValue) {
      throw new Error('Project name not found in spec.system');
    }

    // Convert system value to string (it could be string or object)
    const projectName =
      typeof systemValue === 'string' ? systemValue : String(systemValue);

    // Fetch the project entity to get the organization
    const projectEntityRef = `system:default/${projectName}`;
    const projectEntity = await catalogApi.getEntityByRef(projectEntityRef);

    if (!projectEntity) {
      throw new Error(`Project entity not found: ${projectEntityRef}`);
    }

    // Get organization from the project entity's spec.domain or annotations
    let organizationValue = projectEntity.spec?.domain;
    if (!organizationValue) {
      organizationValue =
        projectEntity.metadata.annotations?.['openchoreo.io/organization'];
    }

    if (!organizationValue) {
      throw new Error(
        `Organization name not found in project entity: ${projectEntityRef}`,
      );
    }

    // Convert organization value to string (it could be string or object)
    const organizationName =
      typeof organizationValue === 'string'
        ? organizationValue
        : String(organizationValue);

    return { componentName, projectName, organizationName };
  }, [entity, catalogApi]);

  const fetchComponentDetails = useCallback(async () => {
    try {
      const { componentName, projectName, organizationName } =
        await getEntityDetails();

      // Get authentication token
      const { token } = await identityApi.getCredentials();

      // Fetch component details
      const baseUrl = await discoveryApi.getBaseUrl('openchoreo');
      const componentResponse = await fetch(
        `${baseUrl}/component?componentName=${encodeURIComponent(
          componentName,
        )}&projectName=${encodeURIComponent(
          projectName,
        )}&organizationName=${encodeURIComponent(organizationName)}`,
        {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        },
      );

      if (!componentResponse.ok) {
        throw new Error(
          `HTTP ${componentResponse.status}: ${componentResponse.statusText}`,
        );
      }

      const componentData = await componentResponse.json();
      setComponentDetails(componentData);
    } catch (err) {
      setError(err as Error);
    }
  }, [discoveryApi, identityApi, getEntityDetails]);

  const fetchBuilds = useCallback(async () => {
    try {
      const { componentName, projectName, organizationName } =
        await getEntityDetails();

      // Get authentication token
      const { token } = await identityApi.getCredentials();

      // Now fetch the builds
      const baseUrl = await discoveryApi.getBaseUrl('openchoreo');
      const response = await fetch(
        `${baseUrl}/builds?componentName=${encodeURIComponent(
          componentName,
        )}&projectName=${encodeURIComponent(
          projectName,
        )}&organizationName=${encodeURIComponent(organizationName)}`,
        {
          headers: {
            Authorization: `Bearer ${token}`,
          },
        },
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
  }, [discoveryApi, identityApi, getEntityDetails]);

  const triggerBuild = async () => {
    setTriggeringBuild(true);
    try {
      const { componentName, projectName, organizationName } =
        await getEntityDetails();

      // Get authentication token
      const { token } = await identityApi.getCredentials();

      // Trigger the build
      const baseUrl = await discoveryApi.getBaseUrl('openchoreo');
      const response = await fetch(`${baseUrl}/builds`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          componentName,
          projectName,
          organizationName,
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      // Refresh the builds list
      await fetchBuilds();
    } catch (err) {
      setError(err as Error);
    } finally {
      setTriggeringBuild(false);
    }
  };

  const refreshBuilds = async () => {
    setRefreshing(true);
    try {
      await fetchBuilds();
    } catch (err) {
      setError(err as Error);
    } finally {
      setRefreshing(false);
    }
  };

  useEffect(() => {
    let ignore = false;
    const fetchData = async () => {
      await Promise.all([fetchComponentDetails(), fetchBuilds()]);
    };
    if (!ignore) fetchData();

    return () => {
      ignore = true;
    };
  }, [
    entity,
    discoveryApi,
    catalogApi,
    identityApi,
    fetchComponentDetails,
    fetchBuilds,
  ]);

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
      render: (row: any) => (
        <BuildStatusComponent status={(row as ModelsBuild).status} />
      ),
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
        ) : (
          'N/A'
        );
      },
    },
    {
      title: 'Time',
      field: 'time',
      render: (row: any) => formatRelativeTime((row as ModelsBuild).createdAt),
    },
  ];

  const getRepositoryUrl = (component: ModelsCompleteComponent) => {
    const baseUrl = component.buildConfig?.repoUrl || component.repositoryUrl;
    const branch = component.buildConfig?.repoBranch || component.branch;
    const componentPath = component.buildConfig?.componentPath;

    if (!componentPath) {
      return baseUrl;
    }

    const separator = baseUrl.endsWith('/') ? '' : '/';
    return `${baseUrl}${separator}tree/${branch}/${componentPath}`;
  };

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch (err) {
      // Fallback for older browsers
      const textArea = document.createElement('textarea');
      textArea.value = text;
      document.body.appendChild(textArea);
      textArea.select();
      document.execCommand('copy');
      document.body.removeChild(textArea);
    }
  };

  return (
    <Box>
      {componentDetails && (
        <Paper style={{ padding: '16px', marginBottom: '16px' }}>
          <Box>
            <Typography variant="body2" color="textSecondary" gutterBottom>
              {componentDetails.type}
            </Typography>
            <Typography variant="h6" style={{ marginBottom: '16px' }}>
              {componentDetails.displayName || componentDetails.name}
            </Typography>
            <Typography variant="body2" color="textSecondary" gutterBottom>
              Source
            </Typography>
            <Box
              display="flex"
              alignItems="center"
              style={{ marginBottom: '8px' }}
            >
              <GitHub
                style={{ fontSize: '16px', marginRight: '6px', color: '#666' }}
              />
              <Link
                href={getRepositoryUrl(componentDetails)}
                target="_blank"
                rel="noopener noreferrer"
                style={{ fontSize: '13px' }}
              >
                {getRepositoryUrl(componentDetails)}
              </Link>
              <IconButton
                size="small"
                onClick={() =>
                  copyToClipboard(getRepositoryUrl(componentDetails))
                }
                style={{ marginLeft: '8px', padding: '4px' }}
                title="Copy URL to clipboard"
              >
                <FileCopy style={{ fontSize: '14px', color: '#666' }} />
              </IconButton>
            </Box>
            <Box
              display="flex"
              alignItems="center"
              justifyContent="space-between"
              style={{ marginBottom: '8px' }}
            >
              <Box display="flex" alignItems="center">
                <CallSplit
                  style={{
                    fontSize: '16px',
                    marginRight: '6px',
                    color: '#666',
                  }}
                />
                <Typography variant="body2">
                  {componentDetails.buildConfig?.repoBranch ||
                    componentDetails.branch}
                </Typography>
              </Box>
              <Box display="flex">
                <Button
                  variant="contained"
                  color="primary"
                  size="small"
                  onClick={triggerBuild}
                  disabled={triggeringBuild}
                  style={{ marginRight: '12px' }}
                >
                  {triggeringBuild ? 'Building...' : 'Build Latest'}
                </Button>
                <Button variant="outlined" size="small" onClick={() => {}}>
                  Show Commits
                </Button>
              </Box>
            </Box>
          </Box>
        </Paper>
      )}
      <Table
        title={
          <Box display="flex" alignItems="center">
            <Typography variant="h6" component="span">
              Builds
            </Typography>
            <IconButton
              size="small"
              onClick={refreshBuilds}
              disabled={refreshing || loading}
              style={{ marginLeft: '8px' }}
              title={refreshing ? 'Refreshing...' : 'Refresh builds'}
            >
              <Refresh style={{ fontSize: '18px' }} />
            </IconButton>
          </Box>
        }
        options={{
          search: true,
          paging: true,
          sorting: true,
        }}
        columns={columns}
        data={builds.sort(
          (a, b) =>
            new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
        )}
        onRowClick={(_, rowData) => {
          setSelectedBuild(rowData as ModelsBuild);
          setDrawerOpen(true);
        }}
        emptyContent={
          <Typography variant="body1">
            No builds found for this component.
          </Typography>
        }
      />
      <BuildLogs
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        build={selectedBuild}
      />
    </Box>
  );
};
