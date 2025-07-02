import {
  coreServices,
  createBackendModule,
} from '@backstage/backend-plugin-api';
import { catalogProcessingExtensionPoint } from '@backstage/plugin-catalog-node/alpha';
import { OpenChoreoEntityProvider } from './provider/OpenChoreoEntityProvider';

/**
 * OpenChoreo catalog backend module
 *
 * @public
 */
export const catalogModuleOpenchoreo = createBackendModule({
  pluginId: 'catalog',
  moduleId: 'openchoreo',
  register(env) {
    env.registerInit({
      deps: {
        catalog: catalogProcessingExtensionPoint,
        config: coreServices.rootConfig,
        logger: coreServices.logger,
        scheduler: coreServices.scheduler,
      },
      async init({ catalog, config, logger, scheduler }) {
        const taskRunner = scheduler.createScheduledTaskRunner({
          frequency: { seconds: 30 }, // Run every 30 seconds
          timeout: { minutes: 2 },
        });

        catalog.addEntityProvider(
          new OpenChoreoEntityProvider(taskRunner, logger, config),
        );
      },
    });
  },
});
