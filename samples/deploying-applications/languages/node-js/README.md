# Reading List Node.js Service

## Overview
The Reading List Node.js Service provides functionalities to manage a reading list, including:
- Adding a new book to the reading list
- Retrieving details of a specific book by UUID
- Listing all books in the reading list
- Deleting a book from the reading list by UUID
- Updating the reading status of a book by UUID

The service exposes several REST endpoints for performing these operations.

### Add a new book
**Endpoint:** `/books`  
**Method:** `POST`  
**Functionality:** Adds a new book by sending a JSON payload.

### Retrieve a book by ID
**Endpoint:** `/books/{uuid}`  
**Method:** `GET`  
**Functionality:** Retrieves book details by their UUID (auto-generated).

### List all books
**Endpoint:** `/books`  
**Method:** `GET`  
**Functionality:** Retrieves all books.

### Delete a book by ID
**Endpoint:** `/books/{uuid}`  
**Method:** `DELETE`  
**Functionality:** Removes a book from the reading list by its UUID.

### Update the status of a book
**Endpoint:** `/books/{uuid]`  
**Method:** `PUT`  
**Functionality:** Updates the reading status of a book.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/reading-books-list-service-nodejs

## Deploy in Choreo

```bash
choreoctl apply -f samples/deploying-applications/languages/node-js/reading-list-service.yaml
``` 

## Check the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build reading-list-node-service-build-01 --organization default-org --project default-project --component reading-list-node-service
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment reading-list-node-service-development-deployment-01 --organization default-org --project default-project --component reading-list-node-service
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
   curl -k -X POST https://development.apis.choreo.localhost:4430/default-project/reading-list-node-service/reading-list/books \
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Galactic Nomad",
   "author": "Nova Starling",
   "status": "reading"
   }'
   ```
   
   Retrieve a book by id
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/reading-list-node-service/reading-list/books/<uuid>
   ```
   
   List all books
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/reading-list-node-service/reading-list/books
   ```

   Update a book
   ```bash
    curl -k -X PUT https://development.apis.choreo.localhost:4430/default-project/reading-list-node-service/reading-list/books/<uuid>
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Galactic Nomad",
   "author": "Nova Starling",
   "status": "read"
   }'
   ```

   Delete a book
   ```bash
    curl -k -X DELETE https://development.apis.choreo.localhost:4430/default-project/reading-list-node-service/reading-list/books/<uuid>
   ```
   