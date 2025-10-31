/**
 * Deployment Pipeline models for OpenChoreo API
 * @public
 */

/**
 * Represents a target environment reference with approval settings
 * @public
 */
export interface TargetEnvironmentRef {
  name: string;
  requiresApproval?: boolean;
  isManualApprovalRequired?: boolean;
}

/**
 * Represents a promotion path in the deployment pipeline
 * @public
 */
export interface PromotionPath {
  sourceEnvironmentRef: string;
  targetEnvironmentRefs: TargetEnvironmentRef[];
}

/**
 * Represents a deployment pipeline in API responses
 * @public
 */
export interface DeploymentPipelineResponse {
  name: string;
  displayName?: string;
  description?: string;
  orgName: string;
  createdAt: string;
  status?: string;
  promotionPaths?: PromotionPath[];
}
