import {
  createPlugin,
  createRoutableExtension,
} from '@backstage/core-plugin-api';
import { rootRouteRef } from './routes';

export const platformEngineerCorePlugin = createPlugin({
  id: 'platform-engineer-core',
  routes: {
    root: rootRouteRef,
  },
});

export const PlatformEngineerViewPage = platformEngineerCorePlugin.provide(
  createRoutableExtension({
    name: 'PlatformEngineerViewPage',
    component: () =>
      import('./views/PlatformEngineerDashboardView').then(
        m => m.PlatformEngineerDashboardView,
      ),
    mountPoint: rootRouteRef,
  }),
);
