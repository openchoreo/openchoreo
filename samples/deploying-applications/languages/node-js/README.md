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

## 1. Deploy in Choreo

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/languages/node-js/reading-list-service.yaml
``` 

## 2. Check the build workflow status

You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build reading-list-node-service-build-01 --organization default-org --project default-project --component reading-list-node-service
```

> [!NOTE]
> The build will take around 5 minutes depending on the network speed.

## 3. Check the deployment status

You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment reading-list-node-service-development-deployment-01 --organization default-org --project default-project --component reading-list-node-service
```

You will see an output similar to the following:

```
=== Pod: reading-list-node-service-reading-list-node-service-main-1kp95s ===

> nodejs-sample-rest-api@1.0.0 start
> node index.mjs

listening on http://localhost:8080
```

## 4. Invoke the service

For this sample, we will use kubectl port-forward to access the service.

I. Run the following command to port-forward the gateway.

    ```bash
    kubectl -n choreo-system port-forward svc/choreo-external-gateway 8443:443
    ```

II. Invoke the service.

   Add a new book
   ```bash
   curl -k -X POST https://dev.choreoapis.localhost:8443/default-project/reading-list-node-service/reading-list/books \
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Galactic Nomad",
   "author": "Nova Starling",
   "status": "reading"
   }'
   ```

> [!TIP]
> The response will include the UUID of the newly added book that we can
> export into a environment variable for further operations.
> ```bash
> export BOOK_UUID=<uuid>
> ```

   Retrieve a book by id
   ```bash
   curl -k https://dev.choreoapis.localhost:8443/default-project/reading-list-node-service/reading-list/books/${BOOK_UUID}
   ```

   List all books
   ```bash
   curl -k https://dev.choreoapis.localhost:8443/default-project/reading-list-node-service/reading-list/books
   ```

   Update a book
   ```bash
   curl -k -X PUT https://dev.choreoapis.localhost:8443/default-project/reading-list-node-service/reading-list/books/${BOOK_UUID} \
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Galactic Nomad",
   "author": "Nova Starling",
   "status": "read"
   }'
   ```

   Delete a book
   ```bash
   curl -k -X DELETE https://dev.choreoapis.localhost:8443/default-project/reading-list-node-service/reading-list/books/${BOOK_UUID}
   ```

## 5. Clean up

To clean up the resources created by this sample, run the following command.

```bash
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/languages/node-js/reading-list-service.yaml
```
