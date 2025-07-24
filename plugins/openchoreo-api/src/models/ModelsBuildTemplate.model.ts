/**
 * Build template parameter
 * @public
 */
export interface BuildTemplateParameter {
  /**
   * Parameter name
   */
  name: string;
  /**
   * Parameter display name
   */
  displayName?: string;
  /**
   * Parameter description
   */
  description?: string;
  /**
   * Parameter type (e.g., string, number, boolean)
   */
  type?: string;
  /**
   * Default value for the parameter
   */
  default?: string;
  /**
   * Whether the parameter is required
   */
  required?: boolean;
}

/**
 * Build template response from OpenChoreo API
 * @public
 */
export interface ModelsBuildTemplate {
  /**
   * Name of the build template
   */
  name: string;
  /**
   * Build template parameters
   */
  parameters?: BuildTemplateParameter[];
  /**
   * Creation timestamp (ISO 8601 format)
   */
  createdAt: string;
}
