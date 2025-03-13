# Reading List Python Service

## Overview
The Reading List Python Service provides functionalities to manage a reading list, including:
- Adding a new book to the reading list
- Retrieving details of a specific book by ID
- Listing all books in the reading list
- Deleting a book from the reading list by ID
- Updating the reading status of a book by ID

The service exposes several REST endpoints for performing these operations.

### Add a new book
**Endpoint:** `/books`  
**Method:** `POST`  
**Functionality:** Adds a new book by sending a JSON payload.

### Retrieve a book by ID
**Endpoint:** `/books/{id}`  
**Method:** `GET`  
**Functionality:** Retrieves book details by their id (auto-generated).

### List all books
**Endpoint:** `/books`  
**Method:** `GET`  
**Functionality:** Retrieves all books.

### Delete a book by ID
**Endpoint:** `/books/{id}`  
**Method:** `DELETE`  
**Functionality:** Removes a book from the reading list by its ID.

### Update the status of a book
**Endpoint:** `/books/{id]`  
**Method:** `PUT`  
**Functionality:** Updates the reading status of a book.

## Deploy in Choreo

```bash
choreoctl apply -f samples/deploying-applications/languages/python/reading-list-service.yaml
``` 

## Checking the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --organization default-org --build reading-list-python-service-build-01 --project default-project --component reading-list-python-service
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --organization default-org --deployment reading-list-python-service-development-deployment-1 --project default-project --component reading-list-python-service
```

Note: You should see a k8s namespace created for your org, project and environment combination.

## Invoke the service
For this sample, we will use kubectl port-forward to access the web application.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Now you can Invoke the endpoints using the following URL.
    ```bash
    https://development.apis.choreo.localhost:4430/default-project/reading-list-python-service/reading-list
   ```
   
2. Invoke the service
   
   Add a new book
   ```bash
   curl -k -X POST https://development.apis.choreo.localhost:4430/default-project/reading-list-python-service/reading-list/books \
   -H "Content-Type: application/json" \
   -d '{
   "author": "Nova Starling",
   "name": "The Galactic Nomad",
   "status": "reading"
   }'
   ```
   
   Retrieve a book by id
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/reading-list-python-service/reading-list/books/1
   ```
   
   List all books
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/reading-list-python-service/reading-list/books
   ```

   Update a book
   ```bash
    curl -k -X PUT https://development.apis.choreo.localhost:4430/default-project/reading-list-python-service/reading-list/books/1
   -H "Content-Type: application/json" \
   -d '{
   "id": "1",
   "status": "read"
   }'
   ```

   Delete a book
   ```bash
    curl -k -X DELETE https://development.apis.choreo.localhost:4430/default-project/reading-list-python-service/reading-list/books/1
   ```
   