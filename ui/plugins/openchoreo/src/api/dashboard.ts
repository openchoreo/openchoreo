import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { API_ENDPOINTS } from '../constants';

export interface ComponentInfo {
  orgName: string;
  projectName: string;
  componentName: string;
}

export async function fetchTotalBindingsCount(
  components: ComponentInfo[],
  discovery: DiscoveryApi,
  identity: IdentityApi,
): Promise<number> {
  const { token } = await identity.getCredentials();
  const backendUrl = `${await discovery.getBaseUrl('openchoreo')}${
    API_ENDPOINTS.DASHBOARD_BINDINGS_COUNT
  }`;

  const res = await fetch(backendUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ components }),
  });
  console.log(res);

  if (!res.ok) {
    throw new Error('Failed to fetch bindings count');
  }

  const data = await res.json();
  return data.totalBindings;
}
