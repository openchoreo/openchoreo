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
export type BuildConfig = {
  repoUrl: string;
  repoBranch: string;
  componentPath: string;
  buildTemplateRef: string;
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
 * Options you can pass into a request for additional information
 * @public
 */
export interface RequestOptions {
  token?: string;
}
