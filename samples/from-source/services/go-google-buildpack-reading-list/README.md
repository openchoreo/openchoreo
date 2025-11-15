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

## Step 3: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.org/component=reading-list-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.org/component=reading-list-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Add a new book

```bash
curl -X POST "http://${HOSTNAME}:9080${PATH_PREFIX}/api/v1/reading-list/books" \
-H "Content-Type: application/json" \
-d '{
"id": "12",
"title": "The Catcher in the Rye",
"author": "J.D. Salinger",
"status": "reading"
}'
```

### Retrieve the book by ID

```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}/api/v1/reading-list/books/12"
```

### Update a book

```bash
curl -X PUT "http://${HOSTNAME}:9080${PATH_PREFIX}/api/v1/reading-list/books/12" \
-H "Content-Type: application/json" \
-d '{
"title": "The Catcher in the Rye",
"author": "J.D. Salinger",
"status": "read"
}'
```

### Delete a book by ID

```bash
curl -X DELETE "http://${HOSTNAME}:9080${PATH_PREFIX}/api/v1/reading-list/books/12"
```

### List all books

```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}/api/v1/reading-list/books"
```

## Clean Up

Remove all resources:

```bash
# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-google-buildpack-reading-list/reading-list-service.yaml
```
