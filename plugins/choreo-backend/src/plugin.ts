import {
  coreServices,
  createBackendPlugin,
} from '@backstage/backend-plugin-api';
import { createRouter } from './router';
import { catalogServiceRef } from '@backstage/plugin-catalog-node/alpha';
import { EnvironmentInfoService } from './services/EnvironmentService/EnvironmentInfoService';
import { CellDiagramInfoService } from './services/CellDiagramService/CellDiagramInfoService';

/**
 * choreoPlugin backend plugin
 *
 * @public
 */
export const choreoPlugin = createBackendPlugin({
  pluginId: 'choreo',
  register(env) {
    env.registerInit({
      deps: {
        logger: coreServices.logger,
        auth: coreServices.auth,
        httpAuth: coreServices.httpAuth,
        httpRouter: coreServices.httpRouter,
        catalog: catalogServiceRef,
        permissions: coreServices.permissions,
        discovery: coreServices.discovery,
        config: coreServices.rootConfig,
      },
      async init({
        logger,
        config,
        httpAuth,
        httpRouter,
        catalog,
        permissions,
        discovery,
      }) {
        const environmentInfoService = await EnvironmentInfoService.create(
          logger,
          config,
          catalog,
          permissions,
          discovery,
        );

        const cellDiagramInfoService = await CellDiagramInfoService.create(
          logger,
          config,
          catalog,
          permissions,
          discovery,
        );

        httpRouter.use(
          await createRouter({
            httpAuth,
            environmentInfoService,
            cellDiagramInfoService,
          }),
        );
      },
    });
  },
});
