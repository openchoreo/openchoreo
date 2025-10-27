import { scaffolderPlugin } from '@backstage/plugin-scaffolder';
import { createScaffolderFieldExtension } from '@backstage/plugin-scaffolder-react';
import {
  BuildTemplateParameters,
  buildTemplateParametersValidation,
} from './BuildTemplateParametersExtension';

export const BuildTemplateParametersFieldExtension = scaffolderPlugin.provide(
  createScaffolderFieldExtension({
    name: 'BuildTemplateParameters',
    component: BuildTemplateParameters,
    validation: buildTemplateParametersValidation,
  }),
);
