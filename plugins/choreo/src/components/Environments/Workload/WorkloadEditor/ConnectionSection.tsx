import React from 'react';
import {
    TextField,
    Button,
    IconButton,
    Grid,
    Typography,
    Box,
    Accordion,
    AccordionSummary,
    AccordionDetails,
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import DeleteIcon from '@material-ui/icons/Delete';
import AddIcon from '@material-ui/icons/Add';
import ExpandMoreIcon from '@material-ui/icons/ExpandMore';

interface ConnectionSectionProps {
    connections: { [key: string]: string };
    onConnectionChange: (connectionName: string, value: string) => void;
    onAddConnection: () => void;
    onRemoveConnection: (connectionName: string) => void;
    disabled: boolean;
}

const useStyles = makeStyles((theme) => ({
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
}));

export const ConnectionSection: React.FC<ConnectionSectionProps> = ({
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
                    {Object.entries(connections).map(([connectionName, connectionValue]) => (
                        <Box key={connectionName} className={classes.dynamicFieldContainer}>
                            <Grid container spacing={2} alignItems="center">
                                <Grid item xs={4}>
                                    <Typography variant="subtitle2">{connectionName}</Typography>
                                </Grid>
                                <Grid item xs={6}>
                                    <TextField
                                        label="Connection Value"
                                        value={connectionValue}
                                        onChange={(e) => onConnectionChange(connectionName, e.target.value)}
                                        fullWidth
                                        variant="outlined"
                                        size="small"
                                    />
                                </Grid>
                                <Grid item xs={2}>
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
                    ))}
                    <Button
                        startIcon={<AddIcon />}
                        onClick={onAddConnection}
                        variant="contained"
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