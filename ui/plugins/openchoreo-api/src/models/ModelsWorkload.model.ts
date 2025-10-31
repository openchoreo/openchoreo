export interface EnvVar {
  key: string; // TODO: change to name
  value?: string;
  valueFrom?: {
    configurationGroupRef: {
      name: string;
      key: string;
    };
  };
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
  // It should be WebSocket, not Websocket
  type: 'TCP' | 'UDP' | 'HTTP' | 'REST' | 'gRPC' | 'Websocket' | 'GraphQL';
  port: number;
  schema?: Schema;
}

export interface ConnectionInjectEnv {
  name: string;
  value: string;
}

export interface ConnectionInject {
  env?: ConnectionInjectEnv[];
}

export interface ConnectionParams {
  componentName: string;
  endpoint: string;
  projectName: string;
}

export interface Connection {
  inject?: ConnectionInject;
  params: ConnectionParams;
  type: string;
}

export interface WorkloadOwner {
  projectName: string;
  componentName: string;
}

export type WorkloadType =
  | 'Service'
  | 'ManualTask'
  | 'ScheduledTask'
  | 'WebApplication';

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
  connections?: { [key: string]: Connection };
  /**
   * Current status of the workload
   */
  status?: string;
  /**
   * Date when the workload was created (ISO 8601 format)
   */
  createdAt?: string;
}
