/**
 * @public
 */
export interface ModelsBuildTemplate {
  /**
   * Name of the build template
   */
  name: string;
  /**
   * Display name of the build template
   */
  displayName?: string;
  /**
   * Description of the build template
   */
  description?: string;
  /**
   * Version of the build template
   */
  version?: string;
  /**
   * Technology stack (e.g., java, nodejs, python)
   */
  stack?: string;
}