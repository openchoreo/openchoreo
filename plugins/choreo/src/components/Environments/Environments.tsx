import { useEffect, useState } from 'react';
import { useEntity } from '@backstage/plugin-catalog-react';
import {
  Content,
  ContentHeader,
  Header,
  HeaderLabel,
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
      <Content>
        <Grid container spacing={3}>
          <Grid item xs={12} md={3}>
            <Card>
              {/* Make this card color different from the others */}
              <Box
                bgcolor="grey.200"
                color="text.primary"
                padding={1}
                borderRadius={1}
              >
                <CardContent>
                  <Typography variant="h6" component="h4">
                    Set up
                  </Typography>

                  <Box
                    borderBottom={1}
                    borderColor="divider"
                    marginBottom={2}
                    marginTop={1}
                  />
                  <Typography color="textSecondary">
                    View and manage deployment environments
                  </Typography>

                </CardContent>
              </Box>
            </Card>
          </Grid>
          {[...environments]
            .sort((a, b) =>
              environmentOrder.indexOf(a.name) - environmentOrder.indexOf(b.name),
            )
            .map(env => (
              <Grid item xs={12} md={3}>
                <Card>
                  <CardContent>
                    <Typography variant="h6" component="h4">
                      {env.name}
                    </Typography>
                    {/* add a line in the ui */}
                    <Box
                      borderBottom={1}
                      borderColor="divider"
                      marginBottom={2}
                      marginTop={1}
                    />
                    <Typography variant="body2" color="textSecondary">
                      Last deployed: {new Date(env.deployment.lastDeployed).toLocaleString()}
                    </Typography>
                    <Box display="flex" alignItems="center" mt={2} bgcolor="grey.200" padding={1} borderRadius={1}>
                      <Typography variant="body2" color="error">
                        Deployment Status: {env.deployment.status === 'success'
                          ? 'Active'
                          : 'Failed'}
                      </Typography>
                    </Box>
                    
                    <Box display="flex" alignItems="center" mt={2} >
                      <Typography variant="body2" color="textSecondary">
                        Image
                      </Typography>
                    </Box>
                    <Box display="flex" alignItems="center" mt={0} bgcolor="grey.200" padding={1} borderRadius={2} >  
                      <Typography variant="body2" color="textSecondary">
                        us-central1-docker.pkg.dev/google-samples/microservices-demo/adservice:v0.10.3
                      </Typography>
                    </Box>

                    <Box display="flex" alignItems="center" mt={2} >
                      <Typography variant="body2" color="textSecondary">
                        Endpoints
                      </Typography>
                    </Box>
                    <Box display="flex" alignItems="center" mt={0} >  
                      <Typography variant="body2" color="textSecondary">
                        <a
                          href={env.endpoint.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          style={{ color: '#1976d2', textDecoration: 'underline' }}
                        >
                          {env.endpoint.url}
                        </a>
                      </Typography>
                    </Box>

                    {/* Promotion buttons section */}
                    {(() => {
                      const promotionPath = promotionPaths.find(
                        path => path.sourceEnvironmentRef === env.name
                      );
                      console.log('Promotion Path:', env.name);
                      return promotionPath ? (
                        // <Grid item xs={12}>
                        <Box display="flex" mt={3}>
                          {promotionPath.targetEnvironmentRefs.map(target => (
                            <Button
                              key={target.name}
                              variant="contained"
                              color="primary"
                              size='small'
                              onClick={async () => {
                                try {
                                  const result = await promoteToEnvironment(entity, discovery, identityApi, target.name.toLowerCase());
                                  alert(`Promotion started: ${JSON.stringify(result)}`);
                                } catch (err) {
                                  alert(`Error promoting: ${err}`);
                                }
                              }}
                            >
                              Promote
                            </Button>
                          ))}
                        </Box>
                      ) : null;
                    })()}


                  </CardContent>
                </Card>
              </Grid>
            ))}
        </Grid>
      </Content>
    </Page>
  );
};
