import {
  coreServices,
  createBackendPlugin,
} from '@backstage/backend-plugin-api';
import { createRouter } from './router';
import { catalogServiceRef } from '@backstage/plugin-catalog-node/alpha';
import { EnvironmentInfoService } from './services/EnvironmentService/EnvironmentInfoService';
import { CellDiagramInfoService } from './services/CellDiagramService/CellDiagramInfoService';
import { BuildTemplateInfoService } from './services/BuildTemplateService/BuildTemplateInfoService';
import { BuildInfoService } from './services/BuildService/BuildInfoService';
import { ComponentInfoService } from './services/ComponentService/ComponentInfoService';
import { RuntimeLogsInfoService } from './services/RuntimeLogsService/RuntimeLogsService';
import { WorkloadInfoService } from './services/WorkloadService/WorkloadInfoService';
import { ObservabilityApiClient } from '@internal/plugin-openchoreo-api';

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
      async init({ logger, config, httpRouter }) {
        const openchoreoConfig = config.getConfig('openchoreo'); // Make optional

        const environmentInfoService = new EnvironmentInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
          openchoreoConfig.getOptional('token'),
        );

        const cellDiagramInfoService = new CellDiagramInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
        );

        const buildTemplateInfoService = new BuildTemplateInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
        );

        const buildInfoService = new BuildInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
        );

        const componentInfoService = new ComponentInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
        );

        const runtimeLogsInfoService = new RuntimeLogsInfoService(
          logger,
          new ObservabilityApiClient(
            openchoreoConfig.get('observabilityBaseUrl'),
            {},
          ),
        );

        const workloadInfoService = new WorkloadInfoService(
          logger,
          openchoreoConfig.get('baseUrl'),
          openchoreoConfig.getOptional('token'),
        );

        httpRouter.use(
          await createRouter({
            environmentInfoService,
            cellDiagramInfoService,
            buildTemplateInfoService,
            buildInfoService,
            componentInfoService,
            runtimeLogsInfoService,
            workloadInfoService,
          }),
        );
      },
    });
  },
});
