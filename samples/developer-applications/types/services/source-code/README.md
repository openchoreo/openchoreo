# Reading List Service
The Reading List Service allows you to manage a collection of books, including:
- Adding a new book
- Retrieving book details by ID
- Updating book information
- Deleting a book
- Listing all books

The service exposes several REST endpoints for performing these operations.

### Add a new book
**Endpoint:** `/reading-list/books`  
**Method:** `POST`  
**Functionality:** Adds a new book to the reading list by sending a JSON payload.

### Retrieve a book by ID
**Endpoint:** `/reading-list/books/{id}`  
**Method:** `GET`  
**Functionality:** Retrieves book details by their ID.

### Update a book
**Endpoint:** `/reading-list/books/{id}`  
**Method:** `PUT`  
**Functionality:** Updates book information by sending a JSON payload.

### Delete a book
**Endpoint:** `/reading-list/books/{id}`  
**Method:** `DELETE`  
**Functionality:** Deletes a book from the reading list.

### List all books
**Endpoint:** `/reading-list/books`  
**Method:** `GET`  
**Functionality:** Retrieves all books from the reading list.

## Deploy in Choreo
The following command will create the component, deployment track, and deployment in Choreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f samples/applications/types/services/source-code/reading-list-service.yaml
```

## Check the Argo Workflow Status
The Argo Workflow will create three tasks for building and deploying the service:

NAMESPACE	            NAME
choreo-ci-default-org	reading-list-service-build-bfe565e2-clone-step-2854902034
choreo-ci-default-org	reading-list-service-build-bfe565e2-build-step-1687356778
choreo-ci-default-org	reading-list-service-build-bfe565e2-push-step-2561049231

You can check the status of the workflow by running the following command:

```bash
kubectl get pods -n choreo-ci-default-org
```
## Check Build Logs
You can check the build logs of each step by running the following commands:

```bash
kubectl -n choreo-ci-default-org logs -l workflow=reading-list-service-build,step=clone-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=reading-list-service-build,step=build-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=reading-list-service-build,step=push-step --tail=-1
```
## Check the Deployment Status
You should see a namespace created for your org, project and environment combination. In this sample it will have the prefix `dp-default-org-default-project-development-`.

List all the namespaces in the cluster to find the namespace created for the deployment.

```bash
kubectl get namespaces
``` 

You can check the status of the deployment by running the following commands.

```bash             
kubectl get pods -n dp-default-org-default-project-development-39faf2d8
```

## Invoke the service
For this sample, we will use kubectl port-forward to access the web application.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Now you can Invoke the endpoints using the following URL.
    ```bash
    https://development.apis.choreo.localhost:4430/default-project/reading-list-service/api/v1/reading-list
   ```

2. Invoke the service

   Add a new book
   ```bash
   curl -k -X POST https://development.apis.choreo.localhost:4430/default-project/reading-list-service/api/v1/reading-list/books \
   -H "Content-Type: application/json" \
   -d '{
   "id": "12",
   "title": "The Catcher in the Rye",
   "author": "J.D. Salinger",
   "status": "reading"
   }'
   ```

   Retrieve a book by ID
      ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/reading-list-service/api/v1/reading-list/books/12
   ```

   Update a new book
   ```bash
   curl -k -X PUT https://development.apis.choreo.localhost:4430/default-project/reading-list-service/api/v1/reading-list/books/12 \
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Catcher in the Rye",
   "author": "J.D. Salinger",
   "status": "read"
   }'
   ```
   
   Delete a book by ID
   ```bash
   curl -k -X DELETE https://development.apis.choreo.localhost:4430/default-project/reading-list-service/api/v1/reading-list/books/12
   ```

   Delete a book by ID
   ```bash
   curl -k https://development.apis.choreo.localhost:4430/default-project/reading-list-service/api/v1/reading-list/books
   ```
