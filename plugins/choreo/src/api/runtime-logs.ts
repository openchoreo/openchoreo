import { Entity } from '@backstage/catalog-model/index';
import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { CHOREO_LABELS } from '../constants/labels';
import { API_ENDPOINTS } from '../constants/api';
import {
  LogsResponse,
  RuntimeLogsParams,
  Environment,
} from '../components/RuntimeLogs/types';

export async function getRuntimeLogs(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
  params: RuntimeLogsParams,
): Promise<LogsResponse> {
  const { token } = await identity.getCredentials();
  const component = entity.metadata.annotations?.[CHOREO_LABELS.COMPONENT]; // TODO: Inconsistent entity labels

  if (!component) {
    throw new Error('Component name not found in entity labels');
  }

  const backendUrl = new URL(
    `${await discovery.getBaseUrl('choreo')}${
      API_ENDPOINTS.RUNTIME_LOGS
    }/${component}`,
  );

  const requestBody = {
    environmentId: params.environmentId,
    logLevels: params.logLevels,
    startTime: params.startTime,
    endTime: params.endTime,
    limit: params.limit,
    offset: params.offset,
  };

  const response = await fetch(backendUrl.toString(), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(requestBody),
  });

  if (!response.ok) {
    throw new Error(
      `Failed to fetch runtime logs: ${response.status} ${response.statusText}`,
    );
  }

  return await response.json();
}

export async function getEnvironments(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<Environment[]> {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('choreo')}${API_ENDPOINTS.ENVIRONMENT_INFO}`,
  );

  const component = entity.metadata.annotations?.[CHOREO_LABELS.COMPONENT];
  const project = entity.metadata.annotations?.[CHOREO_LABELS.PROJECT];
  const organization =
    entity.metadata.annotations?.[CHOREO_LABELS.ORGANIZATION];

  if (!project || !component || !organization) {
    return [];
  }

  const params = new URLSearchParams({
    componentName: component,
    projectName: project,
    organizationName: organization,
  });

  backendUrl.search = params.toString();

  const response = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok) {
    throw new Error(
      `Failed to fetch environments: ${response.status} ${response.statusText}`,
    );
  }

  const envData = await response.json();

  // Transform the environment data to match our interface
  return envData.map((env: any) => ({
    id: env.id || env.name,
    name: env.name || env.id,
  }));
}

export function calculateTimeRange(timeRange: string): {
  startTime: string;
  endTime: string;
} {
  const now = new Date();
  const endTime = now.toISOString();

  let startTime: Date;

  switch (timeRange) {
    case '10m':
      startTime = new Date(now.getTime() - 10 * 60 * 1000);
      break;
    case '30m':
      startTime = new Date(now.getTime() - 30 * 60 * 1000);
      break;
    case '1h':
      startTime = new Date(now.getTime() - 60 * 60 * 1000);
      break;
    case '24h':
      startTime = new Date(now.getTime() - 24 * 60 * 60 * 1000);
      break;
    case '7d':
      startTime = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
      break;
    case '14d':
      startTime = new Date(now.getTime() - 14 * 24 * 60 * 60 * 1000);
      break;
    default:
      startTime = new Date(now.getTime() - 60 * 60 * 1000); // Default to 1 hour
  }

  return {
    startTime: startTime.toISOString(),
    endTime,
  };
}
