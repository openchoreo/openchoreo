import {
  coreServices,
  createBackendModule,
} from '@backstage/backend-plugin-api';
import { catalogProcessingExtensionPoint } from '@backstage/plugin-catalog-node/alpha';
import { OpenChoreoEntityProvider } from './provider/OpenChoreoEntityProvider';
import { EnvironmentEntityProcessor } from './processors';

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
        const openchoreoConfig = config.getOptionalConfig('openchoreo');
        const frequency =
          openchoreoConfig?.getOptionalNumber('schedule.frequency') ?? 30;
        const timeout =
          openchoreoConfig?.getOptionalNumber('schedule.timeout') ?? 120;

        const taskRunner = scheduler.createScheduledTaskRunner({
          frequency: { seconds: frequency },
          timeout: { seconds: timeout },
        });

        // Register the Environment entity processor
        catalog.addProcessor(new EnvironmentEntityProcessor());

        catalog.addEntityProvider(
          new OpenChoreoEntityProvider(taskRunner, logger, config),
        );
      },
    });
  },
});
