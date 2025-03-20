# Book Store Sample with Organization-Level Service Visibility

This sample demonstrates how to deploy two interconnected applications in Choreo: a backend service and a web application. The frontend communicates with the backend service internally.

Choreo enables this communication through organization-level visibility, allowing seamless interaction between internal services.

## What does this sample do?

This sample deploys:

1. A Reading List Service (backend) in the "default-project"
2. A Reading List Web Application (frontend) in the "portal" project
3. Configures organization-level visibility to enable secure communication between the applications within the cluster

## Architecture overview

- **Reading List Service**: A backend service that manages the reading list data
- **Reading List Web App**: A server-side rendered (SSR) web application that consumes the backend service
- The web app makes API calls to the service through Choreo's internal network using organization-level visibility

## Pre-requisites

- Kubernetes cluster with Choreo installed
- The `choreoctl` CLI tool installed
- Docker images:
  - `ghcr.io/choreo-idp/samples/reading-list-service`
  - `ghcr.io/choreo-idp/samples/reading-list-webapp`

## Understand the project structure

- **Default Project**: Contains the Reading List Service
  - Handles data management and business logic
  - Exposed internally to the organization

- **Portal Project**: Contains the Reading List Web Application
  - Provides the user interface
  - Communicates with the service through internal endpoints
  - Configured for server-side rendering

## 1. Deploy the applications

I. First, deploy the `Reading List Service`:

```shell
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/org-visibility/reading-list-service.yaml
```

II. Then, deploy the `Reading List Web Application`:

```shell
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/org-visibility/reading-list-webapp.yaml
```

## 2. Verify the deployment

I. Check the reading list service deployment.

```shell
choreoctl get deployments --organization default-org --project default-project --component reading-list-service
```
II. Check the web application deployment.

```shell
choreoctl get deployments --organization default-org --project portal --component reading-list-webapp
```

## 3. Expose the API gateway locally

Port forward the Choreo gateway service to access it locally.

```bash
kubectl port-forward -n choreo-system svc/choreo-external-gateway 8443:443
```

## 4. Access the web application

Access the web application from your browser using the following URL.

```bash
https://reading-list-webapp-development.choreoapps.localhost:8443
```

## 5. Clean Up

To remove all deployed resources, use the following command.

```shell
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/org-visibility/reading-list-service.yaml
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/org-visibility/reading-list-webapp.yaml
```

> [!NOTE]
> #### Organization-Level Service Visibility
> 
> The Reading List Service is configured to be visible at the organization level through the following configuration in `book-store-service.yaml`:
> 
> ```yaml
> networkVisibilities:
>   public:
>     enable: false
>   organization:
>     enable: true
> ```
>
> #### Internal Service Communication
>
> The web application is configured to communicate with the service using Choreo's internal DNS. This is set up through an environment variable in `book-store-webapp.yaml`:
>
> ```yaml
> env:
> - key: READING_LIST_SERVICE_URL
>   value: https://dev.choreoapis.internal/default-project/reading-list-service/api/v1/reading-list
> ```
>
> #### Server-Side Rendering (SSR)
>
> The web application uses SSR to make API calls from within the cluster, ensuring secure and efficient communication between the frontend and backend services.

> [!TIP]
> #### Troubleshoot
>
> - If the web application can't connect to the service, verify:
>   - The service deployment is running
>   - Organization-level visibility is properly configured
>   - The service URL in the webapp's environment variables is correct
> 
> - For deployment issues, check the logs:
> 
> ```shell
> choreoctl logs --type=deployment --organization default-org --project default-project --component reading-list-service
> choreoctl logs --type=deployment --organization default-org --project portal --component reading-list-webapp
> ```
