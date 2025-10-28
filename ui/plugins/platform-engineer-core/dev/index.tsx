import { createDevApp } from '@backstage/dev-utils';
import {
  platformEngineerCorePlugin,
  PlatformEngineerViewPage,
} from '../src/plugin';

createDevApp()
  .registerPlugin(platformEngineerCorePlugin)
  .addPage({
    element: <PlatformEngineerViewPage />,
    title: 'Platform Engineer View',
    path: '/platform-engineer-view',
  })
  .render();
