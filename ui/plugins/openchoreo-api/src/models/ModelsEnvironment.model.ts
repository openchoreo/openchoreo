export interface ModelsEnvironment {
  name: string;
  namespace: string;
  displayName: string;
  description: string;
  dataPlaneRef: string;
  isProduction: boolean;
  dnsPrefix: string;
  createdAt: string;
  status: string;
}

export interface EnvironmentsGetRequest {
  orgName: string;
}
