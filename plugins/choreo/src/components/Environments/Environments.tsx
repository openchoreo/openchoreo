/* eslint-disable no-nested-ternary */
import { useCallback, useEffect, useState } from 'react';
import { useEntity } from '@backstage/plugin-catalog-react';
import { Content, Page } from '@backstage/core-components';
import {
  Grid,
  Card,
  CardContent,
  Typography,
  Box,
  Button,
  IconButton,
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import AccessTimeIcon from '@mui/icons-material/AccessTime';
import { StatusOK, StatusError } from '@backstage/core-components';

import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import {
  fetchEnvironmentInfo,
  promoteToEnvironment,
  updateComponentBinding,
} from '../../api/environments';
import { formatRelativeTime } from '../../utils/timeUtils';

interface EndpointInfo {
  name: string;
  type: string;
  url: string;
  visibility: 'project' | 'organization' | 'public';
}
import { Workload } from './Workload/Workload';
import Refresh from '@material-ui/icons/Refresh';

const useStyles = makeStyles(theme => ({
  notificationBox: {
    padding: theme.spacing(2),
    marginBottom: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid`,
  },
  successNotification: {
    backgroundColor: theme.palette.success.light,
    borderColor: theme.palette.success.main,
    color: theme.palette.success.dark,
  },
  errorNotification: {
    backgroundColor: theme.palette.error.light,
    borderColor: theme.palette.error.main,
    color: theme.palette.error.dark,
  },
  setupCard: {
    backgroundColor: theme.palette.background.paper,
    color: theme.palette.text.primary,
    padding: theme.spacing(1),
    borderRadius: theme.shape.borderRadius,
  },
  deploymentStatusBox: {
    padding: theme.spacing(1),
    borderRadius: theme.shape.borderRadius,
    marginTop: theme.spacing(2),
  },
  successStatus: {
    backgroundColor: theme.palette.success.light,
    color: theme.palette.success.dark,
  },
  errorStatus: {
    backgroundColor: theme.palette.error.light,
    color: theme.palette.error.dark,
  },
  warningStatus: {
    backgroundColor: theme.palette.warning.light,
    color: theme.palette.warning.dark,
  },
  defaultStatus: {
    backgroundColor: theme.palette.background.paper,
    color: theme.palette.text.primary,
  },
  imageContainer: {
    backgroundColor: theme.palette.background.paper,
    padding: theme.spacing(1.5),
    borderRadius: theme.spacing(3),
    border: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[2],
    marginTop: theme.spacing(1),
  },
  endpointLink: {
    color: theme.palette.primary.main,
    textDecoration: 'underline',
    display: 'block',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    fontSize: '0.875rem',
  },
  timeIcon: {
    fontSize: '1rem',
    color: theme.palette.text.secondary,
    marginLeft: theme.spacing(1),
    marginRight: theme.spacing(1),
  },
}));

interface Environment {
  name: string;
  bindingName?: string;
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
  const classes = useStyles();
  const { entity } = useEntity();
  const [environments, setEnvironmentsData] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(true);
  const [promotingTo, setPromotingTo] = useState<string | null>(null);
  const [updatingBinding, setUpdatingBinding] = useState<string | null>(null);
  const [notification, setNotification] = useState<{
    message: string;
    type: 'success' | 'error';
  } | null>(null);
  const discovery = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);

  const fetchEnvironmentsData = useCallback(async () => {
    try {
      setLoading(true);
      const data = await fetchEnvironmentInfo(entity, discovery, identityApi);
      setEnvironmentsData(data as Environment[]);
    } catch (error) {
      console.error('Error fetching environment data:', error);
    } finally {
      setLoading(false);
    }
  }, [entity, discovery, identityApi]);

  useEffect(() => {
    fetchEnvironmentsData();
  }, [fetchEnvironmentsData]);

  const isWorkloadEditorSupported = entity.metadata.tags?.find(
    tag => tag === 'webapplication' || tag === 'service',
  );
  const isPending = environments.some(
    env => env.deployment.status === 'pending',
  );

  useEffect(() => {
    let intervalId: NodeJS.Timeout;

    if (isPending) {
      intervalId = setInterval(() => {
        fetchEnvironmentsData();
      }, 10000); // 10 seconds
    }

    return () => {
      if (intervalId) {
        clearInterval(intervalId);
      }
    };
  }, [isPending, fetchEnvironmentsData]);

  if (loading && !isPending) {
    return (
      <Page themeId="tool">
        <Content>
          <Box
            display="flex"
            justifyContent="center"
            alignItems="center"
            minHeight="400px"
          >
            <Typography variant="h6">Loading environments...</Typography>
          </Box>
        </Content>
      </Page>
    );
  }

  return (
    <Page themeId="tool">
      <Content>
        {notification && (
          <Box
            className={`${classes.notificationBox} ${
              notification.type === 'success'
                ? classes.successNotification
                : classes.errorNotification
            }`}
          >
            <Typography
              variant="body2"
              style={{ fontWeight: 'bold' }}
            >
              {notification.type === 'success' ? '✓ ' : '✗ '}
              {notification.message}
            </Typography>
          </Box>
        )}
        <Grid container spacing={3}>
          <Grid item xs={12} md={3}>
            <Card>
              {/* Make this card color different from the others */}
              <Box className={classes.setupCard}>
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
                  {isWorkloadEditorSupported && !loading && (
                    <Workload
                      onDeployed={fetchEnvironmentsData}
                      isWorking={isPending}
                    />
                  )}
                </CardContent>
              </Box>
            </Card>
          </Grid>
          {environments.map(env => (
            <Grid key={env.name} item xs={12} md={3}>
              <Card>
                <CardContent>
                  <Box
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                  >
                    <Typography variant="h6" component="h4">
                      {env.name}
                    </Typography>
                    <IconButton onClick={() => fetchEnvironmentsData()}>
                      <Refresh
                        fontSize="inherit"
                        style={{ fontSize: '18px' }}
                      />
                    </IconButton>
                  </Box>
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
                      <AccessTimeIcon className={classes.timeIcon} />
                      <Typography variant="body2" color="textSecondary">
                        {formatRelativeTime(env.deployment.lastDeployed)}
                      </Typography>
                    </Box>
                  )}
                  <Box
                    display="flex"
                    alignItems="center"
                    className={`${classes.deploymentStatusBox} ${
                      env.deployment.status === 'success'
                        ? classes.successStatus
                        : env.deployment.status === 'failed'
                        ? classes.errorStatus
                        : env.deployment.status === 'pending'
                        ? classes.warningStatus
                        : env.deployment.status === 'suspended'
                        ? classes.warningStatus
                        : classes.defaultStatus
                    }`}
                  >
                    <Typography variant="body2" color="textSecondary">
                      Deployment Status:{' '}
                      <span
                        style={{
                          fontWeight:
                            env.deployment.status === 'success'
                              ? 'bold'
                              : 'normal',
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
                        className={classes.imageContainer}
                      >
                        <Typography
                          variant="body2"
                          color="textSecondary"
                          style={{ wordBreak: 'break-all' }}
                        >
                          {env.deployment.image}
                        </Typography>
                      </Box>
                    </>
                  )}

                  {env.deployment.status === 'success' &&
                    env.endpoints.length > 0 && (
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
                                className={classes.endpointLink}
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

                  {/* Actions section - show if deployment is successful or suspended */}
                  {((env.deployment.status === 'success' &&
                    env.promotionTargets &&
                    env.promotionTargets.length > 0) ||
                    ((env.deployment.status === 'success' ||
                      env.deployment.status === 'suspended') &&
                      env.bindingName)) && (
                    <Box mt={3}>
                      {/* Multiple promotion targets - stack vertically */}
                      {env.deployment.status === 'success' &&
                        env.promotionTargets &&
                        env.promotionTargets.length > 1 &&
                        env.promotionTargets.map((target, index) => (
                          <Box
                            key={target.name}
                            mb={
                              index < env.promotionTargets!.length - 1
                                ? 2
                                : (env.deployment.status === 'success' ||
                                    env.deployment.status === 'suspended') &&
                                  env.bindingName
                                ? 2
                                : 0
                            }
                          >
                            <Button
                              variant="contained"
                              color="primary"
                              size="small"
                              disabled={promotingTo === target.name}
                              onClick={async () => {
                                try {
                                  setPromotingTo(target.name);
                                  const result = await promoteToEnvironment(
                                    entity,
                                    discovery,
                                    identityApi,
                                    env.name.toLowerCase(), // source environment
                                    target.name.toLowerCase(), // target environment
                                  );

                                  // Update environments state with fresh data from promotion result
                                  setEnvironmentsData(result as Environment[]);

                                  setNotification({
                                    message: `Component promoted from ${env.name} to ${target.name}`,
                                    type: 'success',
                                  });

                                  // Clear notification after 5 seconds
                                  setTimeout(() => setNotification(null), 5000);
                                } catch (err) {
                                  setNotification({
                                    message: `Error promoting: ${err}`,
                                    type: 'error',
                                  });

                                  // Clear notification after 7 seconds for errors
                                  setTimeout(() => setNotification(null), 7000);
                                } finally {
                                  setPromotingTo(null);
                                }
                              }}
                            >
                              {promotingTo === target.name
                                ? 'Promoting...'
                                : `Promote to ${target.name}`}
                              {target.requiresApproval &&
                                !promotingTo &&
                                ' (Approval Required)'}
                            </Button>
                          </Box>
                        ))}

                      {/* Single promotion target and suspend button - show in same row */}
                      {((env.deployment.status === 'success' &&
                        env.promotionTargets &&
                        env.promotionTargets.length === 1) ||
                        ((env.deployment.status === 'success' ||
                          env.deployment.status === 'suspended') &&
                          env.bindingName)) && (
                        <Box display="flex" flexWrap="wrap">
                          {/* Single promotion button */}
                          {env.deployment.status === 'success' &&
                            env.promotionTargets &&
                            env.promotionTargets.length === 1 && (
                              <Button
                                style={{ marginRight: '8px' }}
                                variant="contained"
                                color="primary"
                                size="small"
                                disabled={
                                  promotingTo === env.promotionTargets[0].name
                                }
                                onClick={async () => {
                                  try {
                                    setPromotingTo(
                                      env.promotionTargets![0].name,
                                    );
                                    const result = await promoteToEnvironment(
                                      entity,
                                      discovery,
                                      identityApi,
                                      env.name.toLowerCase(), // source environment
                                      env.promotionTargets![0].name.toLowerCase(), // target environment
                                    );

                                    // Update environments state with fresh data from promotion result
                                    setEnvironmentsData(
                                      result as Environment[],
                                    );

                                    setNotification({
                                      message: `Component promoted from ${
                                        env.name
                                      } to ${env.promotionTargets![0].name}`,
                                      type: 'success',
                                    });

                                    // Clear notification after 5 seconds
                                    setTimeout(
                                      () => setNotification(null),
                                      5000,
                                    );
                                  } catch (err) {
                                    setNotification({
                                      message: `Error promoting: ${err}`,
                                      type: 'error',
                                    });

                                    // Clear notification after 7 seconds for errors
                                    setTimeout(
                                      () => setNotification(null),
                                      7000,
                                    );
                                  } finally {
                                    setPromotingTo(null);
                                  }
                                }}
                              >
                                {promotingTo === env.promotionTargets[0].name
                                  ? 'Promoting...'
                                  : 'Promote'}
                                {env.promotionTargets[0].requiresApproval &&
                                  !promotingTo &&
                                  ' (Approval Required)'}
                              </Button>
                            )}

                          {/* Suspend/Re-deploy button */}
                          {(env.deployment.status === 'success' ||
                            env.deployment.status === 'suspended') &&
                            env.bindingName && (
                              <Button
                                variant="outlined"
                                color={
                                  env.deployment.status === 'suspended'
                                    ? 'primary'
                                    : 'default'
                                }
                                size="small"
                                disabled={updatingBinding === env.name}
                                onClick={async () => {
                                  try {
                                    setUpdatingBinding(env.name);
                                    const newState =
                                      env.deployment.status === 'suspended'
                                        ? 'Active'
                                        : 'Suspend';
                                    await updateComponentBinding(
                                      entity,
                                      discovery,
                                      identityApi,
                                      env.bindingName!,
                                      newState,
                                    );

                                    // Refresh the environments data
                                    await fetchEnvironmentsData();

                                    setNotification({
                                      message: `Deployment ${
                                        newState === 'Active'
                                          ? 're-deployed'
                                          : 'suspended'
                                      } successfully`,
                                      type: 'success',
                                    });

                                    // Clear notification after 5 seconds
                                    setTimeout(
                                      () => setNotification(null),
                                      5000,
                                    );
                                  } catch (err) {
                                    setNotification({
                                      message: `Error updating deployment: ${err}`,
                                      type: 'error',
                                    });

                                    // Clear notification after 7 seconds for errors
                                    setTimeout(
                                      () => setNotification(null),
                                      7000,
                                    );
                                  } finally {
                                    setUpdatingBinding(null);
                                  }
                                }}
                              >
                                {updatingBinding === env.name
                                  ? 'Updating...'
                                  : env.deployment.status === 'suspended'
                                  ? 'Re-deploy'
                                  : 'Suspend'}
                              </Button>
                            )}
                        </Box>
                      )}
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
