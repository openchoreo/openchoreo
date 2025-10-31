import { createTemplateAction } from '@backstage/plugin-scaffolder-node';
import { OpenChoreoApiClient } from '@openchoreo/backstage-plugin-api';
import { Config } from '@backstage/config';
import { z } from 'zod';

export const createComponentAction = (config: Config) => {
  return createTemplateAction({
    id: 'openchoreo:component:create',
    description: 'Create OpenChoreo Component',
    schema: {
      input: (zImpl: typeof z) =>
        zImpl.object({
          orgName: zImpl
            .string()
            .describe(
              'The name of the organization where the component will be created',
            ),
          projectName: zImpl
            .string()
            .describe(
              'The name of the project where the component will be created',
            ),
          componentName: zImpl
            .string()
            .describe('The name of the component to create'),
          displayName: zImpl
            .string()
            .optional()
            .describe('The display name of the component'),
          description: zImpl
            .string()
            .optional()
            .describe('The description of the component'),
          componentType: zImpl
            .string()
            .describe(
              'The type of the component (e.g., Service, WebApp, ScheduledTask, APIProxy)',
            ),
          useBuiltInCI: zImpl
            .boolean()
            .optional()
            .describe('Whether to use built-in CI in OpenChoreo'),
          repoUrl: zImpl
            .string()
            .optional()
            .describe(
              'The URL of the repository containing the component source code',
            ),
          branch: zImpl
            .string()
            .optional()
            .describe('The branch of the repository to use'),
          componentPath: zImpl
            .string()
            .optional()
            .describe(
              'The path within the repository where the component source code is located',
            ),
          buildTemplateName: zImpl
            .string()
            .optional()
            .describe(
              'The name of the build template to use (e.g., java-maven, nodejs-npm)',
            ),
          buildParameters: zImpl
            .record(zImpl.any())
            .optional()
            .describe('Parameters specific to the selected build template'),
        }),
      output: (zImpl: typeof z) =>
        zImpl.object({
          componentName: zImpl
            .string()
            .describe('The name of the created component'),
          projectName: zImpl
            .string()
            .describe('The project where the component was created'),
          organizationName: zImpl
            .string()
            .describe('The organization where the component was created'),
        }),
    },
    async handler(ctx) {
      ctx.logger.info(
        `Creating component with parameters: ${JSON.stringify(ctx.input)}`,
      );

      // Extract organization name from domain format (e.g., "domain:default/default-org" -> "default-org")
      const extractOrgName = (fullOrgName: string): string => {
        const parts = fullOrgName.split('/');
        return parts[parts.length - 1];
      };

      // Extract project name from system format (e.g., "system:default/project-name" -> "project-name")
      const extractProjectName = (fullProjectName: string): string => {
        const parts = fullProjectName.split('/');
        return parts[parts.length - 1];
      };

      const orgName = extractOrgName(ctx.input.orgName);
      const projectName = extractProjectName(ctx.input.projectName);

      ctx.logger.info(
        `Extracted organization name: ${orgName} from ${ctx.input.orgName}`,
      );
      ctx.logger.info(
        `Extracted project name: ${projectName} from ${ctx.input.projectName}`,
      );

      // Get the base URL from configuration
      const baseUrl = config.getString('openchoreo.baseUrl');

      // Create a new instance of the OpenChoreoApiClient
      const client = new OpenChoreoApiClient(baseUrl, '', ctx.logger);

      // Build configuration for built-in CI
      let buildConfig = undefined;
      if (
        ctx.input.useBuiltInCI &&
        ctx.input.repoUrl &&
        ctx.input.branch &&
        ctx.input.componentPath &&
        ctx.input.buildTemplateName
      ) {
        // Convert buildParameters object to array of TemplateParameter
        let buildTemplateParams = undefined;
        if (
          ctx.input.buildParameters &&
          Object.keys(ctx.input.buildParameters).length > 0
        ) {
          buildTemplateParams = Object.entries(ctx.input.buildParameters).map(
            ([name, value]) => ({
              name,
              value: String(value),
            }),
          );
        }

        buildConfig = {
          repoUrl: ctx.input.repoUrl,
          repoBranch: ctx.input.branch,
          componentPath: ctx.input.componentPath,
          buildTemplateRef: ctx.input.buildTemplateName,
          buildTemplateParams,
        };
        ctx.logger.info(
          `Build configuration created: ${JSON.stringify(buildConfig)}`,
        );
      }

      try {
        const response = await client.createComponent(orgName, projectName, {
          name: ctx.input.componentName,
          displayName: ctx.input.displayName,
          description: ctx.input.description,
          type: ctx.input.componentType,
          buildConfig,
        });

        ctx.logger.info(
          `Component created successfully: ${JSON.stringify(response)}`,
        );

        // Set outputs for the scaffolder
        ctx.output('componentName', response.name);
        ctx.output('projectName', projectName);
        ctx.output('organizationName', orgName);
      } catch (error) {
        ctx.logger.error(`Error creating component: ${error}`);
        throw new Error(`Failed to create component: ${error}`);
      }
    },
  });
};
