/**
 * @public
 */
export interface ModelsOrganization {
  /**
   * Name of the organization
   */
  name: string;
  /**
   * Display name of the organization
   */
  displayName: string;
  /**
   * Description of the organization
   */
  description: string;
  /**
   * Namespace of the organization
   */
  namespace: string;
  /**
   * Date when the organization was created (ISO 8601 format)
   */
  createdAt: string;
  /**
   * Current status of the organization
   */
  status: string;
}
