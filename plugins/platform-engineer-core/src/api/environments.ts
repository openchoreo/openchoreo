import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { Environment } from '../types';

export async function fetchAllEnvironments(
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<Environment[]> {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl('platform-engineer-core')}/environments`,
  );

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!res.ok) {
    throw new Error(`Failed to fetch environments: ${res.statusText}`);
  }

  const data = await res.json();
  if (!data.success) {
    throw new Error(`API error: ${data.error || 'Unknown error'}`);
  }

  return data.data || [];
}

export async function fetchEnvironmentsByOrganization(
  organizationName: string,
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<Environment[]> {
  const { token } = await identity.getCredentials();
  const backendUrl = new URL(
    `${await discovery.getBaseUrl(
      'platform-engineer-core',
    )}/environments/${organizationName}`,
  );

  const res = await fetch(backendUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!res.ok) {
    throw new Error(
      `Failed to fetch environments for ${organizationName}: ${res.statusText}`,
    );
  }

  const data = await res.json();
  if (!data.success) {
    throw new Error(`API error: ${data.error || 'Unknown error'}`);
  }

  return data.data || [];
}
