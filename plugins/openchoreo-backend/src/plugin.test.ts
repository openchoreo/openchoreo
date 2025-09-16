import { startTestBackend } from '@backstage/backend-test-utils';
import { choreoPlugin } from './plugin';
import { catalogServiceMock } from '@backstage/plugin-catalog-node/testUtils';

// TEMPLATE NOTE:
// Plugin tests are integration tests for your plugin, ensuring that all pieces
// work together end-to-end. You can still mock injected backend services
// however, just like anyone who installs your plugin might replace the
// services with their own implementations.
// Basic plugin startup tests - OpenChoreo functionality tests to be added
describe('plugin', () => {
  it('should start the plugin without config', async () => {
    const { server } = await startTestBackend({
      features: [choreoPlugin],
    });

    expect(server).toBeDefined();
  });

  it('should start the plugin with catalog service', async () => {
    const { server } = await startTestBackend({
      features: [
        choreoPlugin,
        catalogServiceMock.factory({
          entities: [
            {
              apiVersion: 'backstage.io/v1alpha1',
              kind: 'Component',
              metadata: {
                name: 'my-component',
                namespace: 'default',
                title: 'My Component',
              },
              spec: {
                type: 'service',
                owner: 'me',
              },
            },
          ],
        }),
      ],
    });

    expect(server).toBeDefined();
  });
});
