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
 * Options you can pass into a request for additional information
 * @public
 */
export interface RequestOptions {
  token?: string;
}