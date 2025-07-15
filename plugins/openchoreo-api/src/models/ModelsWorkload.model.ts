export interface EnvVar {
  name: string;
  value?: string;
}

export interface Container {
  image: string;
  command?: string[];
  args?: string[];
  env?: EnvVar[];
}

export interface Schema {
  type?: string;
  content?: string;
}

export interface WorkloadEndpoint {
  protocol: 'TCP' | 'UDP';
  port: number;
  schema?: Schema;
}

export interface WorkloadOwner {
  projectName: string;
  componentName: string;
}

export type WorkloadType = 'Service' | 'ManualTask' | 'ScheduledTask' | 'WebApplication';

/**
 * @public
 */
export interface ModelsWorkload {
  /**
   * Name of the workload
   */
  name: string;
  /**
   * Type of the workload
   */
  type: WorkloadType;
  /**
   * Owner information
   */
  owner: WorkloadOwner;
  /**
   * Container specifications
   */
  containers?: { [key: string]: Container };
  /**
   * Network endpoint specifications
   */
  endpoints?: { [key: string]: WorkloadEndpoint };
  /**
   * External resource connections
   */
  connections?: { [key: string]: string };
  /**
   * Current status of the workload
   */
  status?: string;
  /**
   * Date when the workload was created (ISO 8601 format)
   */
  createdAt?: string;
}
