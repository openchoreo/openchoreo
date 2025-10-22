import {
  coreServices,
  createBackendModule,
} from '@backstage/backend-plugin-api';
import { catalogProcessingExtensionPoint } from '@backstage/plugin-catalog-node/alpha';
import { ThunderUserGroupEntityProvider } from './provider/ThunderUserGroupEntityProvider';

export const catalogModuleOpenchoreoUsers = createBackendModule({
  pluginId: 'catalog',
  moduleId: 'openchoreo-users',
  register(reg) {
    reg.registerInit({
      deps: {
        catalog: catalogProcessingExtensionPoint,
        config: coreServices.rootConfig,
        logger: coreServices.logger,
        scheduler: coreServices.scheduler,
      },
      async init({ catalog, config, logger, scheduler }) {
        // Read schedule configuration from app-config.yaml
        const thunderConfig = config.getOptionalConfig('thunder');
        const frequency =
          thunderConfig?.getOptionalNumber('schedule.frequency') ?? 600; // Default: 10 minutes
        const timeout =
          thunderConfig?.getOptionalNumber('schedule.timeout') ?? 300; // Default: 5 minutes

        // Create a scheduled task runner
        const taskRunner = scheduler.createScheduledTaskRunner({
          frequency: { seconds: frequency },
          timeout: { seconds: timeout },
        });

        // Create and register the Thunder User & Group Entity Provider
        const provider = new ThunderUserGroupEntityProvider(
          taskRunner,
          logger,
          config,
        );

        catalog.addEntityProvider(provider);

        logger.info(
          'Thunder User & Group Entity Provider registered successfully',
        );
      },
    });
  },
});
