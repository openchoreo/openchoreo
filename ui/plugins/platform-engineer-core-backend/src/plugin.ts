import {
  coreServices,
  createBackendPlugin,
} from '@backstage/backend-plugin-api';
import { createRouter } from './router';
import { PlatformEnvironmentInfoService } from './services/PlatformEnvironmentService';

/**
 * platformEngineerCorePlugin backend plugin
 *
 * @public
 */
export const platformEngineerCorePlugin = createBackendPlugin({
  pluginId: 'platform-engineer-core',
  register(env) {
    env.registerInit({
      deps: {
        logger: coreServices.logger,
        httpRouter: coreServices.httpRouter,
        config: coreServices.rootConfig,
      },
      async init({ logger, config, httpRouter }) {
        const openchoreoConfig = config.getOptionalConfig('openchoreo');

        if (!openchoreoConfig) {
          logger.info(
            'Platform Engineer Core plugin disabled - no OpenChoreo configuration found',
          );
          return;
        }

        const platformEnvironmentService = new PlatformEnvironmentInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
          openchoreoConfig.getOptional('token'),
        );

        httpRouter.use(
          await createRouter({
            platformEnvironmentService,
          }),
        );

        logger.info('Platform Engineer Core backend plugin initialized');
      },
    });
  },
});
