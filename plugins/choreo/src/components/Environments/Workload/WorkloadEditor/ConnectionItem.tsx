import { FC, useCallback, useEffect, useMemo, useState } from 'react';
import {
  IconButton,
  Grid,
  Typography,
  Box,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import DeleteIcon from '@material-ui/icons/Delete';
import { Connection, ModelsWorkload } from '@internal/plugin-openchoreo-api';
import { catalogApiRef, useEntity } from '@backstage/plugin-catalog-react';
import {
  discoveryApiRef,
  identityApiRef,
  useApi,
} from '@backstage/core-plugin-api';
import { Entity } from '@backstage/catalog-model';
import { CHOREO_ANNOTATIONS } from '@internal/plugin-openchoreo-api';
import { fetchWorkloadInfo } from '../../../../api/workloadInfo';

interface ConnectionItemProps {
  connectionName: string;
  connection: Connection;
  onConnectionChange: (connectionName: string, connection: Connection) => void;
  onRemoveConnection: (connectionName: string) => void;
  disabled: boolean;
}

const useStyles = makeStyles(theme => ({
  dynamicFieldContainer: {
    padding: theme.spacing(2),
    marginBottom: theme.spacing(2),
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
  },
}));

enum ConnectionTypes {
  API = 'api',
}

export const ConnectionItem: FC<ConnectionItemProps> = ({
  connectionName,
  connection,
  onConnectionChange,
  onRemoveConnection,
  disabled,
}) => {
  const classes = useStyles();
  const catalogApi = useApi(catalogApiRef);
  const discoveryApi = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);

  const { entity: selectedEntity } = useEntity();
  const [allComponents, setAllComponents] = useState<Entity[]>([]);
  const [endPointList, setEndPointList] = useState<string[]>([]);

  const projectList = useMemo(() => {
    return allComponents.reduce((acc, component) => {
      const projectName =
        component.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT];
      if (projectName && !acc.includes(projectName)) {
        acc.push(projectName);
      }
      return acc;
    }, [] as string[]);
  }, [allComponents]);

  const componentsList = useMemo(() => {
    return allComponents.filter(
      component =>
        component.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT] ===
        connection.params?.projectName,
    );
  }, [allComponents, connection.params?.projectName]);

  useEffect(() => {
    const fetchComponents = async () => {
      const components = await catalogApi.getEntities();
      setAllComponents(
        components.items?.filter(
          entity =>
            entity.kind === 'Component' &&
            !(
              entity.metadata.name === selectedEntity.metadata.name &&
              entity.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT] ===
                connection.params?.projectName
            ),
        ) || [],
      );
    };
    fetchComponents();
  }, [
    catalogApi,
    selectedEntity.metadata.name,
    connection.params?.projectName,
  ]);

  useEffect(() => {
    const fetchEndPoints = async () => {
      const component = allComponents.find(
        entity =>
          entity.metadata.name === connection.params?.componentName &&
          entity.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT] ===
            connection.params?.projectName,
      );
      if (component) {
        try {
          const toWorkload: ModelsWorkload = await fetchWorkloadInfo(
            component,
            discoveryApi,
            identityApi,
          );
          setEndPointList(Object.keys(toWorkload?.endpoints || {}));
        } catch (error) {
          setEndPointList([]);
        }
      }
    };
    fetchEndPoints();
  }, [
    allComponents,
    connection.params?.componentName,
    discoveryApi,
    identityApi,
    connection.params?.projectName,
  ]);

  const handleFieldChange = useCallback(
    (field: string, value: string) => {
      const currentConnection = connection;
      let updatedConnection: Connection;

      if (field === 'type') {
        updatedConnection = { ...currentConnection, type: value };
      } else if (field.startsWith('params.')) {
        const paramField = field.split('.')[1];
        updatedConnection = {
          ...currentConnection,
          params: { ...currentConnection.params, [paramField]: value },
        };
      } else {
        updatedConnection = currentConnection;
      }

      onConnectionChange(connectionName, updatedConnection);
    },
    [connection, connectionName, onConnectionChange],
  );

  useEffect(() => {
    if (connection.params?.projectName) {
      const initialComponent = componentsList[0];
      if (initialComponent) {
        handleFieldChange(
          'params.componentName',
          initialComponent.metadata.name,
        );
        handleFieldChange(
          'params.projectName',
          initialComponent.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT] ||
            '',
        );
      }
    }
  }, [componentsList, connection.params?.projectName, handleFieldChange]);

  return (
    <Box className={classes.dynamicFieldContainer}>
      <Grid container spacing={2} alignItems="center">
        <Grid item xs={12}>
          <Typography variant="subtitle2" style={{ marginBottom: 8 }}>
            {connectionName}
          </Typography>
        </Grid>
        <Grid item xs={12}>
          <FormControl fullWidth variant="outlined">
            <InputLabel>Connection Type</InputLabel>
            <Select
              label="Connection Type"
              placeholder="Connection Type"
              value={connection.type || ''}
              onChange={e =>
                handleFieldChange('type', e.target.value as string)
              }
              fullWidth
              variant="outlined"
              disabled={disabled}
            >
              <MenuItem value={ConnectionTypes.API}>API</MenuItem>
            </Select>
          </FormControl>
        </Grid>

        <Grid item xs={6}>
          <FormControl fullWidth variant="outlined">
            <InputLabel>Project Name</InputLabel>
            <Select
              label="Project Name"
              value={connection.params?.projectName || ''}
              onChange={e =>
                handleFieldChange(
                  'params.projectName',
                  e.target.value as string,
                )
              }
              fullWidth
              variant="outlined"
              disabled={disabled}
            >
              {projectList.map(project => {
                return (
                  <MenuItem key={project} value={project}>
                    {project}
                  </MenuItem>
                );
              })}
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={6}>
          <FormControl fullWidth variant="outlined">
            <InputLabel>Component Name</InputLabel>
            <Select
              label="Component Name"
              value={connection.params?.componentName || ''}
              onChange={e =>
                handleFieldChange(
                  'params.componentName',
                  e.target.value as string,
                )
              }
              fullWidth
              variant="outlined"
              disabled={disabled}
            >
              {componentsList.map(component => (
                <MenuItem
                  key={component.metadata.name}
                  value={component.metadata.name}
                >
                  {component.metadata.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={5}>
          <FormControl fullWidth variant="outlined">
            <InputLabel>Endpoint</InputLabel>
            <Select
              label="Endpoint"
              value={connection.params?.endpoint || ''}
              onChange={e =>
                handleFieldChange('params.endpoint', e.target.value as string)
              }
              fullWidth
              variant="outlined"
              disabled={disabled}
            >
              {endPointList.map(endpoint => (
                <MenuItem key={endpoint} value={endpoint}>
                  {endpoint}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={1}>
          <IconButton
            onClick={() => onRemoveConnection(connectionName)}
            color="secondary"
            size="small"
            disabled={disabled}
          >
            <DeleteIcon />
          </IconButton>
        </Grid>
      </Grid>
    </Box>
  );
};
