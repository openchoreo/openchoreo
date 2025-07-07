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
 * Standard OpenChoreo API response wrapper
 * @public
 */
export interface OpenChoreoApiResponse<T> {
  success: boolean;
  data: PaginatedData<T>;
}

/**
 * Response type for projects endpoints
 * @public
 */
export type ProjectsResponse = TypedResponse<OpenChoreoApiResponse<any>>;
