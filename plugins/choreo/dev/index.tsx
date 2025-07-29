import { createDevApp } from '@backstage/dev-utils';
import { choreoPlugin, Environments } from '../src/plugin';

createDevApp()
  .registerPlugin(choreoPlugin)
  .addPage({
    element: <Environments />,
    title: 'Root Page',
    path: '/choreo',
  })
  .render();
