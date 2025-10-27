import React from 'react';
import { Box, Typography, Card, Chip } from '@material-ui/core';
import StorageIcon from '@material-ui/icons/Storage';
import CloudIcon from '@material-ui/icons/Cloud';
import AppsIcon from '@material-ui/icons/Apps';
import AccountTreeIcon from '@material-ui/icons/AccountTree';
import ExpandMoreIcon from '@material-ui/icons/ExpandMore';
import ExpandLessIcon from '@material-ui/icons/ExpandLess';
import { DataPlaneWithEnvironments } from '../../types';
import { useStyles } from './styles';

interface PlatformDetailsCardProps {
  dataplanesWithEnvironments: DataPlaneWithEnvironments[];
  expandedDataplanes: Set<string>;
  onToggleDataplaneExpansion: (dataplaneName: string) => void;
}

export const PlatformDetailsCard: React.FC<PlatformDetailsCardProps> = ({
  dataplanesWithEnvironments,
  expandedDataplanes,
  onToggleDataplaneExpansion,
}) => {
  const classes = useStyles();

  return (
    <Box className={classes.dataplaneDetailsSection}>
      <Typography className={classes.dataplaneDetailTitle}>
        <AccountTreeIcon />
        Platform Details
      </Typography>

      {dataplanesWithEnvironments.map(dataplane => {
        const isExpanded = expandedDataplanes.has(dataplane.name);

        return (
          <Box key={dataplane.name} className={classes.dataplaneCard}>
            {/* Dataplane Header */}
            <Box
              className={classes.dataplaneHeader}
              onClick={() => onToggleDataplaneExpansion(dataplane.name)}
            >
              <Box className={classes.dataplaneTitle}>
                <StorageIcon className={classes.dataplaneIcon} />
                <Box>
                  <Typography variant="h6">
                    {dataplane.displayName || dataplane.name}
                  </Typography>
                  <Typography variant="body2" color="textSecondary">
                    {dataplane.environments.length} environments •{' '}
                    {dataplane.organization}
                  </Typography>
                </Box>
              </Box>
              <Box display="flex" alignItems="center" gridGap={8}>
                {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
              </Box>
            </Box>

            {/* Expandable Environments Section */}
            {isExpanded && (
              <Box className={classes.environmentsSection}>
                {dataplane.description && (
                  <Typography
                    variant="body2"
                    color="textSecondary"
                    style={{ marginBottom: 16 }}
                  >
                    {dataplane.description}
                  </Typography>
                )}

                <Typography className={classes.sectionTitle}>
                  <CloudIcon style={{ fontSize: '1rem' }} />
                  Environments ({dataplane.environments.length})
                </Typography>

                {dataplane.environments.length === 0 ? (
                  <Box className={classes.emptyState}>
                    <CloudIcon
                      style={{
                        fontSize: '2rem',
                        opacity: 0.3,
                        marginBottom: 8,
                      }}
                    />
                    <Typography variant="body2">
                      No environments found for this dataplane
                    </Typography>
                  </Box>
                ) : (
                  <Box className={classes.environmentGrid}>
                    {dataplane.environments
                      .sort((a, b) => {
                        // Sort non-production first, then production
                        if (a.isProduction === b.isProduction) return 0;
                        return a.isProduction ? 1 : -1;
                      })
                      .map(environment => {
                        const isProduction = environment.isProduction;
                        const cardClass = `${classes.environmentCard} ${
                          isProduction
                            ? classes.environmentCardProduction
                            : classes.environmentCardNonProduction
                        }`;

                        return (
                          <Card
                            key={`${environment.organization}-${environment.name}`}
                            className={cardClass}
                            elevation={0}
                          >
                            {/* Environment Header */}
                            <Box className={classes.environmentHeader}>
                              <Box>
                                <Typography className={classes.environmentName}>
                                  {environment.displayName || environment.name}
                                </Typography>
                              </Box>
                              <Chip
                                label={isProduction ? 'Prod' : 'Non-Prod'}
                                className={`${classes.environmentChip} ${
                                  isProduction
                                    ? classes.productionChip
                                    : classes.nonProductionChip
                                }`}
                                size="small"
                                variant="outlined"
                              />
                            </Box>

                            {/* Environment Content */}
                            <Box className={classes.environmentContent}>
                              <Box className={classes.environmentDetail}>
                                <Typography
                                  className={classes.environmentLabel}
                                >
                                  DNS Prefix
                                </Typography>
                                <Typography
                                  className={classes.environmentValue}
                                >
                                  {environment.dnsPrefix}
                                </Typography>
                              </Box>

                              <Box className={classes.environmentDetail}>
                                <Typography
                                  className={classes.environmentLabel}
                                >
                                  Components
                                </Typography>
                                <Box className={classes.componentCount}>
                                  <AppsIcon style={{ fontSize: '0.875rem' }} />
                                  {environment.componentCount ?? 0}
                                </Box>
                              </Box>

                              <Box className={classes.environmentDetail}>
                                <Typography
                                  className={classes.environmentLabel}
                                >
                                  Status
                                </Typography>
                                <Typography
                                  className={classes.environmentValue}
                                >
                                  {environment.status}
                                </Typography>
                              </Box>
                            </Box>
                          </Card>
                        );
                      })}
                  </Box>
                )}
              </Box>
            )}
          </Box>
        );
      })}
    </Box>
  );
};
