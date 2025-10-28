import { ModelsService } from './ModelsService.model';
import { ModelsWebApplication } from './ModelsWebApplication.model';
import { ModelsWorkload } from './ModelsWorkload.model';
import { BuildConfig } from './requests';

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
  /**
   * Display name of the component
   */
  displayName?: string;
  /**
   * Build configuration
   */
  buildConfig?: BuildConfig;
}

export interface ModelsCompleteComponent extends ModelsComponent {
  workload?: ModelsWorkload;
  service?: ModelsService;
  webApplication?: ModelsWebApplication;
}
