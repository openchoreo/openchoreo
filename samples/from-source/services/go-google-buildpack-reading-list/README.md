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

The following command will create the relevant resources in OpenChoreo. It will also trigger a workflow by creating a workflow resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-source/services/go-google-buildpack-reading-list/reading-list-service.yaml
```

> [!NOTE]
> The Google Cloud Buildpack workflow will take around 5-8 minutes depending on the network speed and system resources.

## Step 2: Monitor the Workflow

After deploying, monitor the workflow progress:

```bash
# Check ComponentWorkflowRun status
kubectl get componentworkflowrun reading-list-service-build-01 -n default -o jsonpath='{.status.conditions}' | jq .

# Watch workflow pods (in openchoreo-ci-default namespace)
kubectl get pods -n openchoreo-ci-default | grep reading-list

# View workflow logs (replace <pod-name> with actual pod name)
kubectl logs -n openchoreo-ci-default <pod-name> -f
```

Wait for the ComponentWorkflowRun to complete successfully. You should see:
- `WorkflowCompleted: True`
- `WorkflowSucceeded: True`
- `WorkloadUpdated: True`

## Step 3: Verify Deployment

After the workflow completes, verify the deployment is ready:

```bash
# Check ReleaseBinding status
kubectl get releasebinding reading-list-service-development -n default -o jsonpath='{.status.conditions}' | jq .

# Verify deployment is ready
kubectl get deployment -A -l openchoreo.dev/component=reading-list-service
```

## Step 4: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.dev/component=reading-list-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.dev/component=reading-list-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Add a new book

```bash
curl -X POST "http://${HOSTNAME}:19080${PATH_PREFIX}/api/v1/reading-list/books" \
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
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/api/v1/reading-list/books/12"
```

### Update a book

```bash
curl -X PUT "http://${HOSTNAME}:19080${PATH_PREFIX}/api/v1/reading-list/books/12" \
-H "Content-Type: application/json" \
-d '{
"title": "The Catcher in the Rye",
"author": "J.D. Salinger",
"status": "read"
}'
```

### Delete a book by ID

```bash
curl -X DELETE "http://${HOSTNAME}:19080${PATH_PREFIX}/api/v1/reading-list/books/12"
```

### List all books

```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/api/v1/reading-list/books"
```

## Troubleshooting

### Workflow Issues

If the workflow fails or takes too long:

1. **Check ComponentWorkflowRun status and conditions:**
   ```bash
   kubectl get componentworkflowrun reading-list-service-build-01 -n default -o jsonpath='{.status.conditions}' | jq .
   ```

2. **Check workflow pod status:**
   ```bash
   kubectl get pods -n openchoreo-ci-default | grep reading-list
   ```

3. **View workflow pod logs for errors:**
   ```bash
   # Get the pod name
   POD_NAME=$(kubectl get pods -n openchoreo-ci-default -l component-workflows.argoproj.io/workflow=reading-list-service-build-01 --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')

   # View logs
   kubectl logs -n openchoreo-ci-default $POD_NAME
   ```

4. **Check if Workload was created after workflow:**
   ```bash
   kubectl get workload -n default | grep reading-list
   ```

### Deployment Issues

If the application is not accessible:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding reading-list-service-development -n default -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding reading-list-service-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=reading-list-service -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=reading-list-service
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=reading-list-service -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=reading-list-service --tail=50
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-source/services/go-google-buildpack-reading-list/reading-list-service.yaml
```
