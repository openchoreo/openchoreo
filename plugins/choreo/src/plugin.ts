import {
  createPlugin,
  createRoutableExtension,
} from '@backstage/core-plugin-api';
import { rootCatalogEnvironmentRouteRef } from './routes';

export const choreoPlugin = createPlugin({
  id: 'choreo',
});

// Component page tab
export const Environments = choreoPlugin.provide(
  createRoutableExtension({
    name: 'ChoreoEnvironments',
    component: () =>
      import('./components/Environments').then(m => m.Environments),
    mountPoint: rootCatalogEnvironmentRouteRef,
  }),
);

// System entity page tab
export const CellDiagram = choreoPlugin.provide(
  createRoutableExtension({
    name: 'ChoreoSystemTab',
    component: () =>
      import('./components/CellDiagram/CellDiagram').then(m => m.CellDiagram),
    mountPoint: rootCatalogEnvironmentRouteRef,
  }),
);
