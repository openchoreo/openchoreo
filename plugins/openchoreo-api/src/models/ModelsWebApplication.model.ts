export interface WebApplicationOwner {
  projectName: string;
  componentName: string;
}

export interface WebApplicationSpec {
  owner: WebApplicationOwner;
  workloadName: string;
  className: string;
  overrides?: { [key: string]: boolean };
}

export interface WebApplicationStatus {
  // Future: Add status fields as needed
}

/**
 * @public
 */
export interface ModelsWebApplication {
  spec: WebApplicationSpec;
  status?: WebApplicationStatus;
}
