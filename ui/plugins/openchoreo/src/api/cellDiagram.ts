import { Entity } from '@backstage/catalog-model';
import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { API_ENDPOINTS } from '../constants';
import { CHOREO_ANNOTATIONS } from '@openchoreo/backstage-plugin-api';

export async function getCellDiagramInfo(
  entity: Entity,
  discovery: DiscoveryApi,
  identity: IdentityApi,
) {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('openchoreo')}${API_ENDPOINTS.CELL_DIAGRAM}`,
  );
  const project = entity.metadata.name;
  const organization =
    entity.metadata.annotations?.[CHOREO_ANNOTATIONS.ORGANIZATION];

  if (!project || !organization) {
    // TODO: Improve logging
    return [];
  }
  const params = new URLSearchParams({
    projectName: project,
    organizationName: organization,
  });

  backendUrl.search = params.toString();

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!res.ok) {
    throw new Error(
      `Failed to fetch cell diagram info: ${res.status} ${res.statusText}`,
    );
  }

  return await res.json();
}
