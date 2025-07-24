import { InputError } from '@backstage/errors';
import express from 'express';
import Router from 'express-promise-router';
import { EnvironmentInfoService } from './services/EnvironmentService/EnvironmentInfoService';
import { BuildTemplateInfoService } from './services/BuildTemplateService/BuildTemplateInfoService';
import { BuildInfoService } from './services/BuildService/BuildInfoService';
import { CellDiagramService } from './types';
import { ComponentInfoService } from './services/ComponentService/ComponentInfoService';
import { RuntimeLogsInfoService } from './services/RuntimeLogsService/RuntimeLogsService';

export async function createRouter({
  environmentInfoService,
  cellDiagramInfoService,
  buildTemplateInfoService,
  buildInfoService,
  componentInfoService,
  runtimeLogsInfoService,
}: {
  environmentInfoService: EnvironmentInfoService;
  cellDiagramInfoService: CellDiagramService;
  buildTemplateInfoService: BuildTemplateInfoService;
  buildInfoService: BuildInfoService;
  componentInfoService: ComponentInfoService;
  runtimeLogsInfoService: RuntimeLogsInfoService;
}): Promise<express.Router> {
  const router = Router();
  router.use(express.json());

  router.get('/deploy', async (req, res) => {
    const { componentName, projectName, organizationName } = req.query;

    if (!componentName || !projectName || !organizationName) {
      throw new InputError(
        'componentName, projectName and organizationName are required query parameters',
      );
    }

    res.json(
      await environmentInfoService.fetchDeploymentInfo({
        componentName: componentName as string,
        projectName: projectName as string,
        organizationName: organizationName as string, // TODO: Get from request or config
      }),
    );
  });

  router.post('/promote-deployment', async (req, res) => {
    const { sourceEnv, targetEnv, componentName, projectName, orgName } = req.body;

    if (!sourceEnv || !targetEnv || !componentName || !projectName || !orgName) {
      throw new InputError(
        'sourceEnv, targetEnv, componentName, projectName and orgName are required in request body',
      );
    }

    res.json(
      await environmentInfoService.promoteComponent({
        sourceEnvironment: sourceEnv,
        targetEnvironment: targetEnv,
        componentName: componentName as string,
        projectName: projectName as string,
        organizationName: orgName as string,
      }),
    );
  });

  router.get(
    '/cell-diagram',
    async (req: express.Request, res: express.Response) => {
      const { projectName, organizationName } = req.query;

      if (!projectName || !organizationName) {
        throw new InputError(
          'projectName and organizationName are required query parameters',
        );
      }
      res.json(
        await cellDiagramInfoService.fetchProjectInfo({
          projectName: projectName as string,
          orgName: organizationName as string,
        }),
      );
    },
  );

  router.get('/build-templates', async (req, res) => {
    const { organizationName } = req.query;

    if (!organizationName) {
      throw new InputError('organizationName is a required query parameter');
    }

    res.json(
      await buildTemplateInfoService.fetchBuildTemplates(
        organizationName as string,
      ),
    );
  });

  router.get('/builds', async (req, res) => {
    const { componentName, projectName, organizationName } = req.query;

    if (!componentName || !projectName || !organizationName) {
      throw new InputError(
        'componentName, projectName and organizationName are required query parameters',
      );
    }

    res.json(
      await buildInfoService.fetchBuilds(
        organizationName as string,
        projectName as string,
        componentName as string,
      ),
    );
  });

  router.post('/builds', async (req, res) => {
    const { componentName, projectName, organizationName, commit } = req.body;

    if (!componentName || !projectName || !organizationName) {
      throw new InputError(
        'componentName, projectName and organizationName are required in request body',
      );
    }

    res.json(
      await buildInfoService.triggerBuild(
        organizationName as string,
        projectName as string,
        componentName as string,
        commit as string | undefined,
      ),
    );
  });

  router.get('/component', async (req, res) => {
    const { componentName, projectName, organizationName } = req.query;

    if (!componentName || !projectName || !organizationName) {
      throw new InputError(
        'componentName, projectName and organizationName are required query parameters',
      );
    }

    res.json(
      await componentInfoService.fetchComponentDetails(
        organizationName as string,
        projectName as string,
        componentName as string,
      ),
    );
  });
  router.post(
    '/logs/component/:componentId',
    async (req: express.Request, res: express.Response) => {
      const { componentId } = req.params;
      const { environmentId, logLevels, startTime, endTime, limit, offset } =
        req.body;

      if (!componentId || !environmentId) {
        return res.status(422).json({
          error: 'Missing Parameter',
          message: 'Component ID or Environment ID is missing from request',
        });
      }

      try {
        const result = await runtimeLogsInfoService.fetchRuntimeLogs({
          componentId,
          environmentId,
          logLevels,
          startTime,
          endTime,
          limit,
          offset,
        });

        res.json(result);
      } catch (error: unknown) {
        const errorMessage =
          error instanceof Error ? error.message : 'Unknown error occurred';

        // Check if it's a fetch error with status code info
        if (errorMessage.includes('Failed to fetch runtime logs: ')) {
          const statusMatch = errorMessage.match(
            /Failed to fetch runtime logs: (\d+)/,
          );
          if (statusMatch) {
            const statusCode = parseInt(statusMatch[1], 10);
            return res
              .status(statusCode >= 400 && statusCode < 600 ? statusCode : 500)
              .json({
                error: 'Failed to fetch runtime logs',
                message: errorMessage,
              });
          }
        }

        // Default to 500 for other errors
        return res.status(500).json({
          error: 'Internal server error',
          message: errorMessage,
        });
      }
    },
  );

  return router;
}
