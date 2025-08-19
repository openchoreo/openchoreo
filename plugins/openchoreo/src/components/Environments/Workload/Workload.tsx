import {
  Drawer,
  Button,
  Typography,
  Box,
  useTheme,
  IconButton,
  CircularProgress,
} from '@material-ui/core';
import { useEffect, useState } from 'react';
import { WorkloadEditor } from './WorkloadEditor';
import CloseIcon from '@material-ui/icons/Close';
import { ModelsWorkload, ModelsBuild } from '@internal/plugin-openchoreo-api';
import { applyWorkload, fetchWorkloadInfo } from '../../../api/workloadInfo';
import { useEntity } from '@backstage/plugin-catalog-react';
import { useApi } from '@backstage/core-plugin-api';
import { discoveryApiRef } from '@backstage/core-plugin-api';
import { identityApiRef } from '@backstage/core-plugin-api';
import { Alert } from '@material-ui/lab';
import { WorkloadProvider } from './WorkloadContext';

export function Workload({
  onDeployed,
  isWorking,
}: {
  onDeployed: () => Promise<void>;
  isWorking: boolean;
}) {
  const discovery = useApi(discoveryApiRef);
  const identity = useApi(identityApiRef);
  const { entity } = useEntity();
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const [workloadSpec, setWorkloadSpec] = useState<ModelsWorkload | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isDeploying, setIsDeploying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [builds, setBuilds] = useState<ModelsBuild[]>([]);

  useEffect(() => {
    const fetchWorkload = async () => {
      try {
        setIsLoading(true);
        const response = await fetchWorkloadInfo(entity, discovery, identity);
        setWorkloadSpec(response);
      } catch (e) {
        setError('Failed to fetch workload info');
      }
      setIsLoading(false);
    };
    fetchWorkload();
    return () => {
      setWorkloadSpec(null);
      setError(null);
    };
  }, [entity, discovery, identity]);

  useEffect(() => {
    const fetchBuilds = async () => {
      try {
        const componentName = entity.metadata.name;
        const projectName =
          entity.metadata.annotations?.['openchoreo.io/project'];
        const organizationName =
          entity.metadata.annotations?.['openchoreo.io/organization'];

        // Get authentication token
        const { token } = await identity.getCredentials();

        // Now fetch the builds
        const baseUrl = await discovery.getBaseUrl('openchoreo');
        if (projectName && organizationName && componentName) {
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
        }
      } catch (err) {
        // Handle error silently or set an error state if needed
        setBuilds([]);
      }
    };
    fetchBuilds();
  }, [entity.metadata.name, entity.metadata.annotations, identity, discovery]);

  const toggleDrawer = () => {
    setOpen(!open);
  };

  const handleDeploy = async () => {
    if (!workloadSpec) {
      return;
    }
    setIsDeploying(true);
    try {
      await applyWorkload(entity, discovery, identity, workloadSpec);
      setTimeout(async () => {
        await onDeployed();
        setOpen(false);
      }, 3000);
    } catch (e) {
      setIsDeploying(false);
      throw new Error('Failed to deploy workload');
    }
  };

  const enableDeploy =
    (workloadSpec || builds.some(build => build.image)) && !isLoading;
  const hasBuils = builds.length > 0 || workloadSpec;

  return (
    <>
      <Box
        display="flex"
        justifyContent="space-between"
        flexDirection="column"
        gridGap={8}
      >
        <Box
          display="flex"
          justifyContent="space-between"
          alignItems="center"
          p={2}
        >
          {isLoading && !error && <CircularProgress />}
        </Box>
        {!enableDeploy && (
          <Alert severity={!hasBuils ? 'error' : 'warning'}>
            {!hasBuils ? error : 'Build your application first.'}
          </Alert>
        )}
        <Button
          onClick={toggleDrawer}
          disabled={!enableDeploy || isDeploying || isLoading || isWorking}
          variant="contained"
          color="primary"
          size="small"
        >
          Configure & Deploy
        </Button>
      </Box>

      <Drawer open={open} onClose={toggleDrawer} anchor="right">
        <Box
          bgcolor={theme.palette.grey[200]}
          minWidth={theme.spacing(80)}
          display="flex"
          flexDirection="column"
          height="100%"
          overflow="hidden"
        >
          <Box
            display="flex"
            justifyContent="space-between"
            alignItems="center"
            p={2}
          >
            <Typography variant="h6" component="h4">
              Configure Workload
            </Typography>
            <IconButton onClick={toggleDrawer} color="default">
              <CloseIcon />
            </IconButton>
          </Box>
          <Box borderBottom={1} borderColor="grey.400" />
          <Box flex={1} paddingBottom={2} overflow="auto" bgcolor="grey.200">
            <WorkloadProvider
              builds={builds}
              workloadSpec={workloadSpec}
              setWorkloadSpec={setWorkloadSpec}
              isDeploying={isDeploying || isLoading || isWorking}
            >
              <WorkloadEditor entity={entity} onDeploy={handleDeploy} />
            </WorkloadProvider>
          </Box>
        </Box>
      </Drawer>
    </>
  );
}
