import {
  coreServices,
  createBackendModule,
} from '@backstage/backend-plugin-api';
import {
  catalogServiceRef,
  catalogProcessingExtensionPoint,
} from '@backstage/plugin-catalog-node/alpha';
import { ChoreoEntityProvider } from './provider/EntityProvider';

export const catalogModuleChoreoProvider = createBackendModule({
  pluginId: 'catalog',
  moduleId: 'choreo-provider',
  register(reg) {
    reg.registerInit({
      deps: {
        catalog: catalogProcessingExtensionPoint,
        logger: coreServices.logger,
        config: coreServices.rootConfig,
        discovery: coreServices.discovery,
        catalogApi: catalogServiceRef,
        permissions: coreServices.permissions,
        auth: coreServices.auth,
        httpAuth: coreServices.httpAuth,
        scheduler: coreServices.scheduler,
      },
      async init({
        catalog,
        logger,
        config,
        catalogApi,
        permissions,
        discovery,
        scheduler,
      }) {
        const taskRunner = scheduler.createScheduledTaskRunner({
          frequency: {
            seconds: config.getOptionalNumber(
              'choreoIngestor.taskRunner.frequency',
            ),
          },
          timeout: {
            seconds: config.getOptionalNumber(
              'choreoIngestor.taskRunner.timeout',
            ),
          },
        });
        const templateEntityProvider = new ChoreoEntityProvider(
          taskRunner,
          logger,
          config,
          catalogApi,
          permissions,
          discovery,
        );
        await catalog.addEntityProvider(templateEntityProvider);
      },
    });
  },
});
