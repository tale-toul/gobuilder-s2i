# Creating the go builder image

## Files and Directories  
| File                   | Required? | Description                                                  |
|------------------------|-----------|--------------------------------------------------------------|
| Dockerfile             | Yes       | Defines the base builder image                               |
| s2i/bin/assemble       | Yes       | Script that builds the application                           |
| s2i/bin/usage          | No        | Script that prints the usage of the builder                  |
| s2i/bin/run            | Yes       | Script that runs the application                             |

### Dockerfile
Use this file to create the container image that can be used as _builder image_ for source to image (S2I) build strategy in OpenShift, to create applications images from go source code.

The Dockerfile contains instructions to install all of the necessary tools and libraries that are needed to build and run the go application.  This file will also handle copying the s2i scripts into the created image.

### S2I scripts

#### assemble
This script is run during the S2I build process and is responsible for the actual building of the go application 

#### run
This script will be used to run the go application and will be set as the CMD for the resulting application image.  To make sure that signals are correctly propagated to the container, the application should be started using the __exec__ command.

#### usage (optional) 
This script will print out instructions on how to use the image.

## Making the builder image available 
To go from the Dockerfile and S2I scripts described above, to a builder image that can be used to transform our go source code into an application image the following steps need to be taken.

* Make the builder image
* Push the builder image to a container registry
* Create an image stream in Openshift referencing the builder image

Each these step has different alternative ways to be executed.

### Making the builder image
To make the builder image, the Dockerfile must be processed.  Two options will be shown for this step:
* Using podman or docker from an independent host
* Running  __oc new-app__ in an Openshift cluster

#### Using podman or docker
To create the builder image using podman or docker the requirements are: Having docker or podman installed in the host; and cloning the git repository containing the Dockerfile and s2i scripts:
```shell
$ sudo yum install -y podman 
$ git clone http://github.com/tale-toul/simple-web
```

Enter the directory where the Dockerfile is located and run the build command.  Both docker and podman use the same options so it does not matter which one is used.  In the case of docker the docker daemon needs to be running, in the case of podman the commands must be run as the root user:

```shell
$ cd simple-web/go_builder_image/
$ sudo podman build -t gobuilder .
```
An image named _localhost/gobuilder_ with tag latest should be added to the host:

```shell
$ sudo podman images
REPOSITORY                            TAG      IMAGE ID       CREATED          SIZE
localhost/gobuilder                   latest   e583efd6a524   20 minutes ago   671 MB
```
This image is not accesible from Openshift, it needs to be pushed to a registry

#### Running __oc new-app__ in an Openshift cluster
S2I supports the creation of container images from a Dockerfile stored in a git repository, therefore it is possible to use the __oc new-app__ command to create the builder image directly from Openshift.  The user running these commands does not require any special permissions in the cluster.
The advantage of this method is that, along with the creation of the builder image, it will also be pushed to the internal Openshift registry and the image stream will be created, so the builder image will be ready to be used from the project where it was created.  If the builder image is to be used from other projects an additional configuration step is required.
The small disadvantage of this method, is that the __oc new-app__ command will try to deploy a container based on the image just created, but since this image is not intended to be run standalone, the deployment will enter a _CrashLoopBackOff_ state and the deployment needs to be manually removed or scaled down to zero.  Also a service is created but is useless in this situation, so it needs to be removed as well.

To run the following commands it is assumed that the user has an active session in an Openshift cluster:
Create the project and run the __oc new-app__ command using the URL of the git repository and directory (context-dir) where the Dockerfile is stored:

```shell
$ oc new-project simplebuildergo
$ oc new-app --name gobuilder https://github.com/tale-toul/simple-web --context-dir go_builder_image
```
The above __oc new-app__ command will return after a few seconds, but the build process will take a few minutes more to complete.  To follow the buil process run:

```shell
$ oc logs -f bc/gobuilder
```
When the build process finishes with the message `Push successful` a deployment is triggered, but as stated before, this will fail.  

```shell
$ oc get pods
NAME                         READY   STATUS             RESTARTS   AGE
gobuilder-1-build            0/1     Completed          0          6m48s
gobuilder-7bf7dbf776-w7f2r   0/1     CrashLoopBackOff   3          2m21s
```
Since this deployment and the service created by __oc new-app__ are of no use, they can be removed from the project. 

```shell
$ oc status
In project simplebuildergo on server https://api.cartapacio.lab.pnq2.cee.redhat.com:6443

svc/gobuilder - 172.30.116.41:8080
  deployment/gobuilder deploys istag/gobuilder:latest <-
    bc/gobuilder docker builds https://github.com/tale-toul/simple-web on istag/ubi8:8.3 
    deployment #2 running for 3 minutes - 0/1 pods (warning: 4 restarts)
    deployment #1 deployed 8 minutes ago

Errors:
  * pod/gobuilder-7bf7dbf776-w7f2r is crash-looping

$ oc delete deployment gobuilder
deployment.apps "gobuilder" deleted

$ oc delete service gobuilder
service "gobuilder" deleted
```

As a result of the __oc new-app__ command the builder image has been created, pushed to the Openshift internal registry and referenced by an image stream

```shell
$ oc get imagestream gobuilder
NAME        IMAGE REPOSITORY                                                                                           TAGS     UPDATED
gobuilder   default-route-openshift-image-registry.apps.cartapacio.lab.pnq2.cee.redhat.com/simplebuildergo/gobuilder   latest   12 minutes ago
```
### Pushing the builder image to a container registry
In the case that the builder image was created with _podman_ or _docker_, or it needs to be available from an external registry, it has to be pushed to the external registry.  In the following example the registry is assumed to be private and needs authentication, both for pushing and pulling images.  Two different ways to push the image will be shown: using podmand (docker) and using skopeo:

#### Using podman or docker
The options for podman and docker are the same, however docker requires the docker daemon to be running, and podman requires the commands to be run as the root user:

Tag the local image with the name of the registry and user:

```shell
$ sudo podman tag localhost/gobuilder quay.io/milponta/gobuilder
```
Log in to the remote registry, assuming only logged in users can push images to the registry.  

```shell
$ sudo podman login -u milponta quay.io
```

Push the image to the registry:
```shell
$ sudo podman push quay.io/milponta/gobuilder
```

#### Using skopeo
The command line tool _skopeo_ can be used to push images to a registry, the advantage of this command is that with a single command it is possible to do the tagging, log in to the registry, and pushing the image:

```shell
$ sudo skopeo copy containers-storage:localhost/gobuilder docker://quay.io/milponta/gobuilder --dest-creds milponta:SuperSecretPass
```
If the registry server is using x509 certificates not know by the local system, the following error message will appear and the image is not pushed:

```
x509: certificate signed by unknown authority`
```
In this case the option __--tls-verify=false__ can be used:

```shell
$ sudo skopeo copy containers-storage:localhost/gobuilder docker://quay.io/milponta/gobuilder --dest-creds milponta:SuperSecretPass --dest-tls-verify=false
```

### Creating the image stream in Openshift
If the image was not build using __oc new-app__ or was pushed to an external registry, an image stream needs to be created.  The reason for this is that the S2I build process that creates the go application always takes the builder image from an image stream.  

If the image registry containing the builder image needs authentication to pull images, the first step is to create a secret containing the credentials to access the registry.  If the registry does not required authenticatino to pull images this step can be skipped.

```shell
$ oc create secret docker-registry extregistry --docker-server quay.io --docker-username milponta --docker-password SuperSecretPass
```

The image stream can be created in the same project where the application is to be deployed or it can be created in a common or shared project where other application projects can also use it.

#### Image stream on the application project
In case the go application and the image stream will reside in the same project, create the image stream with the following command.  If the registry requires credentials, the command will look for a secret valid for the remote registry name:

```shell
$ oc import-image gobuilder --confirm --from quay.io/milponta/gobuilder
```
If the command returns an error about invalid x509 certificates, add the option `--tls_verify=false`

Only if the external registry requires credentials to pull images, the _builder_ service account needs to have explicit access to the secret that was created before:

```shell
$ oc secrets link builder extregistry --for pull,mount
```

#### Image stream on a common project
If the image stream is created on a common project, from which other projects can use it, the __oc import-image__ command will include the option _--reference-policy=local_.  With this option there is need to share the secret or access credentials to the external registry with other projects, if credentials are required to pull images.  If the registry requires credentials, the command will look for a secret valid for the remote registry name:

```shell
$ oc import-image gobuilder --confirm --from quay.io/milponta/gobuilder --reference-policy=local
```
Assuming that no applications will be deployed in this project there is no need to link the _extregistry_ secret with the builder service account.
Service accounts in the projects where applications will be deployed using the _gobuilder_ image stream need access to it, for that the role _system:image-puller_ is used.  For example if the project _simpleweb_ is where the application will be deployed, the command to grant access to that project's service accounts to use the _gobuilder_ image stream is:

```shell
$ oc policy add-role-to-group system:image-puller system:serviceaccounts:simpleweb -n simplebuildergo
```
The above command grants permissions to pull images through the image stream in the project simplebuidergo, where the image stream was created, to all the service accounts in project simpleweb

## Creating the application image
The application image combines the builder image with your applications source code, compiled using the *assemble* script, and run using the *run* script.

The creation of the application image and its deployment can be acomplish with a single __oc new-app__:

If the image stream and the application are going to coexist in the same project:
```shell
$ oc new-app --name simpleweb gobuilder~https://github.com/tale-toul/simple-web
```
The `--name simpleweb` is used to assing a label `app=simpleweb` to the resources created by __oc new-app__
The builder image is specified by prefixing the git repository URL with `gobuilder~`

If the image stream was created on a common project:

```shell
$ oc new-app --name simpleweb common/gobuilder~https://github.com/tale-toul/simple-web
```
The name of project where the image stream was created is prefixed to the name of the image stream.

## Final consideration
* The __oc new-app__ command creates a build config that will combine the builder image with the source code, compile the source code and create the application image.
* The __oc new-app__ command creates a deployment that will run the resulting application container based on the image created by the build config.
* The __oc new-app__ command creates a service resource to provide access to the application service running in the contianer.  The port where the service is listening for connections is obtained from the image definition, in particular from the EXPOSE directive and the label _io.openshift.expose-services_:

```
...
 LABEL io.openshift.expose-services="8080:http" \
...
EXPOSE 8080
```
