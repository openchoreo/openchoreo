import { ChangeEvent, useEffect, useState } from 'react';
import { FieldExtensionComponentProps } from '@backstage/plugin-scaffolder-react';
import type { FieldValidation } from '@rjsf/utils';
import { FormControl, InputLabel, Select, MenuItem, CircularProgress, FormHelperText } from '@material-ui/core';
import { useApi, discoveryApiRef, identityApiRef } from '@backstage/core-plugin-api';
import type { ModelsBuildTemplate } from '@internal/plugin-openchoreo-api';

/*
 Schema for the Custom Field Explorer
*/
export const BuildTemplatePickerSchema = {
  returnValue: { type: 'string' },
};

/*
 This is the actual component that will get rendered in the form
*/
export const BuildTemplatePicker = ({
  onChange,
  rawErrors,
  required,
  formData,
  formContext,
  idSchema,
  uiSchema,
  schema,
}: FieldExtensionComponentProps<string>) => {
  const [buildTemplates, setBuildTemplates] = useState<ModelsBuildTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const discoveryApi = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);

  // Get the organization name from form context
  const organizationName = formContext.formData?.organization_name;

  useEffect(() => {
    const fetchBuildTemplates = async () => {
      if (!organizationName) {
        setBuildTemplates([]);
        // Clear templates from form context when no organization
        if (formContext.buildTemplates) {
          delete formContext.buildTemplates;
        }
        return;
      }

      // Extract the actual organization name from the entity reference format
      // e.g., "domain:default/my-org" -> "my-org"
      const extractOrgName = (fullOrgName: string): string => {
        const parts = fullOrgName.split('/');
        return parts[parts.length - 1];
      };

      const orgName = extractOrgName(organizationName);

      setLoading(true);
      setError(null);
      
      try {
        const { token } = await identityApi.getCredentials();
        const baseUrl = await discoveryApi.getBaseUrl('choreo');
        const response = await fetch(
          `${baseUrl}/build-templates?organizationName=${encodeURIComponent(orgName)}`, {
            headers: {
              Authorization: `Bearer ${token}`,
            },
          }
        );
        
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const templates = await response.json();
        setBuildTemplates(templates);
        
        // Store templates in form context so other components can access them
        formContext.buildTemplates = templates;
      } catch (err) {
        setError(`Failed to fetch build templates: ${err}`);
        console.error('Error fetching build templates:', err);
        setBuildTemplates([]);
        // Clear templates from form context on error
        if (formContext.buildTemplates) {
          delete formContext.buildTemplates;
        }
      } finally {
        setLoading(false);
      }
    };

    fetchBuildTemplates();
  }, [organizationName, discoveryApi]);

  const handleChange = (event: ChangeEvent<{ value: unknown }>) => {
    onChange(event.target.value as string);
    // Ensure templates are still in form context when selection changes
    if (buildTemplates.length > 0) {
      formContext.buildTemplates = buildTemplates;
    }
  };

  return (
    <FormControl 
      fullWidth 
      margin="normal"
      error={!!rawErrors?.length}
      required={required}
    >
      <InputLabel id={`${idSchema?.$id}-label`}>
        {uiSchema?.['ui:title'] || schema.title || 'Build Template'}
      </InputLabel>
      <Select
        labelId={`${idSchema?.$id}-label`}
        value={formData || ''}
        onChange={handleChange}
        disabled={loading || !organizationName}
      >
        {loading && (
          <MenuItem disabled>
            <CircularProgress size={20} style={{ marginRight: 8 }} />
            Loading build templates...
          </MenuItem>
        )}
        {!loading && buildTemplates.length === 0 && !error && (
          <MenuItem disabled>
            {organizationName ? 'No build templates available' : 'Select an organization first'}
          </MenuItem>
        )}
        {!loading && buildTemplates.map((template) => (
          <MenuItem key={template.name} value={template.name}>
            {template.name}
          </MenuItem>
        ))}
      </Select>
      {error && <FormHelperText>{error}</FormHelperText>}
      {rawErrors?.length ? (
        <FormHelperText>{rawErrors.join(', ')}</FormHelperText>
      ) : null}
      {schema.description && !rawErrors?.length && (
        <FormHelperText>{schema.description}</FormHelperText>
      )}
    </FormControl>
  );
};

/*
 This is a validation function that will run when the form is submitted.
*/
export const buildTemplatePickerValidation = (
  value: string,
  validation: FieldValidation,
) => {
  if (!value || value.trim() === '') {
    validation.addError('Build template is required when using built-in CI');
  }
};
