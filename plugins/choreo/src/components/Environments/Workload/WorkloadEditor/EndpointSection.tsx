import React from 'react';
import {
    TextField,
    Select,
    MenuItem,
    FormControl,
    InputLabel,
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
} from '@material-ui/core';
import { makeStyles } from '@material-ui/core/styles';
import DeleteIcon from '@material-ui/icons/Delete';
import AddIcon from '@material-ui/icons/Add';
import ExpandMoreIcon from '@material-ui/icons/ExpandMore';
import { WorkloadEndpoint } from '@internal/plugin-openchoreo-api';

interface EndpointSectionProps {
    endpoints: { [key: string]: WorkloadEndpoint };
    onEndpointChange: (endpointName: string, field: keyof WorkloadEndpoint, value: any) => void;
    onAddEndpoint: () => void;
    onRemoveEndpoint: (endpointName: string) => void;
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

const protocolTypes = ['TCP', 'UDP', 'HTTP', 'REST', 'gRPC', 'Websocket', 'GraphQL'];

export const EndpointSection: React.FC<EndpointSectionProps> = ({
    endpoints,
    onEndpointChange,
    onAddEndpoint,
    onRemoveEndpoint,
    disabled,
}) => {
    const classes = useStyles();

    return (
        <Accordion className={classes.accordion}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="h6">
                    Endpoints ({Object.keys(endpoints).length})
                </Typography>
            </AccordionSummary>
            <AccordionDetails>
                <Box width="100%">
                    {Object.entries(endpoints).map(([endpointName, endpoint]) => (
                        <Card key={endpointName} className={classes.dynamicFieldContainer}>
                            <CardHeader
                                title={
                                    <Box display="flex" alignItems="center" justifyContent="space-between">
                                        <Typography variant="subtitle1">{endpointName}</Typography>
                                        <IconButton
                                            onClick={() => onRemoveEndpoint(endpointName)}
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
                                    <Grid item xs={12} md={6}>
                                        <FormControl fullWidth variant="outlined">
                                            <InputLabel>Type</InputLabel>
                                            <Select
                                                disabled={disabled}
                                                value={endpoint.type}
                                                onChange={(e) => onEndpointChange(endpointName, 'type', e.target.value)}
                                                label="Type"
                                            >
                                                {protocolTypes.map((type) => (
                                                    <MenuItem key={type} value={type}>
                                                        {type}
                                                    </MenuItem>
                                                ))}
                                            </Select>
                                        </FormControl>
                                    </Grid>
                                    <Grid item xs={12} md={6}>
                                        <TextField
                                            label="Port"
                                            type="number"
                                            value={endpoint.port}
                                            onChange={(e) => onEndpointChange(endpointName, 'port', parseInt(e.target.value, 10))}
                                            fullWidth
                                            variant="outlined"
                                            required
                                            disabled={disabled}
                                        />
                                    </Grid>
                                    <Grid item xs={12} md={6}>
                                        <TextField
                                            label="Schema Type"
                                            value={endpoint.schema?.type || ''}
                                            onChange={(e) => onEndpointChange(endpointName, 'schema', { 
                                                ...endpoint.schema, 
                                                type: e.target.value 
                                            })}
                                            fullWidth
                                            variant="outlined"
                                            placeholder="e.g., REST, GraphQL, gRPC"
                                            helperText="Optional: Specify the API schema type"
                                            disabled={disabled}
                                        />
                                    </Grid>
                                    <Grid item xs={12}>
                                        <TextField
                                            label="Schema Content"
                                            value={endpoint.schema?.content || ''}
                                            onChange={(e) => onEndpointChange(endpointName, 'schema', { 
                                                ...endpoint.schema, 
                                                content: e.target.value 
                                            })}
                                            fullWidth
                                            variant="outlined"
                                            multiline
                                            placeholder="Enter schema definition (OpenAPI, GraphQL schema, protobuf, etc.)"
                                            helperText="Optional: Provide the actual schema definition"
                                            disabled={disabled}
                                        />
                                    </Grid>
                                </Grid>
                            </CardContent>
                        </Card>
                    ))}
                    <Button
                        startIcon={<AddIcon />}
                        onClick={onAddEndpoint}
                        variant="contained"
                        color="primary"
                        className={classes.addButton}
                        disabled={disabled}
                    >
                        Add Endpoint
                    </Button>
                </Box>
            </AccordionDetails>
        </Accordion>
    );
};
