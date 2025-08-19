import {
  TextField,
  Button,
  Card,
  CardContent,
  CardHeader,
  IconButton,
  Grid,
  Typography,
  Box,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  FormControl,
  Select,
  MenuItem,
  InputLabel,
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import DeleteIcon from '@material-ui/icons/Delete';
import AddIcon from '@material-ui/icons/Add';
import ExpandMoreIcon from '@material-ui/icons/ExpandMore';
import { Container, EnvVar } from '@openchoreo/backstage-plugin-api';
import { formatRelativeTime } from '../../../../utils/timeUtils';
import { useBuilds } from '../WorkloadContext';

interface ContainerSectionProps {
  containers: { [key: string]: Container };
  onContainerChange: (
    containerName: string,
    field: keyof Container,
    value: any,
  ) => void;
  onEnvVarChange: (
    containerName: string,
    envIndex: number,
    field: keyof EnvVar,
    value: string,
  ) => void;
  onAddContainer: () => void;
  onRemoveContainer: (containerName: string) => void;
  onAddEnvVar: (containerName: string) => void;
  onRemoveEnvVar: (containerName: string, envIndex: number) => void;
  onArrayFieldChange: (
    containerName: string,
    field: 'command' | 'args',
    value: string,
  ) => void;
  disabled: boolean;
  singleContainerMode: boolean;
}

const useStyles = makeStyles(theme => ({
  accordion: {
    marginBottom: theme.spacing(2),
  },
  dynamicFieldContainer: {
    padding: theme.spacing(2),
    marginBottom: theme.spacing(2),
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  },
  addButton: {
    marginTop: theme.spacing(1),
  },
  envVarContainer: {
    padding: theme.spacing(1),
    border: `1px dashed ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    marginBottom: theme.spacing(1),
  },
}));

export function ContainerSection({
  containers,
  onContainerChange,
  onEnvVarChange,
  onAddContainer,
  onRemoveContainer,
  onAddEnvVar,
  onRemoveEnvVar,
  onArrayFieldChange,
  disabled,
  singleContainerMode,
}: ContainerSectionProps) {
  const classes = useStyles();
  const { builds } = useBuilds();

  return (
    <Accordion className={classes.accordion} defaultExpanded>
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography variant="h6">
          Containers ({Object.keys(containers).length})
        </Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Box width="100%">
          {Object.entries(containers).map(([containerName, container]) => (
            <Card key={containerName} className={classes.dynamicFieldContainer}>
              <CardHeader
                title={
                  <Box
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                  >
                    <Typography variant="subtitle1">
                      {containerName === 'main' ? '' : containerName}
                    </Typography>
                    <IconButton
                      onClick={() => onRemoveContainer(containerName)}
                      color="secondary"
                      size="small"
                      disabled={disabled}
                    >
                      <DeleteIcon />
                    </IconButton>
                  </Box>
                }
              />
              <CardContent>
                <Grid container spacing={2}>
                  <Grid item xs={12}>
                    <Box mb={2}>
                      {builds.length === 0 && container.image ? (
                        <FormControl fullWidth variant="outlined">
                          <InputLabel>Image</InputLabel>
                          <TextField
                            label="Image"
                            value={container.image}
                            onChange={e =>
                              onContainerChange(
                                containerName,
                                'image',
                                e.target.value as string,
                              )
                            }
                            fullWidth
                            variant="outlined"
                            disabled={disabled}
                          />
                        </FormControl>
                      ) : (
                        <FormControl fullWidth variant="outlined">
                          <InputLabel>Select Image from Builds</InputLabel>
                          <Select
                            value={container.image || ''}
                            onChange={e =>
                              onContainerChange(
                                containerName,
                                'image',
                                e.target.value as string,
                              )
                            }
                            label="Select Image from Builds"
                            variant="outlined"
                            fullWidth
                            disabled={disabled}
                          >
                            <MenuItem value="">
                              <em>None</em>
                            </MenuItem>
                            {builds
                              .filter(build => build.image)
                              .map(
                                build =>
                                  build.image && (
                                    <MenuItem
                                      key={build.image}
                                      value={build.image}
                                    >
                                      {build.name} (
                                      {formatRelativeTime(build.createdAt)})
                                    </MenuItem>
                                  ),
                              )}
                          </Select>
                        </FormControl>
                      )}
                    </Box>
                  </Grid>
                  <Grid item xs={12} md={6}>
                    <TextField
                      label="Command"
                      value={container.command?.join(', ') || ''}
                      onChange={e =>
                        onArrayFieldChange(
                          containerName,
                          'command',
                          e.target.value,
                        )
                      }
                      fullWidth
                      variant="outlined"
                      placeholder="Comma-separated commands"
                      helperText="Separate multiple commands with commas"
                      disabled={disabled}
                    />
                  </Grid>
                  <Grid item xs={12} md={6}>
                    <TextField
                      label="Arguments"
                      value={container.args?.join(', ') || ''}
                      onChange={e =>
                        onArrayFieldChange(
                          containerName,
                          'args',
                          e.target.value,
                        )
                      }
                      fullWidth
                      variant="outlined"
                      placeholder="Comma-separated arguments"
                      helperText="Separate multiple arguments with commas"
                      disabled={disabled}
                    />
                  </Grid>
                </Grid>

                {/* Environment Variables */}
                <Box mt={2}>
                  <Typography variant="subtitle2" gutterBottom>
                    Environment Variables
                  </Typography>
                  {container.env?.map((envVar, index) => (
                    <Box key={index} className={classes.envVarContainer}>
                      <Grid container spacing={2} alignItems="center">
                        <Grid item xs={5}>
                          <TextField
                            label="Name"
                            value={envVar.key || ''}
                            onChange={e =>
                              onEnvVarChange(
                                containerName,
                                index,
                                'key',
                                e.target.value,
                              )
                            }
                            fullWidth
                            variant="outlined"
                            size="small"
                            disabled={!!envVar.valueFrom || disabled}
                          />
                        </Grid>
                        {envVar.valueFrom ? (
                          <>
                            <Grid item xs={5}>
                              <Typography variant="body2">
                                {envVar.valueFrom.configurationGroupRef.name}:
                                {envVar.valueFrom.configurationGroupRef.key}
                              </Typography>
                            </Grid>
                          </>
                        ) : (
                          <Grid item xs={5}>
                            <TextField
                              disabled={disabled}
                              label="Value"
                              value={envVar.value || ''}
                              onChange={e =>
                                onEnvVarChange(
                                  containerName,
                                  index,
                                  'value',
                                  e.target.value,
                                )
                              }
                              fullWidth
                              variant="outlined"
                              size="small"
                            />
                          </Grid>
                        )}

                        <Grid item xs={2}>
                          <IconButton
                            onClick={() => onRemoveEnvVar(containerName, index)}
                            color="secondary"
                            size="small"
                            disabled={disabled}
                          >
                            <DeleteIcon />
                          </IconButton>
                        </Grid>
                      </Grid>
                    </Box>
                  ))}
                  <Button
                    startIcon={<AddIcon />}
                    onClick={() => onAddEnvVar(containerName)}
                    variant="outlined"
                    size="small"
                    className={classes.addButton}
                    disabled={disabled}
                  >
                    Add Environment Variable
                  </Button>
                </Box>
              </CardContent>
            </Card>
          ))}
          {(!singleContainerMode || Object.keys(containers).length === 0) && (
            <Button
              startIcon={<AddIcon />}
              onClick={onAddContainer}
              variant="contained"
              color="primary"
              className={classes.addButton}
              disabled={disabled}
            >
              Add Container
            </Button>
          )}
        </Box>
      </AccordionDetails>
    </Accordion>
  );
}
