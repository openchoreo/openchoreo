
/**
 * @public
 */
export interface ModelsBuild {
  /**
   * Name of the build
   */
  name: string;
  /**
   * Name of the component this build belongs to
   */
  componentName: string;
  /**
   * Name of the project
   */
  projectName: string;
  /**
   * Organization name
   */
  orgName: string;
  /**
   * Git commit hash
   */
  commit?: string;
  /**
   * High-level build status (e.g., Running, Succeeded, Failed)
   */
  status?: string;
  /**
   * Date when the build was created (ISO 8601 format)
   */
  createdAt: string;
  /**
   * Image name
   */
  image?: string;

}
