/**
 * @public
 */
export interface ModelsComponent {
  /**
   * Name of the component
   */
  name: string;
  /**
   * Description of the component
   */
  description: string;
  /**
   * Type of the component (e.g., Service)
   */
  type: string;
  /**
   * Name of the project to which the component belongs
   */
  projectName: string;
  /**
   * Organization name to which the component belongs
   */
  orgName: string;
  /**
   * Git repository URL
   */
  repositoryUrl: string;
  /**
   * Git branch
   */
  branch: string;
  /**
   * Date when the component was created (ISO 8601 format)
   */
  createdAt: string;
  /**
   * Current status of the component
   */
  status: string;
}
