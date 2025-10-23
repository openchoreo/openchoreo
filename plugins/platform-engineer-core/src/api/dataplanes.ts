import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { DataPlane } from '../types';

export async function fetchAllDataplanes(
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<DataPlane[]> {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('platform-engineer-core')}/dataplanes`,
  );

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!res.ok) {
    throw new Error(`Failed to fetch dataplanes: ${res.statusText}`);
  }

  const data = await res.json();
  if (!data.success) {
    throw new Error(`API error: ${data.error || 'Unknown error'}`);
  }

  return data.data || [];
}

export async function fetchDataplanesByOrganization(
  organizationName: string,
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<DataPlane[]> {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl(
      'platform-engineer-core',
    )}/dataplanes/${organizationName}`,
  );

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!res.ok) {
    throw new Error(
      `Failed to fetch dataplanes for ${organizationName}: ${res.statusText}`,
    );
  }

  const data = await res.json();
  if (!data.success) {
    throw new Error(`API error: ${data.error || 'Unknown error'}`);
  }

  return data.data || [];
}
