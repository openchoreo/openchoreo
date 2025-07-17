import { useEffect, useState } from 'react';
import { useEntity } from '@backstage/plugin-catalog-react';
import {
  Content,
  ContentHeader,
  Page,
  TabbedLayout,
} from '@backstage/core-components';
import { Grid, Card, CardContent, Typography, Box, Button } from '@material-ui/core';
import { StatusOK, StatusError } from '@backstage/core-components';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { fetchEnvironmentInfo, promoteToEnvironment } from '../../api/getEnvironmentInfo';

interface Environment {
  name: string;
  deployment: {
    status: 'success' | 'failed';
    lastDeployed: string;
  };
  endpoint: {
    url: string;
    status: 'active' | 'inactive';
  };
}

const promotionPaths = [
  {
    sourceEnvironmentRef: 'Development',
    targetEnvironmentRefs: [
      { name: 'Staging' },
    ],
  },
  {
    sourceEnvironmentRef: 'Staging',
    targetEnvironmentRefs: [
      { name: 'Production' },
    ],
  },
];

const environmentOrder = ['Development', 'Staging', 'Production'];

export const Environments = () => {
  const { entity } = useEntity();
  const [environments, setEnvironmentsData] = useState<Environment[]>([]);
  const discovery = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);
  useEffect(() => {
    const fetchData = async () => {
      const data = await fetchEnvironmentInfo(entity, discovery, identityApi);
      setEnvironmentsData(data as Environment[]);
    };

    fetchData();
  }, []);
  // TODO Add loading state

  return (
    <Page themeId="tool">
      {/* <Header title="Deployments" type="tool">
        <HeaderLabel label="Component" value={entity.metadata.name} />
      </Header> */}
      <Content>
        <ContentHeader title="Environments" />
        <TabbedLayout>
          {[...environments]
            .sort((a, b) =>
              environmentOrder.indexOf(a.name) - environmentOrder.indexOf(b.name),
            )
            .map(env => (
              // {environments.map(env => (
              <TabbedLayout.Route key={env.name} path={env.name} title={env.name}>
                <Grid container spacing={3}>
                  <Grid item xs={12} md={6}>
                    <Card>
                      <CardContent>
                        <Typography variant="h6">Deployment Status</Typography>
                        <Box display="flex" alignItems="center" mt={2}>
                          {env.deployment.status === 'success' ? (
                            <StatusOK />
                          ) : (
                            <StatusError />
                          )}
                          <Typography variant="body1" style={{ marginLeft: 8 }}>
                            {env.deployment.status === 'success'
                              ? 'Successful'
                              : 'Failed'}
                          </Typography>
                        </Box>
                        <Typography variant="body2" color="textSecondary">
                          Last deployed:{' '}
                          {new Date(env.deployment.lastDeployed).toLocaleString()}
                        </Typography>
                      </CardContent>
                    </Card>
                  </Grid>
                  <Grid item xs={12} md={6}>
                    <Card>
                      <CardContent>
                        <Typography variant="h6">Endpoint</Typography>
                        <Box display="flex" alignItems="center" mt={2}>
                          {env.endpoint.status === 'active' ? (
                            <StatusOK />
                          ) : (
                            <StatusError />
                          )}
                          <Typography variant="body1" style={{ marginLeft: 8 }}>
                            {env.endpoint.status === 'active'
                              ? 'Active'
                              : 'Inactive'}
                          </Typography>
                        </Box>
                        <Typography variant="body2" color="textSecondary">
                          {/* URL: {env.endpoint.url} */}
                          {/* URL:{' '} */}
                          {/* <a href={env.endpoint.url} target="_blank" rel="noopener noreferrer">
                          {env.endpoint.url}
                        </a> */}
                          <a
                            href={env.endpoint.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            style={{ color: '#1976d2', textDecoration: 'underline' }}
                          >
                            {env.endpoint.url}
                          </a>
                        </Typography>
                      </CardContent>
                    </Card>
                  </Grid>

                  {/* Promotion buttons section */}
                  {(() => {
                    const promotionPath = promotionPaths.find(
                      path => path.sourceEnvironmentRef === env.name
                    );
                    console.log('Promotion Path:', env.name);
                    // alert('Promotion Path:', env.name);
                    return promotionPath ? (
                      <Grid item xs={12}>
                        <Box mt={2}>
                          {/* <Typography variant="subtitle1">Promote to:</Typography> */}
                          <Box display="flex" mt={1}>
                            {promotionPath.targetEnvironmentRefs.map(target => (
                              <Button
                                key={target.name}
                                variant="contained"
                                color="primary"
                                onClick={async () => {
                                  try {
                                    const result = await promoteToEnvironment(entity, discovery, identityApi, target.name.toLowerCase());
                                    alert(`Promotion started: ${JSON.stringify(result)}`);
                                  } catch (err) {
                                    alert(`Error promoting: ${err}`);
                                  }
                                }}
                              >
                                Promote to {target.name.charAt(0).toUpperCase() + target.name.slice(1)}
                              </Button>
                            ))}
                          </Box>
                        </Box>
                      </Grid>
                    ) : null;
                  })()}

                </Grid>
              </TabbedLayout.Route>
            ))}
        </TabbedLayout>
      </Content>
    </Page>
  );
};
