# Deploy a Web Application in Choreo using choreoctl

This section guides you through creating, deploy and accessing a sample Web Application using `choreoctl`.

If you haven't installed Choreo & `choreoctl` yet, please follow the [installation guide](../../../../docs/install-guide.md/#install-the-choreoctl) to install them.

## Step 1 - Create the sample Web Application component

For this, you will be using a sample Web Application component from the [awesome-compose](https://github.com/docker/awesome-compose).

Run the following command to create a sample Web Application component in Choreo.

```shell
choreoctl create component --name hello-world --type WebApplication --git-repository-url https://github.com/docker/awesome-compose --branch master --path /react-nginx --buildpack-name React --buildpack-version 18.20.6
```

You will see the following output:

```text
Component 'hello-world' created successfully in project 'default-project' of organization 'default-org'
```

## Step 2 - Build the created sample component

Create a build resource for hello-world component using Choreo CLI interactive mode.

```shell
choreoctl create build -i
```
Use the build name as 'build1' and keep other inputs as defaults.
```text
$ choreoctl create build -i
Selected inputs:
- organization: default-org
- project: default-project
- component: hello-world
- deployment track: default
- name: b1
- revision: latest
Enter git revision (optional, press Enter to use latest):
Build 'b1' created successfully for component 'hello-world' in project 'default-project' of organization 'default-org'
```

## Step 3 - View build logs and status
```shell
choreoctl logs --type build --component hello-world --build b1 --follow
```
> [!NOTE]
> The build step will take around 5 minutes to get all the dependencies and complete the build.

See the build status using get build resource command.
```shell
choreoctl get build --component hello-world  b1
```
> [!NOTE]
> Proceed to the next step after build  is in `Ready (BuildImageSucceeded)` status.

## Step 4 - View the generated deployable artifact

As part of the successful build, a deployment artifact resource is created to trigger a deployment.
```shell
choreoctl get deployableartifact --component hello-world
```
## Step 5 - Create a deployment resource

For this option, we will explore the interactive mode which guide you to create the deployment resource.
```shell
choreoctl create deployment -i
```
Name the deployment as 'dev-deployment'. Following is a sample CLI output.
```text
$ choreoctl create deployment -i
Selected resources:
- organization: default-org
- project: default-project
- component: hello-world
- deployment track: default
- environment: development
- deployable artifact: default-org-default-project-hello-world-default-b1-6179fb65
- name: dev-deployment
Enter deployment name:
Deployment 'dev-deployment' created successfully in environment 'development' for component 'hello-world' of project 'default-project' in organization 'default-org'
```

## Step 6 - View the generated endpoint resource

As part of the successful deployment, an endpoint resource is created to access the deployed component.

```shell
choreoctl get endpoint --component hello-world
```
You should see a similar output as follows.

```text
NAME     TYPE   ADDRESS                                                                                 STATUS                  AGE   COMPONENT     PROJECT           ORGANIZATION   ENVIRONMENT
webapp   HTTP   https://default-org-default-project-hello-world-d0366d03-development.choreo.localhost   Ready (EndpointReady)   8h    hello-world   default-project   default-org    development
```

> [!NOTE]
> Now you can access the deployed Web Application using the generated endpoint URL. Please make sure you have exposed the Choreo external gateway to your lost machine to access the endpoint URL.
> To learn how to expose the Choreo external gateway, please refer to our [installation] guide

## Step 7 - View deployment logs

You can view the deployment logs using the following command:

```shell
choreoctl logs --type deployment --component hello-world --deployment dev-deployment --follow
```