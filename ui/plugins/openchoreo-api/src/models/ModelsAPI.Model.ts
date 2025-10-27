export enum EndpointType {
  REST = 'rest',
  GRPC = 'grpc',
  TCP = 'tcp',
}

export enum HTTPMethod {
  GET = 'GET',
  POST = 'POST',
  PUT = 'PUT',
  DELETE = 'DELETE',
  PATCH = 'PATCH',
  HEAD = 'HEAD',
  OPTIONS = 'OPTIONS',
}

export enum RESTOperationExposeLevel {
  Project = 'Project',
  Organization = 'Organization',
  Public = 'Public',
}

export interface HTTPBackend {
  port: number;
  basePath?: string;
}

export interface RESTEndpointOperation {
  method: HTTPMethod;
  path: string;
  description?: string;
  scopes?: string[];
  exposeLevels?: RESTOperationExposeLevel[];
}

export interface RESTEndpoint {
  backend?: HTTPBackend;
  exposeLevels?: RESTOperationExposeLevel[];
  operations?: RESTEndpointOperation[];
}

export interface EndpointTemplateSpec {
  className?: string;
  type: EndpointType;
  rest?: RESTEndpoint;
}
