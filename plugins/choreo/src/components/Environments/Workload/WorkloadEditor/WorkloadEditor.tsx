import React, { useState, useEffect } from 'react';
import {
    Typography,
    Box,
    Button,
} from '@material-ui/core';
import { ModelsWorkload, Container, WorkloadEndpoint, EnvVar } from '@internal/plugin-openchoreo-api';
import { ContainerSection } from './ContainerSection';
import { EndpointSection } from './EndpointSection';
import { ConnectionSection } from './ConnectionSection';
import { Alert } from '@material-ui/lab';

interface WorkloadEditorProps {
    workloadSpec: ModelsWorkload | null;
    onWorkloadSpecChange: (workloadSpec: ModelsWorkload) => void;
    onDeploy: () => Promise<void>;
}

export function WorkloadEditor({ workloadSpec, onWorkloadSpecChange, onDeploy }: WorkloadEditorProps) {
    const [formData, setFormData] = useState<ModelsWorkload>({
        name: '',
        type: 'Service',
        owner: {
            projectName: '',
            componentName: '',
        },
        containers: {},
        endpoints: {},
        connections: {},
        status: '',
    });

    const [isDeploying, setIsDeploying] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (workloadSpec) {
            setFormData(workloadSpec);
        }
    }, [workloadSpec]);

    const handleContainerChange = (containerName: string, field: keyof Container, value: any) => {
        const updatedContainers = {
            ...formData.containers,
            [containerName]: {
                ...formData.containers?.[containerName],
                [field]: value,
            } as Container,
        };
        const updatedData = { ...formData, containers: updatedContainers };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const handleDeploy = async () => {
        setIsDeploying(true);
        setError(null);
        try {
            await onDeploy();
        } catch (e: any) {
            setError(e.message);
        } finally {
            setIsDeploying(false);
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
        const containerName = `container-${Object.keys(formData.containers || {}).length + 1}`;
        const newContainer: Container = {
            image: '',
            command: [],
            args: [],
            env: [],
        };
        const updatedContainers = { ...formData.containers, [containerName]: newContainer };
        const updatedData = { ...formData, containers: updatedContainers };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const removeContainer = (containerName: string) => {
        const updatedContainers = { ...formData.containers };
        delete updatedContainers[containerName];
        const updatedData = { ...formData, containers: updatedContainers };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
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
        onWorkloadSpecChange(updatedData);
    };

    const addEndpoint = () => {
        const endpointName = `endpoint-${Object.keys(formData.endpoints || {}).length + 1}`;
        const newEndpoint: WorkloadEndpoint = {
            protocol: 'TCP',
            port: 8080,
        };
        const updatedEndpoints = { ...formData.endpoints, [endpointName]: newEndpoint };
        const updatedData = { ...formData, endpoints: updatedEndpoints };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const removeEndpoint = (endpointName: string) => {
        const updatedEndpoints = { ...formData.endpoints };
        delete updatedEndpoints[endpointName];
        const updatedData = { ...formData, endpoints: updatedEndpoints };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const handleConnectionChange = (connectionName: string, value: string) => {
        const updatedConnections = { ...formData.connections, [connectionName]: value };
        const updatedData = { ...formData, connections: updatedConnections };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const addConnection = () => {
        const connectionName = `connection-${Object.keys(formData.connections || {}).length + 1}`;
        const updatedConnections = { ...formData.connections, [connectionName]: '' };
        const updatedData = { ...formData, connections: updatedConnections };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const removeConnection = (connectionName: string) => {
        const updatedConnections = { ...formData.connections };
        delete updatedConnections[connectionName];
        const updatedData = { ...formData, connections: updatedConnections };
        setFormData(updatedData);
        onWorkloadSpecChange(updatedData);
    };

    const handleArrayFieldChange = (containerName: string, field: 'command' | 'args', value: string) => {
        const arrayValue = value.split(',').map(item => item.trim()).filter(item => item.length > 0);
        handleContainerChange(containerName, field, arrayValue);
    };

    if (!workloadSpec) {
        return (
            <Box display="flex" justifyContent="center" alignItems="center" height={200}>
                <Typography variant="h6" color="textSecondary">
                    Loading workload specification...
                </Typography>
            </Box>
        );
    }

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