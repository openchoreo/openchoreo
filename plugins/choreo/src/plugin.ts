import {
  createPlugin,
  createRoutableExtension,
} from '@backstage/core-plugin-api';
import {
  rootCatalogEnvironmentRouteRef,
  rootCatalogRuntimeLogsRouteRef,
} from './routes';

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

// Runtime logs page tab
export const RuntimeLogs = choreoPlugin.provide(
  createRoutableExtension({
    name: 'ChoreoRuntimeLogs',
    component: () =>
      import('./components/RuntimeLogs/RuntimeLogs').then(m => m.RuntimeLogs),
    mountPoint: rootCatalogRuntimeLogsRouteRef,
  }),
);
