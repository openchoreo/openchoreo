/**
 * Request types for OpenChoreo API
 * @public
 */

/**
 * Request parameters for getting all projects
 * @public
 */
export type ProjectsGetRequest = {
  orgName: string;
};

/**
 * Request parameters for getting projects by organization
 * @public
 */
export type OrgProjectsGetRequest = {
  orgName: string;
};

/**
 * Request parameters for getting all organizations
 * @public
 */
export type OrganizationsGetRequest = {
  // No parameters needed for getting all organizations
};

/**
 * Request parameters for getting all components of a project
 * @public
 */
export type ComponentsGetRequest = {
  orgName: string;
  projectName: string;
};

/**
 * Request parameters for getting all components of a project
 * @public
 */
export type ComponentGetRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
  
};

/**
 * Request parameters for creating a new project
 * @public
 */
export type ProjectsPostRequest = {
  orgName: string;
  name: string;
  displayName?: string;
  description?: string;
  deploymentPipeline?: string;
};

/**
 * Build configuration for component creation
 * @public
 */
export type TemplateParameter = {
  name: string;
  value: string;
};

export type BuildConfig = {
  repoUrl: string;
  repoBranch: string;
  componentPath: string;
  buildTemplateRef: string;
  buildTemplateParams?: TemplateParameter[];
};

/**
 * Request parameters for creating a new component
 * @public
 */
export type ComponentsPostRequest = {
  orgName: string;
  projectName: string;
  name: string;
  displayName?: string;
  description?: string;
  type: string;
  buildConfig?: BuildConfig;
};

/**
 * Request parameters for getting build templates
 * @public
 */
export type BuildTemplatesGetRequest = {
  orgName: string;
};

/**
 * Request parameters for getting builds of a component
 * @public
 */
export type BuildsGetRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
};

/**
 * Request parameters for triggering a build for a component
 * @public
 */
export type BuildsTriggerRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
  commit?: string;
};

/**
 * Request parameters for getting component bindings
 * @public
 */
export type BindingsGetRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
  environment?: string[];
};

/**
 * Request parameters for getting project deployment pipeline
 * @public
 */
export type ProjectDeploymentPipelineGetRequest = {
  orgName: string;
  projectName: string;
};

/**
 * Request body for promoting a component between environments
 * @public
 */
export type PromoteComponentRequest = {
  sourceEnv: string;
  targetEnv: string;
};

/**
 * Request parameters for promoting a component
 * @public
 */
export type ComponentPromotePostRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
  promoteComponentRequest: PromoteComponentRequest;
};

/**
 * Request parameters for getting workload information
 * @public
 */
export type WorkloadGetRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
};

/**
 * Request parameters for updating workload configuration
 * @public
 */
export type WorkloadPostRequest = {
  orgName: string;
  projectName: string;
  componentName: string;
  workloadSpec: any; // Will be ModelsWorkload but avoiding circular import
};

/**
 * Options you can pass into a request for additional information
 * @public
 */
export interface RequestOptions {
  token?: string;
}
