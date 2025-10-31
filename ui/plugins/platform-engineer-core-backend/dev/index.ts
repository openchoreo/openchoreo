import { createBackend } from '@backstage/backend-defaults';
import { platformEngineerCorePlugin } from '../src/plugin';

const backend = createBackend();
backend.add(platformEngineerCorePlugin);
backend.start();
