import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { DataPlaneWithEnvironments } from '../types';

export async function fetchDataplanesWithEnvironments(
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<DataPlaneWithEnvironments[]> {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl(
      'platform-engineer-core',
    )}/dataplanes-with-environments`,
  );

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!res.ok) {
    throw new Error(
      `Failed to fetch dataplanes with environments: ${res.statusText}`,
    );
  }

  const data = await res.json();
  if (!data.success) {
    throw new Error(`API error: ${data.error || 'Unknown error'}`);
  }

  return data.data || [];
}
