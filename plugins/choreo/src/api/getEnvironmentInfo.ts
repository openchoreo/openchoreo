import { Entity } from '@backstage/catalog-model/index';
import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { CHOREO_LABELS, API_ENDPOINTS } from '../constants';

export async function getEnvironmentInfo(
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
