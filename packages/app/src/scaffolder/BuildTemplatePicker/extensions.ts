/*
  This is where the magic happens and creates the custom field extension.
*/

import { scaffolderPlugin } from '@backstage/plugin-scaffolder';
import { createScaffolderFieldExtension } from '@backstage/plugin-scaffolder-react';
import {
  BuildTemplatePicker,
  buildTemplatePickerValidation,
  // BuildTemplatePickerSchema,
} from './BuildTemplatePickerExtension';

export const BuildTemplatePickerFieldExtension = scaffolderPlugin.provide(
  createScaffolderFieldExtension({
    name: 'BuildTemplatePicker',
    component: BuildTemplatePicker,
    validation: buildTemplatePickerValidation,
    // schema: BuildTemplatePickerSchema,
  }),
);