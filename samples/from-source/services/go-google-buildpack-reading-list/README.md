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

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-google-buildpack-reading-list/reading-list-service.yaml
```

> [!NOTE]
> The build will take around 8 minutes depending on the network speed.

## Step 2: Port-forward the OpenChoreo Gateway

Port forward the OpenChoreo gateway service to access the frontend locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Test the Application

   Add a new book:

   ```bash
   curl -k -X POST "$(kubectl get servicebinding reading-list-service -o jsonpath='{.status.endpoints[0].public.uri}')/books" \
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
   curl -k "$(kubectl get servicebinding reading-list-service -o jsonpath='{.status.endpoints[0].public.uri}')/books/12"
   ```

   Update a new book:

   ```bash
   curl -k -X PUT "$(kubectl get servicebinding reading-list-service -o jsonpath='{.status.endpoints[0].public.uri}')/books/12" \
   -H "Content-Type: application/json" \
   -d '{
   "title": "The Catcher in the Rye",
   "author": "J.D. Salinger",
   "status": "read"
   }'
   ```
   
   Delete a book by ID:

   ```bash
   curl -k -X DELETE "$(kubectl get servicebinding reading-list-service -o jsonpath='{.status.endpoints[0].public.uri}')/books/12"
   ```

   Delete all books:

   ```bash
   curl -k "$(kubectl get servicebinding reading-list-service -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Find and stop the specific port-forward process
pkill -f "port-forward.*gateway-external.*8443:443"

# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-google-buildpack-reading-list/reading-list-service.yaml
```
