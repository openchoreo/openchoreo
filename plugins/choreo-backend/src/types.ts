import { Project } from 'choreo-cell-diagram';

export interface EnvironmentService {
  fetchDeploymentInfo(request: {
    projectName: string;
    componentName: string;
    organizationName: string;
  }): Promise<Environment[]>;
  
  promoteComponent(request: {
    sourceEnvironment: string;
    targetEnvironment: string;
    componentName: string;
    projectName: string;
    organizationName: string;
  }): Promise<Environment[]>;
}

export interface EndpointInfo {
  name: string;
  type: string;
  url: string;
  visibility: 'project' | 'organization' | 'public';
}

export interface Environment {
  name: string;
  deployment: {
    status: 'success' | 'failed' | 'pending' | 'not-deployed' | 'suspended';
    lastDeployed?: string;
    image?: string;
    statusMessage?: string;
  };
  endpoints: EndpointInfo[];
  promotionTargets?: {
    name: string;
    requiresApproval?: boolean;
    isManualApprovalRequired?: boolean;
  }[];
}

export type ObjectToFetch = {
  group: string;
  apiVersion: string;
  plural: string;
  objectType: 'customresources';
};

export const environmentChoreoWorkflowTypes: ObjectToFetch[] = [
  {
    group: 'core.choreo.dev',
    apiVersion: 'v1',
    plural: 'environments',
    objectType: 'customresources',
  },
  {
    group: 'core.choreo.dev',
    apiVersion: 'v1',
    plural: 'deployments',
    objectType: 'customresources',
  },
  {
    group: 'core.choreo.dev',
    apiVersion: 'v1',
    plural: 'endpoints',
    objectType: 'customresources',
  },
];

export const cellChoreoWorkflowTypes: ObjectToFetch[] = [
  {
    group: 'core.choreo.dev',
    apiVersion: 'v1',
    plural: 'projects',
    objectType: 'customresources',
  },
  {
    group: 'core.choreo.dev',
    apiVersion: 'v1',
    plural: 'components',
    objectType: 'customresources',
  },
  {
    group: 'core.choreo.dev',
    apiVersion: 'v1',
    plural: 'endpoints',
    objectType: 'customresources',
  },
];

export interface CellDiagramService {
  fetchProjectInfo(request: {
    projectName: string;
    orgName: string;
  }): Promise<Project | undefined>;
}

export interface RuntimeLogsService {
  fetchRuntimeLogs(request: {
    componentId: string;
    namespace: string;
    environmentId: string;
    logLevels?: string[];
    startTime?: string;
    endTime?: string;
    limit?: number;
    offset?: number;
  }): Promise<RuntimeLogsResponse>;
}

export interface LogEntry {
  timestamp: string;
  log: string;
  logLevel: 'ERROR' | 'WARN' | 'INFO' | 'DEBUG';
  componentId: string;
  environmentId: string;
  projectId: string;
  version: string;
  versionId: string;
  namespace: string;
  podId: string;
  containerName: string;
  labels: Record<string, string>;
}

export interface RuntimeLogsResponse {
  logs: LogEntry[];
  totalCount: number;
  tookMs: number;
}
