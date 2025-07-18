/**
 * Build status information from OpenChoreo BuildV2Status
 * @public
 */
export interface BuildStatus {
  /**
   * Overall build phase (e.g., Running, Succeeded, Failed)
   */
  phase?: string;
  /**
   * Start time of the build
   */
  startTime?: string;
  /**
   * Completion time of the build
   */
  completionTime?: string;
  /**
   * Duration of the build in seconds
   */
  duration?: number;
  /**
   * Detailed message about the build status
   */
  message?: string;
  /**
   * Build steps and their status
   */
  steps?: Array<{
    name: string;
    status: string;
    startTime?: string;
    completionTime?: string;
    message?: string;
  }>;
}

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
   * Git branch name
   */
  branch?: string;
  /**
   * Built container image name/tag
   */
  image?: string;
  /**
   * High-level build status (e.g., Running, Succeeded, Failed)
   */
  status?: string;
  /**
   * Detailed build status information
   */
  buildStatus?: BuildStatus;
  /**
   * Date when the build was created (ISO 8601 format)
   */
  createdAt: string;
}