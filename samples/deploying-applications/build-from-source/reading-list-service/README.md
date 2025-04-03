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

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/go-reading-list-rest-api

## Deploy in Choreo

The following command will create the component, deployment track, and deployment in Choreo. It will also trigger a build by creating a build resource.

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/build-from-source/reading-list-service/reading-list-service.yaml
```

## Check the Build Workflow Status

You can check the build workflow status by running the following command.

```bash
choreoctl get build reading-list-service-build-01 --component reading-list-service
```

## Check the Build Workflow Logs

You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build reading-list-service-build-01 --organization default-org --project default-project --component reading-list-service
```

> [!NOTE]
> The build will take around 5 minutes depending on the network speed.

## Check the Deployment Status

You can check the deployment status by running the following command.

```bash
choreoctl get deployment --component reading-list-service
```

## Check the Deployment Logs

You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment reading-list-service-development-deployment-01 --organization default-org --project default-project --component reading-list-service
```

## Invoke the Service

For this sample, we will use kubectl port-forward to access the service.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/choreo-external-gateway -n choreo-system 8443:443
    ```

2. Invoke the service.

   Add a new book:

   ```bash
   curl -k -X POST https://dev.choreoapis.localhost:8443/default-project/reading-list-service/api/v1/reading-list/books \
   -H "Content-Type: application/json" \
   -d '{
   "id": "12",
   "title": "The Catcher in the Rye",
   "author": "J.D. Salinger",
   "status": "reading"
   }'
   ```

   Retrieve the book by ID:

   ```bash
   curl -k https://dev.choreoapis.localhost:8443/default-project/reading-list-service/api/v1/reading-list/books/12
   ```

   Update a new book:

   ```bash
   curl -k -X PUT https://dev.choreoapis.localhost:8443/default-project/reading-list-service/api/v1/reading-list/books/12 \
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Catcher in the Rye",
   "author": "J.D. Salinger",
   "status": "read"
   }'
   ```
   
   Delete a book by ID:

   ```bash
   curl -k -X DELETE https://dev.choreoapis.localhost:8443/default-project/reading-list-service/api/v1/reading-list/books/12
   ```

   Delete all books:

   ```bash
   curl -k https://dev.choreoapis.localhost:8443/default-project/reading-list-service/api/v1/reading-list/books
   ```

## Clean up

To clean up the resources created by this sample, run the following command.

```bash
choreoctl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/build-from-source/reading-list-service/reading-list-service.yaml
```
