# Deploy a Web Application in OpenChoreo using choreoctl

This section guides you through creating, deploy and accessing a sample Web Application using `choreoctl`.

If you haven't installed OpenChoreo & `choreoctl` yet, please follow the [installation guide](../../../../docs/install-guide.md/#install-the-choreoctl) to install them.

> [!IMPORTANT]
> For this sample, we are assuming that you have set the default organization, default project properly in your `choreoctl` context.
> To verify the current context, run the following command:
> ```shell
> choreoctl config current-context
> ```
> You should see the following output:
> ```text
> PROPERTY         VALUE
> Current Context  default
> Organization     default-org
> Project          default-project
> Component        -
> Environment      development
> Data Plane       default-dataplane
> ```

## Step 1 - Create the sample Web Application component

For this, you will be using a sample Web Application component from the [awesome-compose](https://github.com/docker/awesome-compose).

Run the following command to create a sample Web Application component in OpenChoreo.

```shell
choreoctl create component --name hello-world --type WebApplication --git-repository-url https://github.com/docker/awesome-compose --branch master --path /react-nginx --buildpack-name React --buildpack-version 18.20.6
```

> [!TIP]
> You could also create the component using the interactive mode by running `choreoctl create component -i`. 
> Please make sure to provide the required inputs as it prompts.

You will see the following output:

```text
Component 'hello-world' created successfully in project 'default-project' of organization 'default-org'
```

## Step 2 - Build the created sample component

Create a build resource for hello-world component using `choreoctl` interactive mode.

```shell
choreoctl create build -i
```

Use the build name as 'b1' and keep other inputs as defaults.

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

To view the build logs:

```shell
choreoctl logs --type build --component hello-world --build b1 --follow
```

> [!NOTE]
> The build step will take around 5 minutes to get all the dependencies and complete the build.

To see the build status use the following command:

```shell
choreoctl get build --component hello-world  b1
```

> [!NOTE]
> Proceed to the next step after build  is in `Ready (BuildImageSucceeded)` status.

## Step 4 - View the generated deployable artifact

As part of the successful build, a deployable artifact resource is created to trigger a deployment.

You can check the deployable artifact using the following command:

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

## Step 7 - Access the deployed Web Application

You can quickly access the deployed Web Application by port-forwarding the OpenChoreo external gateway service.

Run the following command to port-forward the gateway:

```bash
kubectl port-forward svc/choreo-external-gateway -n choreo-system 8443:443
```

> [!TIP]
> To learn more on exposing the OpenChoreo external gateway, please refer to our [installation](../../../../docs/install-guide.md#exposing-the-openchoreo-gateway) guide.

## Step 7 - View deployment logs

You can view the deployment logs using the following command:

```shell
choreoctl logs --type deployment --component hello-world --deployment dev-deployment --follow
```

[//]: # (Todo: Uncomment this once we implemented the deletion via `choreoctl` properly.)
[//]: # (## Clean up)

[//]: # ()
[//]: # (To clean up the resources created in this guide, you can delete the component, build, deployment, and endpoint resources.)

[//]: # ()
[//]: # (```shell)

[//]: # (choreoctl delete component hello-world)

[//]: # (```)
