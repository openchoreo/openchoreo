import { createBackendModule } from "@backstage/backend-plugin-api";
import { scaffolderActionsExtensionPoint  } from '@backstage/plugin-scaffolder-node/alpha';
import { coreServices } from '@backstage/backend-plugin-api';
import { createProjectAction } from "./actions/project";

/**
 * A backend module that registers the action into the scaffolder
 */
export const scaffolderModule = createBackendModule({
  moduleId: 'openchoreo:project:create',
  pluginId: 'scaffolder',
  register({ registerInit }) {
    registerInit({
      deps: {
        scaffolderActions: scaffolderActionsExtensionPoint,
        config: coreServices.rootConfig
      },
      async init({ scaffolderActions, config}) {
        scaffolderActions.addActions(createProjectAction(config));
      }
    });
  },
})
