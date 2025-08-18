import { createBackendModule } from '@backstage/backend-plugin-api';
import { scaffolderActionsExtensionPoint } from '@backstage/plugin-scaffolder-node/alpha';
import { coreServices } from '@backstage/backend-plugin-api';
import { createProjectAction } from './actions/project';
import { createComponentAction } from './actions/component';

/**
 * A backend module that registers the actions into the scaffolder
 */
export const scaffolderModule = createBackendModule({
  moduleId: 'openchoreo-scaffolder-actions',
  pluginId: 'scaffolder',
  register({ registerInit }) {
    registerInit({
      deps: {
        scaffolderActions: scaffolderActionsExtensionPoint,
        config: coreServices.rootConfig,
      },
      async init({ scaffolderActions, config }) {
        scaffolderActions.addActions(
          createProjectAction(config),
          createComponentAction(config),
        );
      },
    });
  },
});
