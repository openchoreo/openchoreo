import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import type {
  ModelsBuild,
  RuntimeLogsResponse,
} from '@internal/plugin-openchoreo-api';

export interface BuildLogsParams {
  componentName: string;
  buildId: string;
  buildUuid: string;
  limit?: number;
  sortOrder?: 'asc' | 'desc';
}

export async function getBuildLogs(
  discovery: DiscoveryApi,
  identity: IdentityApi,
  params: BuildLogsParams,
): Promise<RuntimeLogsResponse> {
  const { token } = await identity.getCredentials();
  const baseUrl = await discovery.getBaseUrl('choreo');

  const url = new URL(`${baseUrl}/build-logs`);
  url.searchParams.set('componentName', params.componentName);
  url.searchParams.set('buildId', params.buildId);
  url.searchParams.set('buildUuid', params.buildUuid);
  url.searchParams.set('limit', (params.limit || 100).toString());
  url.searchParams.set('sortOrder', params.sortOrder || 'desc');

  const response = await fetch(url.toString(), {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
  }

  return await response.json();
}

export async function fetchBuildLogsForBuild(
  discovery: DiscoveryApi,
  identity: IdentityApi,
  build: ModelsBuild,
): Promise<RuntimeLogsResponse> {
  if (!build.componentName || !build.name || !build.uuid) {
    throw new Error('Component name, Build ID, or UUID not available');
  }

  const params: BuildLogsParams = {
    componentName: build.componentName,
    buildId: build.name,
    buildUuid: build.uuid,
    limit: 100,
    sortOrder: 'desc',
  };

  return getBuildLogs(discovery, identity, params);
}
