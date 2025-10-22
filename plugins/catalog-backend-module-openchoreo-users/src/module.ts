import {
  coreServices,
  createBackendModule,
} from '@backstage/backend-plugin-api';

export const catalogModuleOpenchoreoUsers = createBackendModule({
  pluginId: 'catalog',
  moduleId: 'openchoreo-users',
  register(reg) {
    reg.registerInit({
      deps: { logger: coreServices.logger },
      async init({ logger }) {
        logger.info('Hello World!');
      },
    });
  },
});
