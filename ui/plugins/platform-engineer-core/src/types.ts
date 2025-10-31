export interface Environment {
  name: string;
  namespace: string;
  displayName: string;
  description: string;
  organization: string;
  dataPlaneRef: string;
  isProduction: boolean;
  dnsPrefix: string;
  createdAt: string;
  status: string;
  componentCount?: number;
}

export interface DataPlane {
  name: string;
  namespace?: string;
  displayName?: string;
  description?: string;
  organization: string;
  registryPrefix?: string;
  registrySecretRef?: string;
  kubernetesClusterName?: string;
  apiServerURL?: string;
  publicVirtualHost?: string;
  organizationVirtualHost?: string;
  observerURL?: string;
  observerUsername?: string;
  createdAt?: string;
  status?: string;
}

export interface DataPlaneWithEnvironments extends DataPlane {
  environments: Environment[];
}
