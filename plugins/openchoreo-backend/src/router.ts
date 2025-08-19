import { InputError } from '@backstage/errors';
import express from 'express';
import Router from 'express-promise-router';
import { EnvironmentInfoService } from './services/EnvironmentService/EnvironmentInfoService';
import { BuildTemplateInfoService } from './services/BuildTemplateService/BuildTemplateInfoService';
import { BuildInfoService } from './services/BuildService/BuildInfoService';
import { CellDiagramService, WorkloadService } from './types';
import { ComponentInfoService } from './services/ComponentService/ComponentInfoService';
import { RuntimeLogsInfoService } from './services/RuntimeLogsService/RuntimeLogsService';
import { ObservabilityNotConfiguredError } from '@internal/plugin-openchoreo-api';

export async function createRouter({
  environmentInfoService,
  cellDiagramInfoService,
  buildTemplateInfoService,
  buildInfoService,
  componentInfoService,
  runtimeLogsInfoService,
  workloadInfoService,
}: {
  environmentInfoService: EnvironmentInfoService;
  cellDiagramInfoService: CellDiagramService;
  buildTemplateInfoService: BuildTemplateInfoService;
  buildInfoService: BuildInfoService;
  componentInfoService: ComponentInfoService;
  runtimeLogsInfoService: RuntimeLogsInfoService;
  workloadInfoService: WorkloadService;
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
    const { sourceEnv, targetEnv, componentName, projectName, orgName } =
      req.body;

    if (
      !sourceEnv ||
      !targetEnv ||
      !componentName ||
      !projectName ||
      !orgName
    ) {
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

  router.patch('/update-binding', async (req, res) => {
    const { componentName, projectName, orgName, bindingName, releaseState } =
      req.body;

    if (
      !componentName ||
      !projectName ||
      !orgName ||
      !bindingName ||
      !releaseState
    ) {
      throw new InputError(
        'componentName, projectName, orgName, bindingName and releaseState are required in request body',
      );
    }

    if (!['Active', 'Suspend', 'Undeploy'].includes(releaseState)) {
      throw new InputError(
        'releaseState must be one of: Active, Suspend, Undeploy',
      );
    }

    res.json(
      await environmentInfoService.updateComponentBinding({
        componentName: componentName as string,
        projectName: projectName as string,
        organizationName: orgName as string,
        bindingName: bindingName as string,
        releaseState: releaseState as 'Active' | 'Suspend' | 'Undeploy',
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
  router.get('/build-logs', async (req, res) => {
    const { componentName, buildId, buildUuid, projectName, orgName } =
      req.query;

    if (!componentName || !buildId || !buildUuid) {
      throw new InputError(
        'componentName, buildId and buildUuid are required query parameters',
      );
    }

    try {
      const result = await buildInfoService.fetchBuildLogs(
        orgName as string,
        projectName as string,
        componentName as string,
        buildId as string,
        buildUuid as string,
      );
      return res.json(result);
    } catch (error: unknown) {
      if (error instanceof ObservabilityNotConfiguredError) {
        return res.status(200).json({
          message: "Observability hasn't been configured",
        });
      }
      throw error;
    }
  });

  // Runtime logs
  router.post(
    '/logs/component/:componentId',
    async (req: express.Request, res: express.Response) => {
      const { componentId } = req.params;
      const { orgName, projectName } = req.query;
      const { environmentId, logLevels, startTime, endTime, limit, offset } =
        req.body;

      if (!componentId || !environmentId) {
        return res.status(422).json({
          error: 'Missing Parameter',
          message: 'Component ID or Environment ID is missing from request',
        });
      }

      try {
        const result = await runtimeLogsInfoService.fetchRuntimeLogs(
          {
            componentId,
            environmentId,
            logLevels,
            startTime,
            endTime,
            limit,
            offset,
          },
          orgName as string,
          projectName as string,
        );

        return res.json(result);
      } catch (error: unknown) {
        if (error instanceof ObservabilityNotConfiguredError) {
          return res.status(200).json({
            message: 'observability is disabled',
          });
        }

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

  router.get('/workload', async (req, res) => {
    const { componentName, projectName, organizationName } = req.query;

    if (!componentName || !projectName || !organizationName) {
      throw new InputError(
        'componentName, projectName and organizationName are required query parameters',
      );
    }

    try {
      const result = await workloadInfoService.fetchWorkloadInfo({
        componentName: componentName as string,
        projectName: projectName as string,
        organizationName: organizationName as string,
      });

      res.json(result);
    } catch (error) {
      console.error('Workload fetch error:', error);
      res.status(500).json({
        error: {
          message: error instanceof Error ? error.message : 'Unknown error',
          name: error instanceof Error ? error.name : 'UnknownError',
          ...(error instanceof Error && error.stack && { stack: error.stack }),
        },
      });
    }
  });

  router.post('/workload', async (req, res) => {
    const { componentName, projectName, organizationName } = req.query;
    const workloadSpec = req.body;

    if (!componentName || !projectName || !organizationName) {
      throw new InputError(
        'componentName, projectName and organizationName are required query parameters',
      );
    }

    if (!workloadSpec) {
      throw new InputError(
        'Workload specification is required in request body',
      );
    }

    try {
      const result = await workloadInfoService.applyWorkload({
        componentName: componentName as string,
        projectName: projectName as string,
        organizationName: organizationName as string,
        workloadSpec,
      });

      res.json(result);
    } catch (error) {
      console.error('Workload apply error:', error);
      res.status(500).json({
        error: {
          message: error instanceof Error ? error.message : 'Unknown error',
          name: error instanceof Error ? error.name : 'UnknownError',
          ...(error instanceof Error && error.stack && { stack: error.stack }),
        },
      });
    }
  });

  return router;
}
