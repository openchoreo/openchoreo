import { Entity } from '@backstage/catalog-model/index';
import { API_ENDPOINTS } from '../constants';
import { CHOREO_ANNOTATIONS } from '@internal/plugin-openchoreo-api';
import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { ModelsWorkload } from '@internal/plugin-openchoreo-api';

export async function fetchWorkloadInfo(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
) {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('choreo')}${
      API_ENDPOINTS.DEPLOYEMNT_WORKLOAD
    }`,
  );
  const componentName =
    entity.metadata.annotations?.[CHOREO_ANNOTATIONS.COMPONENT];
  const projectName = entity.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT];
  const organizationName =
    entity.metadata.annotations?.[CHOREO_ANNOTATIONS.ORGANIZATION];
  if (!componentName || !projectName || !organizationName) {
    throw new Error('Missing required labels');
  }
  const params = new URLSearchParams({
    componentName,
    projectName,
    organizationName,
  });
  backendUrl.search = params.toString();
  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });
  if (!res.ok) {
    throw new Error('Failed to fetch workload info');
  }
  return res.json();
}

export async function applyWorkload(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
  workloadSpec: ModelsWorkload,
) {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('choreo')}${
      API_ENDPOINTS.DEPLOYEMNT_WORKLOAD
    }`,
  );
  const componentName =
    entity.metadata.annotations?.[CHOREO_ANNOTATIONS.COMPONENT];
  const projectName = entity.metadata.annotations?.[CHOREO_ANNOTATIONS.PROJECT];
  const organizationName =
    entity.metadata.annotations?.[CHOREO_ANNOTATIONS.ORGANIZATION];
  if (!componentName || !projectName || !organizationName) {
    throw new Error('Missing required labels');
  }
  const params = new URLSearchParams({
    componentName,
    projectName,
    organizationName,
  });
  backendUrl.search = params.toString();
  const res = await fetch(backendUrl, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(workloadSpec),
  });
  if (!res.ok) {
    throw new Error('Failed to apply workload');
  }
  return res.json();
}
