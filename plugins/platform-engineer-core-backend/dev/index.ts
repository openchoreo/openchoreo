import { createBackend } from '@backstage/backend-defaults';
import { platformEngineerViewPlugin } from '../src/plugin';

const backend = createBackend();
backend.add(platformEngineerViewPlugin);
backend.start();
