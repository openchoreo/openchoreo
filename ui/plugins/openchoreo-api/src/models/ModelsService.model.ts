import { EndpointTemplateSpec } from './ModelsAPI.Model';

/**
 * @public
 */
export interface ServiceOwner {
  projectName: string;
  componentName: string;
}

export interface ModelsService {
  owner: ServiceOwner;
  workloadName: string;
  className: string;
  overrides?: { [key: string]: boolean };
  apis?: { [key: string]: EndpointTemplateSpec };
}
