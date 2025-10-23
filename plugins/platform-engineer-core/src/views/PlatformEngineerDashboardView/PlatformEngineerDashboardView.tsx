import { Content, Page, Header } from '@backstage/core-components';
import { Typography, Grid } from '@material-ui/core';
import {
  InfrastructureWidget,
  DeveloperPortalWidget,
  HomePagePlatformDetailsCard,
} from '../../components';
import { useStyles } from './styles';

export const PlatformEngineerDashboardView = () => {
  const classes = useStyles();
  // const [dataplanesWithEnvironments, setDataplanesWithEnvironments] = useState<DataPlaneWithEnvironments[]>([]);
  // const [loading, setLoading] = useState(true);
  // const [error, setError] = useState<string | null>(null);
  // const [expandedDataplanes, setExpandedDataplanes] = useState<Set<string>>(new Set());
  // const [distinctDeployedComponentsCount, setDistinctDeployedComponentsCount] = useState<number>(0);

  // const discovery = useApi(discoveryApiRef);
  // const identityApi = useApi(identityApiRef);
  // const catalogApi = useApi(catalogApiRef);

  // const fetchData = useCallback(async () => {
  //   try {
  //     setLoading(true);
  //     setError(null);

  //     // Fetch both dataplanes data and distinct deployed components count in parallel
  //     const [dataplanesData, distinctCount] = await Promise.all([
  //       fetchDataplanesWithEnvironmentsAndComponents(discovery, identityApi, catalogApi),
  //       fetchDistinctDeployedComponentsCount(discovery, identityApi, catalogApi),
  //     ]);

  //     setDataplanesWithEnvironments(dataplanesData);
  //     setDistinctDeployedComponentsCount(distinctCount);
  //   } catch (err) {
  //     setError(err instanceof Error ? err.message : 'Failed to fetch data');
  //   } finally {
  //     setLoading(false);
  //   }
  // }, [discovery, identityApi, catalogApi]);

  // useEffect(() => {
  //   fetchData();
  // }, [fetchData]);

  // if (loading) {
  //   return (
  //     <Page themeId="tool">
  //       <Header title="Platform Engineer Dashboard">
  //         <HeaderLabel label="Status" value={`Loading...`} />
  //       </Header>
  //       <Content className={classes.pageContent}>
  //         <Box className={classes.loadingContainer}>
  //           <CircularProgress size={60} style={{ color: '#667eea' }} />
  //           <Typography variant="h6" style={{ color: '#374151', fontWeight: 500 }}>
  //             Loading platform data...
  //           </Typography>
  //           <Typography variant="body2" color="textSecondary">
  //             Fetching dataplanes, environments, and component information
  //           </Typography>
  //         </Box>
  //       </Content>
  //     </Page>
  //   );
  // }

  // if (error) {
  //   return (
  //     <Page themeId="tool">
  //       <Header title="Platform Engineer Dashboard">
  //         <HeaderLabel label="Error" value="Failed to load" />
  //       </Header>
  //       <Content className={classes.pageContent}>
  //         <Box className={classes.errorContainer}>
  //           <Typography variant="h6" style={{ color: '#dc2626', fontWeight: 600, marginBottom: 8 }}>
  //             Failed to load platform data
  //           </Typography>
  //           <Typography variant="body1" color="textSecondary" style={{ marginBottom: 16, textAlign: 'center' }}>
  //             {error}
  //           </Typography>
  //           {/* <IconButton
  //             onClick={fetchData}
  //             style={{
  //               backgroundColor: '#667eea',
  //               color: 'white'
  //             }}
  //           >
  //             <Refresh />
  //           </IconButton> */}
  //         </Box>
  //       </Content>
  //     </Page>
  //   );
  // }

  // // Calculate summary metrics
  // const totalDataplanes = dataplanesWithEnvironments.length;
  // const totalEnvironments = dataplanesWithEnvironments.reduce((total, dp) => total + dp.environments.length, 0);
  // const healthyComponents = dataplanesWithEnvironments.reduce((total, dp) =>
  //   total + dp.environments.reduce((envTotal, env) => {
  //     // Assume components are healthy if they're deployed (have a count > 0)
  //     return envTotal + (env.componentCount ?? 0);
  //   }, 0), 0
  // );

  // // For projects, we'll estimate based on distinct deployed components (assuming multiple components per project)
  // const estimatedProjects = Math.ceil(distinctDeployedComponentsCount / 3); // Rough estimate

  return (
    <Page themeId="tool">
      <Header title="Platform Engineer Dashboard">
        {/* <IconButton onClick={fetchData} disabled={loading}>
          <Refresh />
        </IconButton> */}
      </Header>
      <Content className={classes.pageContent}>
        <Grid container spacing={3}>
          <Grid item xs={12} md={12}>
            <Typography variant="h3">Platform Overview</Typography>
          </Grid>
          <Grid item xs={12} md={4} sm={6}>
            <InfrastructureWidget />
          </Grid>
          <Grid item xs={12} md={4} sm={6}>
            <DeveloperPortalWidget />
          </Grid>
          <Grid item xs={12} md={12}>
            <HomePagePlatformDetailsCard />
          </Grid>
        </Grid>
      </Content>
    </Page>
  );
};
