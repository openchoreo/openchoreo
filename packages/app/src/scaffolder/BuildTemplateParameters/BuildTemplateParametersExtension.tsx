import { useEffect, useState } from 'react';
import { FieldExtensionComponentProps } from '@backstage/plugin-scaffolder-react';
import { 
  TextField, 
  FormControl, 
  FormHelperText, 
  Typography,
  Box,
  Checkbox,
  FormControlLabel
} from '@material-ui/core';
import { useApi, discoveryApiRef, identityApiRef } from '@backstage/core-plugin-api';
import type { ModelsBuildTemplate, BuildTemplateParameter } from '@internal/plugin-openchoreo-api';

/*
 Schema for the Build Template Parameters Field
*/
export const BuildTemplateParametersSchema = {
  returnValue: { 
    type: 'object',
    additionalProperties: true 
  },
};

/*
 This component dynamically renders form fields based on the selected build template's parameters
*/
export const BuildTemplateParameters = ({
  onChange,
  rawErrors,
  formData,
  formContext,
  idSchema,
}: FieldExtensionComponentProps<Record<string, any>>) => {
  const [parameters, setParameters] = useState<BuildTemplateParameter[]>([]);
  const [values, setValues] = useState<Record<string, any>>(formData || {});
  const [buildTemplates, setBuildTemplates] = useState<ModelsBuildTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  
  const discoveryApi = useApi(discoveryApiRef);
  const identityApi = useApi(identityApiRef);

  // Get the selected build template and organization from form context
  const selectedTemplateName = formContext?.formData?.build_template_name;
  const organizationName = formContext?.formData?.organization_name;

  // Fetch build templates when organization changes
  useEffect(() => {
    let ignore = false
    const fetchBuildTemplates = async () => {
      if (!organizationName) {
        setBuildTemplates([]);
        return;
      }

      // Extract the actual organization name from the entity reference format
      const extractOrgName = (fullOrgName: string): string => {
        const parts = fullOrgName.split('/');
        return parts[parts.length - 1];
      };

      const orgName = extractOrgName(organizationName);

      setLoading(true);
      
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
        if (!ignore) setBuildTemplates(templates);
      } catch (err) {
        setBuildTemplates([]);
      } finally {
        setLoading(false);
      }
    };

    fetchBuildTemplates();
    return () => {
      ignore = true
    };
  }, [organizationName, discoveryApi, identityApi]);

  // Update parameters when template selection or templates change
  useEffect(() => {
    if (selectedTemplateName && buildTemplates.length > 0) {
      const selectedTemplate = buildTemplates.find(
        t => t.name === selectedTemplateName,
      );

      if (selectedTemplate?.parameters) {
        setParameters(selectedTemplate.parameters);

        // Initialize default values for new parameters
        const newValues: Record<string, any> = { ...values };
        selectedTemplate.parameters.forEach(param => {
          if (!(param.name in newValues) && param.default !== undefined) {
            newValues[param.name] = param.default;
          }
        });
        setValues(newValues);
        onChange(newValues);
      } else {
        setParameters([]);
      }
    } else {
      setParameters([]);
    }
  }, [selectedTemplateName, buildTemplates, onChange, values]);

  const handleFieldChange = (paramName: string, value: any) => {
    const newValues = { ...values, [paramName]: value };
    setValues(newValues);
    onChange(newValues);
  };

  const renderParameterField = (param: BuildTemplateParameter) => {
    const value = values[param.name] || '';
    const fieldId = `${idSchema?.$id}-${param.name}`;
    
    switch (param.type) {
      case 'boolean': {
        let booleanHelperText: string | undefined;
        if (param.description) {
          booleanHelperText = param.description;
          if (param.default) {
            booleanHelperText += ` (Default: ${param.default})`;
          }
        } else if (param.default) {
          booleanHelperText = `Default value: ${param.default}`;
        } else {
          booleanHelperText = undefined;
        }
        
        return (
          <Box>
            <FormControlLabel
              control={
                <Checkbox
                  checked={value === true || value === 'true'}
                  onChange={(e) => handleFieldChange(param.name, e.target.checked)}
                  name={param.name}
                />
              }
              label={param.displayName || param.name}
            />
            {booleanHelperText && (
              <FormHelperText style={{ marginLeft: 32 }}>
                {booleanHelperText}
              </FormHelperText>
            )}
          </Box>
        );
      }
      
      case 'number':
        return (
          <TextField
            id={fieldId}
            label={param.displayName || param.name}
            type="number"
            value={value}
            onChange={(e) => handleFieldChange(param.name, parseInt(e.target.value, 10))}
            fullWidth
            required={param.required}
            placeholder={param.default ? `Default: ${param.default}` : undefined}
            helperText={param.description || (param.default ? `Default value: ${param.default}` : undefined)}
            error={!!rawErrors?.find(err => err.includes(param.name))}
          />
        );
      
      default: // string or unspecified
        return (
          <TextField
            id={fieldId}
            label={param.displayName || param.name}
            value={value}
            onChange={(e) => handleFieldChange(param.name, e.target.value)}
            fullWidth
            required={param.required}
            placeholder={param.default ? `Default: ${param.default}` : undefined}
            helperText={param.description || (param.default ? `Default value: ${param.default}` : undefined)}
            error={!!rawErrors?.find(err => err.includes(param.name))}
          />
        );
    }
  };

  if (!selectedTemplateName) {
    return (
      <Box mt={2}>
        <Typography variant="body2" color="textSecondary">
          Please select a build template first
        </Typography>
      </Box>
    );
  }

  if (loading) {
    return (
      <Box mt={2}>
        <Typography variant="body2" color="textSecondary">
          Loading template parameters...
        </Typography>
      </Box>
    );
  }

  if (parameters.length === 0) {
    return (
      <Box mt={2}>
        <Typography variant="body2" color="textSecondary">
          No additional parameters required for this template
        </Typography>
      </Box>
    );
  }

  return (
    <FormControl fullWidth margin="normal">
      <Typography variant="subtitle1" gutterBottom>
        Build Template Parameters
      </Typography>
      <Box display="flex" flexDirection="column">
        {parameters.map((param, index) => (
          <Box key={param.name} mb={index < parameters.length - 1 ? 2 : 0}>
            {renderParameterField(param)}
          </Box>
        ))}
      </Box>
      {rawErrors?.length ? (
        <FormHelperText error>{rawErrors.join(', ')}</FormHelperText>
      ) : null}
    </FormControl>
  );
};

/*
 Validation function for build template parameters
*/
export const buildTemplateParametersValidation = (
  value: Record<string, any>,
  validation: any,
  { formContext }: any
) => {
  if (!formContext) {
    return;
  }
  
  const selectedTemplateName = formContext.formData?.build_template_name;
  const buildTemplates: ModelsBuildTemplate[] = formContext.buildTemplates || [];
  
  if (selectedTemplateName && buildTemplates.length > 0) {
    const selectedTemplate = buildTemplates.find(t => t.name === selectedTemplateName);
    if (selectedTemplate?.parameters) {
      selectedTemplate.parameters.forEach(param => {
        if (param.required && (!value || !value[param.name])) {
          validation.addError(`${param.displayName || param.name} is required`);
        }
      });
    }
  }
};
