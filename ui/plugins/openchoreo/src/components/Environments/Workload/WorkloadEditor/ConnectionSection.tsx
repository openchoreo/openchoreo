import { FC } from 'react';
import {
  Button,
  Typography,
  Box,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import AddIcon from '@material-ui/icons/Add';
import ExpandMoreIcon from '@material-ui/icons/ExpandMore';
import { Connection } from '@openchoreo/backstage-plugin-api';
import { ConnectionItem } from './ConnectionItem';

interface ConnectionSectionProps {
  connections: { [key: string]: Connection };
  onConnectionChange: (connectionName: string, connection: Connection) => void;
  onAddConnection: () => void;
  onRemoveConnection: (connectionName: string) => void;
  disabled: boolean;
}

const useStyles = makeStyles(theme => ({
  accordion: {
    border: `1px solid ${theme.palette.grey[100]}`,
    marginBottom: theme.spacing(1),
    borderRadius: 4,
    boxShadow: 'none',
    '&:before': {
      backgroundColor: 'transparent',
    },
  },
  addButton: {
    marginTop: theme.spacing(1),
  },
}));

export const ConnectionSection: FC<ConnectionSectionProps> = ({
  connections,
  onConnectionChange,
  onAddConnection,
  onRemoveConnection,
  disabled,
}) => {
  const classes = useStyles();

  return (
    <Accordion className={classes.accordion}>
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography variant="h6">
          Connections ({Object.keys(connections).length})
        </Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Box width="100%">
          {Object.entries(connections).map(([connectionName, connection]) => (
            <ConnectionItem
              key={connectionName}
              connectionName={connectionName}
              connection={connection}
              onConnectionChange={onConnectionChange}
              onRemoveConnection={onRemoveConnection}
              disabled={disabled}
            />
          ))}
          <Button
            startIcon={<AddIcon />}
            onClick={onAddConnection}
            variant="outlined"
            color="primary"
            className={classes.addButton}
            disabled={disabled}
          >
            Add Connection
          </Button>
        </Box>
      </AccordionDetails>
    </Accordion>
  );
};
