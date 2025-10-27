import { Entity } from '@backstage/catalog-model';

/**
 * Backstage catalog Environment kind Entity. Represents an OpenChoreo environment.
 *
 * @public
 */
export interface EnvironmentEntityV1alpha1 extends Entity {
  /**
   * The apiVersion string of the Environment.
   */
  apiVersion: 'backstage.io/v1alpha1';
  /**
   * The kind of the entity
   */
  kind: 'Environment';
  /**
   * The specification of the Environment Entity
   */
  spec: {
    /**
     * The type of environment (e.g., 'development', 'staging', 'production')
     */
    type: string;
    /**
     * The owner of the environment
     */
    owner: string;
    /**
     * The domain this environment belongs to
     */
    domain?: string;
    /**
     * Whether this is a production environment
     */
    isProduction?: boolean;
    /**
     * The data plane reference for this environment
     */
    dataPlaneRef?: string;
    /**
     * DNS prefix for this environment
     */
    dnsPrefix?: string;
  };
}
