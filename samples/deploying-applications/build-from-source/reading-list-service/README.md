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
kubectl apply -f samples/deploying-applications/build-from-source/reading-list-service/reading-list-service.yaml
```

## Check the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build reading-list-service-build-01 --organization default-org --project default-project --component reading-list-service
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment reading-list-service-development-deployment-01 --organization default-org --project default-project --component reading-list-service
```

Note: You should see a k8s namespace created for your org, project and environment combination.

## Invoke the service
For this sample, we will use kubectl port-forward to access the service.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Note: You can find the <hash> part of the gateway name by running the following command:
    ```bash
    kubectl -n choreo-system get services
   ```

2. Invoke the service.

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
