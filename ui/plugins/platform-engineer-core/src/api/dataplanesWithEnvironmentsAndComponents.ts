import { DiscoveryApi, IdentityApi } from '@backstage/core-plugin-api';
import { CatalogApi } from '@backstage/catalog-client';
import { DataPlaneWithEnvironments } from '../types';

interface ComponentInfo {
  orgName: string;
  projectName: string;
  componentName: string;
}

export async function fetchDataplanesWithEnvironmentsAndComponents(
  discovery: DiscoveryApi,
  identity: IdentityApi,
  catalogApi: CatalogApi,
): Promise<DataPlaneWithEnvironments[]> {
  const { token } = await identity.getCredentials();

  // First, get the basic dataplanes with environments
  const dataplanesUrl = new URL(
    `${await discovery.getBaseUrl(
      'platform-engineer-core',
    )}/dataplanes-with-environments-and-components`,
  );

  const dataplanesRes = await fetch(dataplanesUrl, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });
  console.log('dataplanesRes', dataplanesRes);

  if (!dataplanesRes.ok) {
    throw new Error(
      `Failed to fetch dataplanes with environments: ${dataplanesRes.statusText}`,
    );
  }

  const dataplanesData = await dataplanesRes.json();
  if (!dataplanesData.success) {
    throw new Error(`API error: ${dataplanesData.error || 'Unknown error'}`);
  }

  const dataplanesWithEnvironments: DataPlaneWithEnvironments[] =
    dataplanesData.data || [];

  // Now get component counts using the bindings API
  try {
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
      // These might be stored in different ways depending on your setup
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

    // Call the backend to get component counts per environment using bindings API
    const componentCountsUrl = new URL(
      `${await discovery.getBaseUrl(
        'platform-engineer-core',
      )}/component-counts-per-environment`,
    );

    const componentCountsRes = await fetch(componentCountsUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        components: componentInfos,
      }),
    });

    if (componentCountsRes.ok) {
      const countsData = await componentCountsRes.json();
      if (countsData.success) {
        const componentCountsByEnvironment: Record<string, number> =
          countsData.data;

        // Update the environments with component counts
        const enrichedDataplanes = dataplanesWithEnvironments.map(
          dataplane => ({
            ...dataplane,
            environments: dataplane.environments.map(env => ({
              ...env,
              componentCount: componentCountsByEnvironment[env.name] || 0,
            })),
          }),
        );

        return enrichedDataplanes;
      }
    }

    console.warn(
      'Failed to fetch component counts, returning data without counts',
    );
    return dataplanesWithEnvironments;
  } catch (catalogError) {
    console.warn(
      'Failed to fetch component counts from catalog:',
      catalogError,
    );
    // Return the original data without component counts if catalog fails
    return dataplanesWithEnvironments;
  }
}
