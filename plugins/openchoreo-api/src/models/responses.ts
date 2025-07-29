/**
 * Response types for OpenChoreo API
 * @public
 */

/**
 * Wraps the Response type to convey a type on the json call.
 * @public
 */
export type TypedResponse<T> = Omit<Response, 'json'> & {
  json: () => Promise<T>;
};

/**
 * Paginated data wrapper for OpenChoreo API responses
 * @public
 */
export interface PaginatedData<T> {
  items: T[];
  totalCount: number;
  page: number;
  pageSize: number;
}

/**
 * Standard OpenChoreo API response wrapper for paginated data
 * @public
 */
export interface OpenChoreoApiResponse<T> {
  success: boolean;
  data: PaginatedData<T>;
}

/**
 * OpenChoreo API response wrapper for single object responses
 * @public
 */
export interface OpenChoreoApiSingleResponse<T> {
  success: boolean;
  data: T;
}

/**
 * Response type for projects GET endpoints (paginated)
 * @public
 */
export type ProjectsGetResponse = TypedResponse<OpenChoreoApiResponse<any>>;

/**
 * Response type for projects POST endpoints (single object)
 * @public
 */
export type ProjectsPostResponse = TypedResponse<OpenChoreoApiSingleResponse<any>>;

/**
 * Connection method configuration for observer URL
 * @public
 */
export interface ConnectionMethod {
  type: string;
  username: string;
  password: string;
}

/**
 * Observer URL data response
 * @public
 */
export interface ObserverUrlData {
  observerUrl: string;
  connectionMethod: ConnectionMethod;
}
