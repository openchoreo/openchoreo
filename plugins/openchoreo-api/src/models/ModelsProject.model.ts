/**
 * @public
 */
export interface ModelsProject {
  /**
   * Name of the project
   */
  name: string;
  /**
   * Organization name to which the project belongs
   */
  orgName: string;
    /**
   * Display name of the organization
   */
  displayName: string;
  /**
   * Description of the organization
   */
  description: string;
  /**
   * Reference to the deployment pipeline (note: API returns "deploymentPipeline" with typo)
   */
  deploymentPipeline?: string;
  /**
   * Date when the project was created (ISO 8601 format)
   */
  createdAt: string;
  /**
   * Current status of the project
   */
  status: string;
  /**
   * Git branch to deploy from
   */
  repositoryBranch?: string;
  /**
   * Git repository URL
   */
  repositoryUrl?: string;
}
