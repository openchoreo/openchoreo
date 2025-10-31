/**
 * Binding-related models for OpenChoreo API
 * @public
 */

/**
 * Exposed endpoint configuration
 * @public
 */
export interface ExposedEndpoint {
  /**
   * Host address
   */
  host: string;
  /**
   * Port number
   */
  port: number;
  /**
   * Scheme (e.g., gRPC, HTTP, etc.)
   */
  scheme?: string;
  /**
   * Base path for HTTP-based endpoints
   */
  basePath?: string;
  /**
   * Complete URI
   */
  uri?: string;
}

/**
 * Endpoint status information
 * @public
 */
export interface EndpointStatus {
  /**
   * Name of the endpoint
   */
  name: string;
  /**
   * Type of the endpoint
   */
  type: string;
  /**
   * Project-level exposed endpoint
   */
  project?: ExposedEndpoint;
  /**
   * Organization-level exposed endpoint
   */
  organization?: ExposedEndpoint;
  /**
   * Public exposed endpoint
   */
  public?: ExposedEndpoint;
}

/**
 * Service binding information
 * @public
 */
export interface ServiceBinding {
  /**
   * List of endpoint statuses
   */
  endpoints: EndpointStatus[];
  /**
   * Container image URL
   */
  image?: string;
}

/**
 * Web application binding information
 * @public
 */
export interface WebApplicationBinding {
  /**
   * List of endpoint statuses
   */
  endpoints: EndpointStatus[];
  /**
   * Container image URL
   */
  image?: string;
}

/**
 * Scheduled task binding information
 * @public
 */
export interface ScheduledTaskBinding {
  // Currently empty as per Go type definition
}

/**
 * Binding status type values
 * @public
 */
export type BindingStatusType =
  | 'InProgress'
  | 'Active'
  | 'Failed'
  | 'Suspended'
  | 'NotYetDeployed';

/**
 * Binding status information
 * @public
 */
export interface BindingStatus {
  /**
   * Reason for the status
   */
  reason: string;
  /**
   * Status message
   */
  message: string;
  /**
   * Status type - Active, InProgress, Failed, Suspended, or NotYetDeployed
   */
  status: BindingStatusType;
  /**
   * When the status last changed (ISO 8601 format)
   */
  lastTransitioned: string;
}

/**
 * Component binding response
 * @public
 */
export interface BindingResponse {
  /**
   * Name of the binding
   */
  name: string;
  /**
   * Type of the binding
   */
  type: string;
  /**
   * Name of the component
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
   * Environment name
   */
  environment: string;
  /**
   * Binding status
   */
  status: BindingStatus;
  /**
   * Service-specific binding data
   */
  serviceBinding?: ServiceBinding;
  /**
   * Web application-specific binding data
   */
  webApplicationBinding?: WebApplicationBinding;
  /**
   * Scheduled task-specific binding data
   */
  scheduledTaskBinding?: ScheduledTaskBinding;
}
