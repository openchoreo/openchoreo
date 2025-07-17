import { Entity } from '@backstage/catalog-model/index';
import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { CHOREO_LABELS, API_ENDPOINTS } from '../constants';

export async function fetchEnvironmentInfo(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
) {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('choreo')}${API_ENDPOINTS.ENVIRONMENT_INFO}`,
  );
  const component = entity.metadata.annotations?.[CHOREO_LABELS.COMPONENT];
  const project = entity.metadata.annotations?.[CHOREO_LABELS.PROJECT];
  const organization =
    entity.metadata.annotations?.[CHOREO_LABELS.ORGANIZATION];
  if (!project || !component || !organization) {
    console.log('Missing required labels:', {
      project,
      organization,
      component,
    });
    return [];
  }
  const params = new URLSearchParams({
    componentName: component,
    projectName: project,
    organizationName: organization,
  });

  backendUrl.search = params.toString();

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  return await res.json();
}

export async function promoteToEnvironment(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
  sourceEnvironment: string,
) {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('choreo')}${API_ENDPOINTS.PROMOTE_DEPLOYMENT}`,
  );
  const component = entity.metadata.labels?.[CHOREO_LABELS.NAME];
  const project = entity.metadata.labels?.[CHOREO_LABELS.PROJECT];
  const organization = entity.metadata.labels?.[CHOREO_LABELS.ORGANIZATION];

  if (!project || !component || !organization) {
    throw new Error('Missing required metadata in entity');
  }

  const promoteReq = {
    environmentToPromote: sourceEnvironment,
    componentName: component,
    projectName: project,
    orgName: organization,
    deploymentTrack: 'main',
  };

  const res = await fetch(backendUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(promoteReq),
  });

  if (!res.ok) {
    const errText = await res.text();
    throw new Error(`Promotion failed: ${errText}`);
  }

  return await res.json();
}
