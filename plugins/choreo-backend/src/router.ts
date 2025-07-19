import { HttpAuthService } from '@backstage/backend-plugin-api';
import { InputError } from '@backstage/errors';
import express from 'express';
import Router from 'express-promise-router';
import { EnvironmentInfoService } from './services/EnvironmentService/EnvironmentInfoService';
import { BuildTemplateInfoService } from './services/BuildTemplateService/BuildTemplateInfoService';
import { BuildInfoService } from './services/BuildService/BuildInfoService';
import { CellDiagramService } from './types';
import { ComponentInfoService } from './services/ComponentService/ComponentInfoService';

export async function createRouter({
  environmentInfoService,
  cellDiagramInfoService,
  buildTemplateInfoService,
  buildInfoService,
  componentInfoService,
}: {
  httpAuth: HttpAuthService;
  environmentInfoService: EnvironmentInfoService;
  cellDiagramInfoService: CellDiagramService;
  buildTemplateInfoService: BuildTemplateInfoService;
  buildInfoService: BuildInfoService;
  componentInfoService: ComponentInfoService;
}): Promise<express.Router> {
  const router = Router();
  router.use(express.json());

  router.get('/environments', async (req, res) => {
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
      throw new InputError(
        'organizationName is a required query parameter',
      );
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

  return router;
}
