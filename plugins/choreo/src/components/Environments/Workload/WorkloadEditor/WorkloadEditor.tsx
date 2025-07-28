import React, { useState, useEffect } from 'react';
import {
    Typography,
    Box,
    Button,
    CircularProgress,
} from '@material-ui/core';
import { ModelsWorkload, Container, WorkloadEndpoint, EnvVar, Connection, WorkloadType } from '@internal/plugin-openchoreo-api';
import { ContainerSection } from './ContainerSection';
import { EndpointSection } from './EndpointSection';
import { ConnectionSection } from './ConnectionSection';
import { Alert } from '@material-ui/lab';
import { CHOREO_LABELS } from '../../../../constants';
import { Entity } from '@backstage/catalog-model';
import { useWorkloadContext } from '../WorkloadContext';

interface WorkloadEditorProps {
    onDeploy: () => Promise<void>;
    entity: Entity;
}

export function WorkloadEditor({ onDeploy, entity }: WorkloadEditorProps) {
    const { workloadSpec, setWorkloadSpec, isDeploying } = useWorkloadContext();

    const componentName = entity.metadata.annotations?.[CHOREO_LABELS.COMPONENT];
    const projectName = entity.metadata.annotations?.[CHOREO_LABELS.PROJECT];

    const [formData, setFormData] = useState<Omit<ModelsWorkload, 'type'>>({
        name: entity.metadata.name,
        owner: {
            projectName: projectName || '',
            componentName: componentName || '',
        },
        containers: {},
        endpoints: {},
        connections: {},
        status: '',
    });

    const [workloadType, setWorkloadType] = useState<WorkloadType>('Service');
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (workloadSpec) {
            setFormData(workloadSpec);
            setWorkloadType(workloadSpec.type);
        }
    }, [workloadSpec]);

    // Helper function to update workload spec
    const updateWorkloadSpec = (updatedData: Omit<ModelsWorkload, 'type'>) => {
        setFormData(updatedData);
        setWorkloadSpec({ ...updatedData, type: workloadType });
    };

    const handleContainerChange = (containerName: string, field: keyof Container, value: any) => {
        const updatedContainers = {
            ...formData.containers,
            [containerName]: {
                ...(formData.containers?.[containerName] || {}),
                [field]: value,
            } as Container,
        };
        const updatedData = { ...formData, containers: updatedContainers };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const containerCount = Object.keys(formData.containers || {}).length;
    const endpointCount = Object.keys(formData.endpoints || {}).length;

    const handleDeploy = async () => {
        if (containerCount === 0 && endpointCount === 0) {
            setError('Please a container and one endpoint');
            return;
        }
        if (containerCount === 0) {
            setError('Please add a container');
            return;
        }
        if (endpointCount === 0) {
            setError('Please add at least one endpoint');
            return;
        }
        setError(null);
        try {
            await onDeploy();
        } catch (e: any) {
            setError(e.message);
        }
    };

    const handleEnvVarChange = (containerName: string, envIndex: number, field: keyof EnvVar, value: string) => {
        const container = formData.containers?.[containerName];
        if (!container) return;

        const updatedEnvVars = [...(container.env || [])];
        updatedEnvVars[envIndex] = { ...updatedEnvVars[envIndex], [field]: value };

        handleContainerChange(containerName, 'env', updatedEnvVars);
    };

    const addEnvVar = (containerName: string) => {
        const container = formData.containers?.[containerName];
        if (!container) return;

        const newEnvVar: EnvVar = { key: '', value: '' };
        const updatedEnvVars = [...(container.env || []), newEnvVar];
        handleContainerChange(containerName, 'env', updatedEnvVars);
    };

    const removeEnvVar = (containerName: string, envIndex: number) => {
        const container = formData.containers?.[containerName];
        if (!container) return;

        const updatedEnvVars = container.env?.filter((_, index) => index !== envIndex) || [];
        handleContainerChange(containerName, 'env', updatedEnvVars);
    };

    const addContainer = () => {
        const containerName = `container-${Object.keys(formData.containers || {}).length}`;
        const updatedContainers = {
            ...formData.containers,
            [Object.keys(formData.containers || {}).length === 0 ? 'main' : containerName]: {
                name: Object.keys(formData.containers || {}).length === 0 ? 'main' : containerName,
                image: '',
                resources: {},
                env: [],
                command: [],
                args: [],
            } as Container,
        };
        const updatedData = { ...formData, containers: updatedContainers };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const removeContainer = (containerName: string) => {
        const updatedContainers = { ...formData.containers };
        delete updatedContainers[containerName];
        const updatedData = { ...formData, containers: updatedContainers };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const handleEndpointChange = (endpointName: string, field: keyof WorkloadEndpoint, value: any) => {
        const updatedEndpoints = {
            ...formData.endpoints,
            [endpointName]: {
                ...formData.endpoints?.[endpointName],
                [field]: value,
            } as WorkloadEndpoint,
        };
        const updatedData = { ...formData, endpoints: updatedEndpoints };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const addEndpoint = () => {
        const endpointName = `endpoint-${Object.keys(formData.endpoints || {}).length + 1}`;
        const updatedEndpoints = {
            ...formData.endpoints,
            [endpointName]: {
                type: 'HTTP',
                port: 8080,
            } as WorkloadEndpoint,
        };
        const updatedData = { ...formData, endpoints: updatedEndpoints };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const removeEndpoint = (endpointName: string) => {
        const updatedEndpoints = { ...formData.endpoints };
        delete updatedEndpoints[endpointName];
        const updatedData = { ...formData, endpoints: updatedEndpoints };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const handleConnectionChange = (connectionName: string, connection: Connection) => {
        const updatedConnections = { ...formData.connections, [connectionName]: connection };
        const updatedData = { ...formData, connections: updatedConnections };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const addConnection = () => {
        const connectionName = `connection-${Object.keys(formData.connections || {}).length + 1}`;
        const newConnection: Connection = {
            type: '',
            params: {
                componentName: '',
                endpoint: '',
                projectName: '',
            },
            inject: {
                env: []
            }
        };
        const updatedConnections = { ...formData.connections, [connectionName]: newConnection };
        const updatedData = { ...formData, connections: updatedConnections };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const removeConnection = (connectionName: string) => {
        const updatedConnections = { ...formData.connections };
        delete updatedConnections[connectionName];
        const updatedData = { ...formData, connections: updatedConnections };
        setFormData(updatedData);
        updateWorkloadSpec(updatedData);
    };

    const handleArrayFieldChange = (containerName: string, field: 'command' | 'args', value: string) => {
        const arrayValue = value.split(',').map(item => item.trim()).filter(item => item.length > 0);
        handleContainerChange(containerName, field, arrayValue);
    };
    
    return (
        <Box overflow="hidden">
            <ContainerSection
                disabled={isDeploying}
                containers={formData.containers || {}}
                onContainerChange={handleContainerChange}
                onEnvVarChange={handleEnvVarChange}
                onAddContainer={addContainer}
                onRemoveContainer={removeContainer}
                onAddEnvVar={addEnvVar}
                onRemoveEnvVar={removeEnvVar}
                onArrayFieldChange={handleArrayFieldChange}
                singleContainerMode
            />
            <EndpointSection
                disabled={isDeploying}
                endpoints={formData.endpoints || {}}
                onEndpointChange={handleEndpointChange}
                onAddEndpoint={addEndpoint}
                onRemoveEndpoint={removeEndpoint}
            />
            <ConnectionSection
                disabled={isDeploying}
                connections={formData.connections || {}}
                onConnectionChange={handleConnectionChange}
                onAddConnection={addConnection}
                onRemoveConnection={removeConnection}
            />
            {error && (
                <Alert
                    severity="error"
                >
                    {error}
                </Alert>
            )}
            <Box display="flex" justifyContent="flex-end" margin={2}>
                <Button disabled={isDeploying} variant="contained" color="primary" onClick={handleDeploy}>
                    Submit & Deploy
                </Button>   
            </Box>
        </Box>
    );
}