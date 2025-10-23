import express from 'express';
import Router from 'express-promise-router';
import { PlatformEnvironmentInfoService } from './services/PlatformEnvironmentService';

export interface RouterOptions {
  platformEnvironmentService: PlatformEnvironmentInfoService;
}

export async function createRouter(
  options: RouterOptions,
): Promise<express.Router> {
  const { platformEnvironmentService } = options;
  const router = Router();

  router.use(express.json());

  // Get all environments across the platform
  router.get('/environments', async (_req, res) => {
    try {
      const environments =
        await platformEnvironmentService.fetchAllEnvironments();
      res.json({
        success: true,
        data: environments,
      });
    } catch (error) {
      res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Get environments for a specific organization
  router.get('/environments/:orgName', async (req, res) => {
    try {
      const { orgName } = req.params;
      const environments =
        await platformEnvironmentService.fetchEnvironmentsByOrganization(
          orgName,
        );
      res.json({
        success: true,
        data: environments,
      });
    } catch (error) {
      res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Get all dataplanes across the platform
  router.get('/dataplanes', async (_req, res) => {
    try {
      const dataplanes = await platformEnvironmentService.fetchAllDataplanes();
      res.json({
        success: true,
        data: dataplanes,
      });
    } catch (error) {
      res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Get dataplanes for a specific organization
  router.get('/dataplanes/:orgName', async (req, res) => {
    try {
      const { orgName } = req.params;
      const dataplanes =
        await platformEnvironmentService.fetchDataplanesByOrganization(orgName);
      res.json({
        success: true,
        data: dataplanes,
      });
    } catch (error) {
      res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Get all dataplanes with their associated environments
  router.get('/dataplanes-with-environments', async (_req, res) => {
    try {
      const dataplanesWithEnvironments =
        await platformEnvironmentService.fetchDataplanesWithEnvironments();
      res.json({
        success: true,
        data: dataplanesWithEnvironments,
      });
    } catch (error) {
      res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Get all dataplanes with their associated environments and component counts
  router.get(
    '/dataplanes-with-environments-and-components',
    async (_req, res) => {
      try {
        const dataplanesWithEnvironments =
          await platformEnvironmentService.fetchDataplanesWithEnvironmentsAndComponentCounts();
        res.json({
          success: true,
          data: dataplanesWithEnvironments,
        });
      } catch (error) {
        res.status(500).json({
          success: false,
          error: error instanceof Error ? error.message : 'Unknown error',
        });
      }
    },
  );

  // Get component counts per environment using bindings API
  router.post('/component-counts-per-environment', async (req, res) => {
    try {
      const { components } = req.body;

      if (!components || !Array.isArray(components)) {
        return res.status(400).json({
          success: false,
          error:
            'Invalid request body. Expected { components: Array<{orgName, projectName, componentName}> }',
        });
      }

      const componentCounts =
        await platformEnvironmentService.fetchComponentCountsPerEnvironment(
          components,
        );

      // Convert Map to object for JSON response
      const countsObject = Object.fromEntries(componentCounts);

      return res.json({
        success: true,
        data: countsObject,
      });
    } catch (error) {
      return res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Get distinct deployed components count using bindings API
  router.post('/distinct-deployed-components-count', async (req, res) => {
    try {
      const { components } = req.body;

      if (!components || !Array.isArray(components)) {
        return res.status(400).json({
          success: false,
          error:
            'Invalid request body. Expected { components: Array<{orgName, projectName, componentName}> }',
        });
      }

      const distinctCount =
        await platformEnvironmentService.fetchDistinctDeployedComponentsCount(
          components,
        );

      return res.json({
        success: true,
        data: distinctCount,
      });
    } catch (error) {
      return res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // Health check endpoint
  router.get('/health', (_req, res) => {
    res.json({
      success: true,
      message: 'Platform Engineer Core Backend is healthy',
    });
  });

  return router;
}
