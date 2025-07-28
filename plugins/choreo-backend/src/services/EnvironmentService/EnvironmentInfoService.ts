import { LoggerService } from '@backstage/backend-plugin-api';
import { EnvironmentService, Environment, EndpointInfo } from '../../types';
import {
  DefaultApiClient,
  ModelsEnvironment,
  OpenChoreoApiClient,
  BindingResponse,
  BindingStatusType,
} from '@internal/plugin-openchoreo-api';

/**
 * Service for managing and retrieving environment-related information for deployments.
 * This service handles fetching environment details from the OpenChoreo API.
 */
export class EnvironmentInfoService implements EnvironmentService {
  private readonly logger: LoggerService;
  private readonly client: OpenChoreoApiClient;
  private readonly defaultClient: DefaultApiClient;

  public constructor(logger: LoggerService, baseUrl: string, token?: string) {
    this.logger = logger;
    this.client = new OpenChoreoApiClient(baseUrl, token, logger);
    // Reuse the same defaultClient instance
    this.defaultClient = new DefaultApiClient(baseUrl, {});
  }

  static create(
    logger: LoggerService,
    baseUrl: string,
    token?: string,
  ): EnvironmentInfoService {
    return new EnvironmentInfoService(logger, baseUrl, token);
  }

  /**
   * Fetches deployment information for a specific component in a project.
   * This method retrieves detailed information about deployments across different environments
   * using the bindings API, including their status, deployment time, images, and endpoints.
   * Environments are returned in the order defined by the deployment pipeline.
   *
   * @param {Object} request - The request parameters
   * @param {string} request.projectName - Name of the project containing the component
   * @param {string} request.componentName - Name of the component to fetch deployment info for
   * @param {string} request.organizationName - Name of the organization owning the project
   * @returns {Promise<Environment[]>} Array of environments with their deployment information
   * @throws {Error} When there's an error fetching data from the API
   */
  async fetchDeploymentInfo(request: {
    projectName: string;
    componentName: string;
    organizationName: string;
  }): Promise<Environment[]> {
    const startTime = Date.now();
    try {
      this.logger.info(`Starting environment fetch for component: ${request.componentName}`);
      
      // Fetch environments, bindings and deployment pipeline in parallel with individual timing
      const createTimedPromise = <T>(promise: Promise<T>, name: string) => {
        const start = Date.now();
        return promise
          .then(result => ({ type: name, result, duration: Date.now() - start }))
          .catch(error => {
            const duration = Date.now() - start;
            if (name === 'bindings') {
              this.logger.warn(
                `Failed to fetch bindings for component ${request.componentName}: ${error}`,
              );
              return { type: name, result: [] as any, duration };
            } else if (name === 'pipeline') {
              this.logger.warn(
                `No deployment pipeline found for project ${request.projectName}, using default ordering`,
              );
              return { type: name, result: null as any, duration };
            }
            throw error;
          });
      };
      
      const environmentsPromise = createTimedPromise(
        this.defaultClient.environmentsGet({
          orgName: request.organizationName,
        }),
        'environments'
      );
      
      const bindingsPromise = createTimedPromise(
        this.client.getComponentBindings(
          request.organizationName,
          request.projectName,
          request.componentName,
        ),
        'bindings'
      );
      
      const pipelinePromise = createTimedPromise(
        this.client.getProjectDeploymentPipeline(
          request.organizationName,
          request.projectName,
        ),
        'pipeline'
      );

      const fetchStart = Date.now();
      const [environmentsResult, bindingsResult, pipelineResult] = await Promise.all([
        environmentsPromise,
        bindingsPromise, 
        pipelinePromise
      ]);
      const fetchEnd = Date.now();

      // Log individual timings
      this.logger.info(`API call timings - Environments: ${environmentsResult.duration}ms, Bindings: ${bindingsResult.duration}ms, Pipeline: ${pipelineResult.duration}ms`);
      this.logger.info(`Total parallel API calls completed in ${fetchEnd - fetchStart}ms`);

      const environmentsResponse = environmentsResult.result;
      const bindings = bindingsResult.result;
      const deploymentPipeline = pipelineResult.result;

      if (!environmentsResponse.ok) {
        this.logger.error(
          `Failed to fetch environments for organization ${request.organizationName}`,
        );
        return [];
      }

      const environmentsData = await environmentsResponse.json();
      if (!environmentsData.success || !environmentsData.data?.items) {
        this.logger.warn('No environments found in API response');
        return [];
      }

      const environments = environmentsData.data.items as ModelsEnvironment[];

      // Transform environment data with bindings and promotion information
      const transformStart = Date.now();
      const result = this.transformEnvironmentDataWithBindings(
        environments,
        bindings,
        deploymentPipeline,
      );
      const transformEnd = Date.now();
      
      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Environment fetch completed for ${request.componentName}: ` +
        `Individual API calls (Env: ${environmentsResult.duration}ms, Bind: ${bindingsResult.duration}ms, Pipeline: ${pipelineResult.duration}ms), ` +
        `Parallel execution: ${fetchEnd - fetchStart}ms, ` +
        `Transform: ${transformEnd - transformStart}ms, ` +
        `Total: ${totalTime}ms`
      );
      
      return result;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching deployment info for ${request.projectName} (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  private transformEnvironmentDataWithBindings(
    environmentData: ModelsEnvironment[],
    bindings: BindingResponse[],
    deploymentPipeline: any | null,
  ): Environment[] {
    // Create maps for easy lookup
    const envMap = new Map<string, ModelsEnvironment>();
    const envNameMap = new Map<string, string>(); // lowercase -> actual name
    const bindingsByEnv = new Map<string, BindingResponse>();
    
    // Build environment maps
    for (const env of environmentData) {
      const displayName = env.displayName || env.name;
      envMap.set(displayName, env);
      envMap.set(displayName.toLowerCase(), env);
      envNameMap.set(displayName.toLowerCase(), displayName);
    }

    // Build bindings map by environment
    for (const binding of bindings) {
      const envName = envNameMap.get(binding.environment.toLowerCase()) || binding.environment;
      bindingsByEnv.set(envName, binding);
    }

    // If no pipeline data, use default ordering
    if (!deploymentPipeline || !deploymentPipeline.promotionPaths) {
      this.logger.info('No deployment pipeline found, using default ordering');
      return this.transformEnvironmentDataWithBindingsOnly(environmentData, bindingsByEnv);
    }

    // Build promotion map from pipeline data (normalized to actual env names)
    const promotionMap = new Map<string, any[]>();
    for (const path of deploymentPipeline.promotionPaths) {
      const sourceEnv = envNameMap.get(path.sourceEnvironmentRef.toLowerCase()) || path.sourceEnvironmentRef;
      const targets = path.targetEnvironmentRefs.map((ref: any) => ({
        ...ref,
        name: envNameMap.get(ref.name.toLowerCase()) || ref.name,
      }));
      promotionMap.set(sourceEnv, targets);
    }

    // Determine environment order based on pipeline
    const orderedEnvNames = this.getEnvironmentOrder(
      deploymentPipeline.promotionPaths,
      envNameMap,
    );
    
    // Transform environments in pipeline order
    const orderedEnvironments: Environment[] = [];
    const processedEnvs = new Set<string>();
    
    for (const envName of orderedEnvNames) {
      const envData = envMap.get(envName);
      if (envData && !processedEnvs.has(envName)) {
        processedEnvs.add(envName);
        const binding = bindingsByEnv.get(envName);
        const promotionTargets = promotionMap.get(envName);
        
        const transformedEnv = this.createEnvironmentFromBinding(
          envData,
          binding,
          promotionTargets,
        );

        orderedEnvironments.push(transformedEnv);
      }
    }

    // Add any environments not in the pipeline at the end
    for (const env of environmentData) {
      const envName = env.displayName || env.name;
      if (!processedEnvs.has(envName)) {
        const binding = bindingsByEnv.get(envName);
        orderedEnvironments.push(this.createEnvironmentFromBinding(env, binding));
      }
    }

    return orderedEnvironments;
  }

  private createEnvironmentFromBinding(
    envData: ModelsEnvironment,
    binding: BindingResponse | undefined,
    promotionTargets?: any[],
  ): Environment {
    const envName = envData.displayName || envData.name;
    
    // Extract endpoints from binding
    const endpoints: EndpointInfo[] = [];
    let image: string | undefined;
    let deploymentStatus: 'success' | 'failed' | 'pending' | 'not-deployed' | 'suspended' = 'not-deployed';
    let statusMessage: string | undefined;
    let lastDeployed: string | undefined;

    if (binding) {
      // Get deployment status from binding using new status values
      if (binding.status) {
        switch (binding.status.status) {
          case 'Active':
            deploymentStatus = 'success';
            break;
          case 'Failed':
            deploymentStatus = 'failed';
            break;
          case 'InProgress':
            deploymentStatus = 'pending';
            break;
          case 'NotYetDeployed':
            deploymentStatus = 'not-deployed';
            break;
          case 'Suspended':
            deploymentStatus = 'suspended';
            break;
          default:
            deploymentStatus = 'pending';
        }
        statusMessage = binding.status.message;
        lastDeployed = binding.status.lastTransitioned || envData.createdAt;
      }

      // Extract image from binding
      if (binding.webApplicationBinding?.image) {
        image = binding.webApplicationBinding.image;
      } else if (binding.serviceBinding?.image) {
        image = binding.serviceBinding.image;
      }

      // Extract endpoints
      const bindingEndpoints = binding.webApplicationBinding?.endpoints || 
                               binding.serviceBinding?.endpoints || [];
      
      // Check if this is a WebApplication component
      const isWebApp = binding.type === 'WebApplication' || binding.webApplicationBinding;
      
      for (const endpoint of bindingEndpoints) {
        if (isWebApp) {
          // For WebApplication components, only show public endpoints
          if (endpoint.public) {
            endpoints.push({
              name: endpoint.name,
              type: endpoint.type,
              url: endpoint.public.uri || `${endpoint.public.scheme}://${endpoint.public.host}:${endpoint.public.port}${endpoint.public.basePath || ''}`,
              visibility: 'public',
            });
          }
        } else {
          // For other component types, show all endpoints
          // Add public endpoint if available
          if (endpoint.public) {
            endpoints.push({
              name: endpoint.name,
              type: endpoint.type,
              url: endpoint.public.uri || `${endpoint.public.scheme}://${endpoint.public.host}:${endpoint.public.port}${endpoint.public.basePath || ''}`,
              visibility: 'public',
            });
          }
          // Add organization endpoint if available
          if (endpoint.organization) {
            endpoints.push({
              name: endpoint.name,
              type: endpoint.type,
              url: endpoint.organization.uri || `${endpoint.organization.scheme}://${endpoint.organization.host}:${endpoint.organization.port}${endpoint.organization.basePath || ''}`,
              visibility: 'organization',
            });
          }
          // Add project endpoint if available
          if (endpoint.project) {
            endpoints.push({
              name: endpoint.name,
              type: endpoint.type,
              url: endpoint.project.uri || `${endpoint.project.scheme}://${endpoint.project.host}:${endpoint.project.port}${endpoint.project.basePath || ''}`,
              visibility: 'project',
            });
          }
        }
      }
    }

    const transformedEnv: Environment = {
      name: envName,
      bindingName: binding?.name,
      deployment: {
        status: deploymentStatus,
        lastDeployed,
        image,
        statusMessage,
      },
      endpoints,
    };

    // Add promotion targets if they exist
    if (promotionTargets && promotionTargets.length > 0) {
      transformedEnv.promotionTargets = promotionTargets.map(
        (ref: any) => ({
          name: ref.name,
          requiresApproval: ref.requiresApproval,
          isManualApprovalRequired: ref.isManualApprovalRequired,
        }),
      );
    }

    return transformedEnv;
  }

  private transformEnvironmentDataWithBindingsOnly(
    environmentData: ModelsEnvironment[],
    bindingsByEnv: Map<string, BindingResponse>,
  ): Environment[] {
    return environmentData.map(env => {
      const envName = env.displayName || env.name;
      const binding = bindingsByEnv.get(envName);
      return this.createEnvironmentFromBinding(env, binding);
    });
  }


  private getEnvironmentOrder(
    promotionPaths: any[],
    envNameMap: Map<string, string>,
  ): string[] {
    // Build a proper dependency graph
    const graph = new Map<string, Set<string>>();
    const allEnvs = new Set<string>();
    
    // Initialize graph and collect all environments
    for (const path of promotionPaths) {
      const source = envNameMap.get(path.sourceEnvironmentRef.toLowerCase()) || path.sourceEnvironmentRef;
      allEnvs.add(source);
      
      if (!graph.has(source)) {
        graph.set(source, new Set());
      }
      
      for (const target of path.targetEnvironmentRefs) {
        const targetName = envNameMap.get(target.name.toLowerCase()) || target.name;
        allEnvs.add(targetName);
        graph.get(source)!.add(targetName);
      }
    }
    
    // Kahn's algorithm for topological sort
    const inDegree = new Map<string, number>();
    const queue: string[] = [];
    const result: string[] = [];
    
    // Initialize in-degrees
    for (const env of allEnvs) {
      inDegree.set(env, 0);
    }
    
    // Calculate in-degrees
    for (const [_, targets] of graph) {
      for (const target of targets) {
        inDegree.set(target, (inDegree.get(target) || 0) + 1);
      }
    }
    
    // Find nodes with no incoming edges
    for (const [env, degree] of inDegree) {
      if (degree === 0) {
        queue.push(env);
      }
    }
    
    // Process queue
    while (queue.length > 0) {
      const current = queue.shift()!;
      result.push(current);
      
      const neighbors = graph.get(current) || new Set();
      for (const neighbor of neighbors) {
        const newDegree = (inDegree.get(neighbor) || 0) - 1;
        inDegree.set(neighbor, newDegree);
        if (newDegree === 0) {
          queue.push(neighbor);
        }
      }
    }
    
    // If we have a specific order preference for environments with same level, apply it
    // This ensures Development -> Staging -> Production order when they're at the same level
    const preferredOrder = ['Development', 'Staging', 'Production'];
    
    // Group environments by their level in the DAG
    const levels = new Map<string, number>();
    const visited = new Set<string>();
    
    const calculateLevel = (env: string, level: number = 0): number => {
      if (visited.has(env)) return levels.get(env) || 0;
      visited.add(env);
      levels.set(env, level);
      
      const neighbors = graph.get(env) || new Set();
      for (const neighbor of neighbors) {
        calculateLevel(neighbor, level + 1);
      }
      return level;
    };
    
    // Calculate levels for all environments
    for (const env of result) {
      if (!visited.has(env)) {
        calculateLevel(env);
      }
    }
    
    // Sort by level first, then by preferred order
    result.sort((a, b) => {
      const levelA = levels.get(a) || 0;
      const levelB = levels.get(b) || 0;
      
      if (levelA !== levelB) {
        return levelA - levelB;
      }
      
      // Same level, use preferred order
      const indexA = preferredOrder.indexOf(a);
      const indexB = preferredOrder.indexOf(b);
      
      if (indexA !== -1 && indexB !== -1) {
        return indexA - indexB;
      }
      
      if (indexA !== -1) return -1;
      if (indexB !== -1) return 1;
      
      return a.localeCompare(b);
    });
    
    return result;
  }

  /**
   * Promotes a component from source environment to target environment.
   * Uses the OpenChoreo API client to perform the promotion and returns updated environment data.
   *
   * @param {Object} request - The promotion request parameters
   * @param {string} request.sourceEnvironment - Source environment name
   * @param {string} request.targetEnvironment - Target environment name
   * @param {string} request.componentName - Name of the component to promote
   * @param {string} request.projectName - Name of the project containing the component
   * @param {string} request.organizationName - Name of the organization owning the project
   * @returns {Promise<Environment[]>} Array of environments with updated deployment information
   * @throws {Error} When there's an error promoting the component
   */
  async promoteComponent(request: {
    sourceEnvironment: string;
    targetEnvironment: string;
    componentName: string;
    projectName: string;
    organizationName: string;
  }): Promise<Environment[]> {
    const startTime = Date.now();
    try {
      this.logger.info(
        `Starting promotion for component: ${request.componentName} from ${request.sourceEnvironment} to ${request.targetEnvironment}`,
      );

      // Call the promotion API
      const promotionResult = await this.client.promoteComponent(
        request.organizationName,
        request.projectName,
        request.componentName,
        request.sourceEnvironment,
        request.targetEnvironment,
      );

      this.logger.info(
        `Promotion completed successfully. Received ${promotionResult.length} binding responses.`,
      );

      // Fetch fresh environment data to return updated information
      const refreshedEnvironments = await this.fetchDeploymentInfo({
        componentName: request.componentName,
        projectName: request.projectName,
        organizationName: request.organizationName,
      });

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Component promotion completed for ${request.componentName}: Total: ${totalTime}ms`,
      );

      return refreshedEnvironments;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error promoting component ${request.componentName} from ${request.sourceEnvironment} to ${request.targetEnvironment} (${totalTime}ms):`,
        error as Error,
      );
      throw error;
    }
  }

  /**
   * Updates a component binding's release state (Active, Suspend, or Undeploy).
   * Uses the OpenChoreo API client to update the binding and returns updated environment data.
   *
   * @param {Object} request - The update request parameters
   * @param {string} request.componentName - Name of the component 
   * @param {string} request.projectName - Name of the project containing the component
   * @param {string} request.organizationName - Name of the organization owning the project
   * @param {string} request.bindingName - Name of the binding to update
   * @param {'Active' | 'Suspend' | 'Undeploy'} request.releaseState - The new release state
   * @returns {Promise<Environment[]>} Array of environments with updated deployment information
   * @throws {Error} When there's an error updating the binding
   */
  async updateComponentBinding(request: {
    componentName: string;
    projectName: string;
    organizationName: string;
    bindingName: string;
    releaseState: 'Active' | 'Suspend' | 'Undeploy';
  }): Promise<Environment[]> {
    const startTime = Date.now();
    try {
      this.logger.info(
        `Starting binding update for component: ${request.componentName}, binding: ${request.bindingName}, new state: ${request.releaseState}`,
      );

      // Call the update binding API
      await this.client.updateComponentBinding(
        request.organizationName,
        request.projectName,
        request.componentName,
        request.bindingName,
        request.releaseState,
      );

      this.logger.info(
        `Binding update completed successfully for ${request.bindingName}.`,
      );

      // Fetch fresh environment data to return updated information
      const refreshedEnvironments = await this.fetchDeploymentInfo({
        componentName: request.componentName,
        projectName: request.projectName,
        organizationName: request.organizationName,
      });

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Component binding update completed for ${request.componentName}: Total: ${totalTime}ms`,
      );

      return refreshedEnvironments;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error updating binding ${request.bindingName} for component ${request.componentName} (${totalTime}ms):`,
        error as Error,
      );
      throw error;
    }
  }
}
