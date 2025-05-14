// Define a proper type for the Kubernetes resources
export interface KubernetesResource {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    [key: string]: any;
  };
  spec?: any;
  status?: any;
  clusterName: string;
  [key: string]: any;
}

export const ChoreoPrefix = 'core.choreo.dev/';
