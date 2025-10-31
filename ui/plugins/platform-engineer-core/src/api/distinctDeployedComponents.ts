import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { CatalogApi } from '@backstage/catalog-client';

interface ComponentInfo {
  orgName: string;
  projectName: string;
  componentName: string;
}

export async function fetchDistinctDeployedComponentsCount(
  discovery: DiscoveryApi,
  identity: IdentityApi,
  catalogApi: CatalogApi,
): Promise<number> {
  try {
    const { token } = await identity.getCredentials();

    // Fetch all components from the catalog
    const components = await catalogApi.getEntities({
      filter: {
        kind: 'Component',
      },
    });

    // Extract component information needed for bindings API
    const componentInfos: ComponentInfo[] = [];

    components.items.forEach(component => {
      const annotations = component.metadata.annotations || {};

      // Extract org, project, and component name from annotations or metadata
      const orgName =
        annotations['openchoreo.org/organization'] ||
        annotations['backstage.io/managed-by-location']?.split('/')[3] ||
        'default'; // fallback

      const projectName =
        annotations['openchoreo.org/project'] ||
        component.metadata.namespace ||
        'default'; // fallback

      const componentName = component.metadata.name;

      if (orgName && projectName && componentName) {
        componentInfos.push({
          orgName,
          projectName,
          componentName,
        });
      }
    });

    // Call the backend to get distinct deployed components count
    const distinctCountUrl = new URL(
      `${await discovery.getBaseUrl(
        'platform-engineer-core',
      )}/distinct-deployed-components-count`,
    );

    const distinctCountRes = await fetch(distinctCountUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        components: componentInfos,
      }),
    });

    if (distinctCountRes.ok) {
      const countData = await distinctCountRes.json();
      if (countData.success) {
        return countData.data;
      }
    }

    console.warn('Failed to fetch distinct deployed components count');
    return 0;
  } catch (error) {
    console.warn('Failed to fetch distinct deployed components count:', error);
    return 0;
  }
}
