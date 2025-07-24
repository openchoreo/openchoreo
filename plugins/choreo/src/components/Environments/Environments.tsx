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
import { Grid, Card, CardContent, Typography, Box, Button, IconButton } from '@material-ui/core';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import AccessTimeIcon from '@mui/icons-material/AccessTime';
import { StatusOK, StatusError } from '@backstage/core-components';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { fetchEnvironmentInfo, promoteToEnvironment } from '../../api/getEnvironmentInfo';
import { formatRelativeTime } from '../../utils/timeUtils';

interface EndpointInfo {
  name: string;
  type: string;
  url: string;
  visibility: 'project' | 'organization' | 'public';
}

interface Environment {
  name: string;
  deployment: {
    status: 'success' | 'failed' | 'pending' | 'not-deployed' | 'suspended';
    lastDeployed?: string;
    image?: string;
    statusMessage?: string;
  };
  endpoints: EndpointInfo[];
  promotionTargets?: {
    name: string;
    requiresApproval?: boolean;
    isManualApprovalRequired?: boolean;
  }[];
}


export const Environments = () => {
  const { entity } = useEntity();
  const [environments, setEnvironmentsData] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(true);
  const discovery = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);
  
  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        const data = await fetchEnvironmentInfo(entity, discovery, identityApi);
        setEnvironmentsData(data as Environment[]);
      } catch (error) {
        console.error('Error fetching environment data:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  if (loading) {
    return (
      <Page themeId="tool">
        <Content>
          <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
            <Typography variant="h6">Loading environments...</Typography>
          </Box>
        </Content>
      </Page>
    );
  }

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
          {environments.map(env => (
              <Grid key={env.name} item xs={12} md={3}>
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
                    {env.deployment.lastDeployed && (
                      <Box display="flex" alignItems="center">
                        <Typography variant="body2" color="textSecondary">
                          Deployed
                        </Typography>
                        <AccessTimeIcon style={{ fontSize: '1rem', color: 'rgba(0, 0, 0, 0.54)', marginLeft: '8px', marginRight: '8px' }} />
                        <Typography variant="body2" color="textSecondary">
                          {formatRelativeTime(env.deployment.lastDeployed)}
                        </Typography>
                      </Box>
                    )}
                    <Box 
                      display="flex" 
                      alignItems="center" 
                      mt={2} 
                      bgcolor={
                        env.deployment.status === 'success' ? '#e8f5e9' : 
                        env.deployment.status === 'failed' ? '#ffebee' :
                        env.deployment.status === 'pending' ? '#fff3e0' :
                        env.deployment.status === 'suspended' ? '#fff8e1' :
                        'grey.200'
                      }
                      padding={1} 
                      borderRadius={1}
                    >
                      <Typography variant="body2" color="textSecondary">
                        Deployment Status:{' '}
                        <span
                          style={{
                            color: 
                              env.deployment.status === 'success' ? '#2e7d32' : 
                              env.deployment.status === 'failed' ? '#d32f2f' :
                              env.deployment.status === 'pending' ? '#f57c00' :
                              env.deployment.status === 'suspended' ? '#f57f17' :
                              'inherit',
                            fontWeight: env.deployment.status === 'success' ? 'bold' : 'normal'
                          }}
                        >
                          {env.deployment.status === 'success'
                            ? 'Active'
                            : env.deployment.status === 'pending'
                            ? 'Pending'
                            : env.deployment.status === 'not-deployed'
                            ? 'Not Deployed'
                            : env.deployment.status === 'suspended'
                            ? 'Suspended'
                            : 'Failed'}
                        </span>
                      </Typography>
                    </Box>
                    {env.deployment.statusMessage && (
                      <Box mt={1}>
                        <Typography variant="caption" color="textSecondary">
                          {env.deployment.statusMessage}
                        </Typography>
                      </Box>
                    )}
                    
                    {env.deployment.image && (
                      <>
                        <Box display="flex" alignItems="center" mt={2}>
                          <Typography variant="body2" color="textSecondary">
                            Image
                          </Typography>
                        </Box>
                        <Box 
                          display="flex" 
                          alignItems="center" 
                          mt={1} 
                          bgcolor="white" 
                          padding={1.5} 
                          borderRadius={3}
                          border="1px solid #e0e0e0"
                          boxShadow="0 2px 4px rgba(0,0,0,0.1)"
                        >
                          <Typography variant="body2" color="textSecondary" style={{ wordBreak: 'break-all' }}>
                            {env.deployment.image}
                          </Typography>
                        </Box>
                      </>
                    )}

                    {env.deployment.status === 'success' && env.endpoints.length > 0 && (
                      <>
                        <Box display="flex" alignItems="center" mt={2}>
                          <Typography variant="body2" color="textSecondary">
                            Endpoints
                          </Typography>
                        </Box>
                        {env.endpoints.map((endpoint, index) => (
                          <Box 
                            key={index} 
                            display="flex" 
                            alignItems="center" 
                            mt={index === 0 ? 0 : 1}
                            sx={{ minWidth: 0, width: '100%' }}
                          >
                            <Box sx={{ flex: 1, minWidth: 0, mr: 1 }}>
                              <a
                                href={endpoint.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                style={{ 
                                  color: '#1976d2', 
                                  textDecoration: 'underline',
                                  display: 'block',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                  whiteSpace: 'nowrap',
                                  fontSize: '0.875rem'
                                }}
                              >
                                {endpoint.url}
                              </a>
                            </Box>
                            <Box sx={{ flexShrink: 0 }}>
                              <IconButton
                                size="small"
                                onClick={() => {
                                  navigator.clipboard.writeText(endpoint.url);
                                  // You could add a toast notification here
                                }}
                              >
                                <ContentCopyIcon fontSize="small" />
                              </IconButton>
                            </Box>
                          </Box>
                        ))}
                      </>
                    )}

                    {/* Promotion buttons section - only show if deployment is successful */}
                    {env.deployment.status === 'success' && 
                     env.promotionTargets && 
                     env.promotionTargets.length > 0 && (
                      <Box mt={3}>
                        {env.promotionTargets.map((target, index) => (
                          <Box key={target.name} mb={index < env.promotionTargets!.length - 1 ? 2 : 0}>
                            <Button
                              variant="contained"
                              color="primary"
                              size="small"
                              onClick={async () => {
                                try {
                                  const result = await promoteToEnvironment(
                                    entity,
                                    discovery,
                                    identityApi,
                                    target.name.toLowerCase(),
                                  );
                                  alert(`Promotion started: ${JSON.stringify(result)}`);
                                } catch (err) {
                                  alert(`Error promoting: ${err}`);
                                }
                              }}
                            >
                              {env.promotionTargets!.length === 1 ? 'Promote' : `Promote to ${target.name}`}
                              {target.requiresApproval && ' (Approval Required)'}
                            </Button>
                          </Box>
                        ))}
                      </Box>
                    )}


                  </CardContent>
                </Card>
              </Grid>
            ))}
        </Grid>
      </Content>
    </Page>
  );
};
