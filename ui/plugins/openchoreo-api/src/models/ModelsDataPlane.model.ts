export interface ModelsDataPlane {
  name: string;
  namespace?: string;
  displayName?: string;
  description?: string;
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
