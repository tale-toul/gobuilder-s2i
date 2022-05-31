# S2I builder image for golang

## Motivation
Openshift 4 includes a golang builder image that can be used in an [S2I](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md#required-image-contents) process to deploy a pod from go source code using the `oc new-app` command, in a few minutes and with just one command:

```shell
$ oc new-project s2i-go
$ oc new-app --name testero golang~https://github.com/tale-toul/testero 
```
However the included golang builder image has a couple drawbacks:
* The size of the image is quite big:
 
```shell
$ oc describe istag golang:latest -n openshift|grep Size
Image Size:     848.7MB in 5 layers
```
* The image does not expose any network ports, so the `oc new-app` command will not create a service, and the deployment and pod resource definitions will not include any port reference, even if the application listens on a port.  The application network ports can still be used but the configuration must be done manually, and the port numbers and protocols must be known to the administrator.

```shell
$ oc describe istag golang:latest -n openshift|grep -i expose
Exposes Ports:  <none>
```
In addition to the previous points, the included golang image may not work with every go source code project.

Creating your own go s2i builder image can greatly improve the situation while maintaining the benefits of s2i.  The process of [creating the builder image](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md#required-image-contents) is relatively simple and the result is an image that only contains the necessary components for the project, potentially reducing its size considerably; the new builder image also exposes the ports required by the project, and can produce a valid application image.

## S2I versus CI/CD tools
S2i is readily availabe in Openshift, in particular it does not require the installation of any additional components in the cluster.  On the developer workstation the only requirements are: the _oc_ client, and podman or docker, and even these last two are not strictly required. 

S2I is very simple to use, to fully deploy most applications a single `oc new-app` command is enough, only the final step of creating the external route is left out for security reasons, not all applications are meant to be made public.

The S2I process consumes few resources, mostly the application building from source code done by the builder container, based on the builder image; and later the deployment of the application in a pod.

For all the above reasons, S2I fits very well for simple use cases that don't require complicated setups.

On the other hand, S2I lacks many of the features of any traditional CI/CD system.

When using S2I, since the builder image, the source code and the binary application are combined together, the resulting application image will contain the whole development toolset, and the application source code, which may be a security risk, and a waste of space.  The resulting image is much bigger than need be, consuming more storage resources and taking more time to deploy containers based on it, if the pod is deployed on a host that needs to pull the image from a registry.  A traditional CI/CD system cand build the application and create a new image containing only the necessary elements to run the application.  

However CI/CD systems requires the installation of additional components, a complex configuration, and consume a significat ammount of resources.  Making the deployment of such systems a complex and time consuming task, specially for simple use cases.

## Files and Directories for the builder image 
| File                   | Required? | Description                                                  |
|------------------------|-----------|--------------------------------------------------------------|
| Dockerfile             | Yes       | Defines the base builder image                               |
| s2i/bin/assemble       | Yes       | Script that builds the application                           |
| s2i/bin/usage          | No        | Script that prints the usage of the builder                  |
| s2i/bin/run            | Yes       | Script that runs the application                             |

### Dockerfile
The Dockerfile file contains the instructions describing how to create the s2i builder image.  It needs to include all the components required to build the go application from source code. By limiting the components installed to only those required, the size of the image can be reduced considerably in comparison to the golang image included with Openshift.

The Dockerfile file included in this repository contains the following steps:

The image is base on an ubi8 image:
```
FROM registry.access.redhat.com/ubi8:8.3
```
Some variables are defined to be used during the building of the image, and later will also be availabe in the containers created from the resulting image.  
GOPATH and GOCACHE are used by the go tools, APPROOT is defined as the directory where to install the binary application, APPROOT needs to be defined in a separate ENV directive because it uses a previouly defined variable (GOPATH).
```
ENV ...
    GOCACHE=/tmp/src \
    GOPATH=/tmp/go 
ENV APPROOT=$GOPATH/bin
```
Some labels are defined to provide information to Openshift on some aspects of the image, like the network ports it exposes and the location of the s2i scripts.
```
LABEL ...
      io.openshift.expose-services="8080:http" \
      io.openshift.s2i.scripts-url="image:///usr/libexec/s2i" \
      io.openshift.tags="builder,go,golang"
```
The packages required to build go source code are installed, both golang and git have many dependencies that will also be installed.
```
RUN yum install -y --disableplugin=subscription-manager --setopt=tsflags=nodocs golang git && \
    yum clean all -y --disableplugin=subscription-manager 
```
The s2i scripts are copied on the same directory that was defined for the label _io.openshift.s2i.scripts-url_.  More on these scripts later.
```
COPY ./s2i/bin/ /usr/libexec/s2i
```
The containers running in Openshift, based on the go builder image, use random unprivileged users so the directories used by the S2I process must be set accordingly. The 1001 user id is irrelevant, and not guarrantied to be the actual user id used by the containers based on this image.  
The important part here is the group 0.  This is actually a __non privileged__ group, and the container is guarrantied to run using this group id when created inside Openshift. 
```
RUN chown -R 1001:0 /usr/libexec/s2i && \
    chmod -R +x /usr/libexec/s2i/. && \
    mkdir -p $GOPATH/src && \
    mkdir -p $APPROOT && \
    chown -R 1001:0 $GOPATH $APPROOT && \
    chmod -R g=u $GOPATH $APPROOT
USER 1001
```
The workdir is defined 
```
WORKDIR $GOPATH/src
```
The port where the go application provides its services is defined. This information is used by `oc new-app` to create a service:
```
EXPOSE 8080
```
The starting command for any container based on this image is set to the _usage_ script.  This script simply prints and usage message.  The builder image is not intended to be used on its own, that is why its starting command does not do anything useful
```
CMD ["/usr/libexec/s2i/usage"]
```

### S2I scripts
The _assemble_, _run_, _save-artifacts_ and _usage_ scripts must be present in the builder image at the directory specified by the label __io.openshift.s2i.scripts-url__, usually /usr/libexec/s2i.  The _assemble_ and _run_ scripts are required, _save-artifacts_ and _usage_ are not required.  In this repository the save-artifacts is not present.

As seen in the previous section, the scripts are copied from the s2i/bin directory in the local host to /usr/libexec/s2i in the container image, for example the assemble script is at /usr/libexec/s2i/assemble, inside the container image.  

#### assemble
This script is run during the S2I build process and is responsible for the actual building of the go application.  In this repository the _assemble_ script is very simple.

This script is run from the directory defined with the  _WORDIR_ directive in the Dockerfile, which is /tmp/go/src and takes the following steps:

* First, the directory with the source code cloned from the git repository, that the S2I process copies to /tmp/src, is moved and renamed to app-src, which actually is /tmp/go/src/app-src
```shell
mv /tmp/src app-src
```
* Next the the working directory is changed to app-src
```shell
pushd app-src
```
* Then the packages required by the source code, if any, are downloaded and stored where the go tools can access them, $GOPATH/src.
```shell
go get -u -d -v ./...
```

* Next the application source code is built in verbose mode, and the resulting executable file called gobinary is placed at /tmp/go/bin/gobinary.  
```shell
go build -v -x -o ${APPROOT}/gobinary
```
* Finally the working directory is changed back to what it was before

```shell
popd 
```

#### run
This script will be used to run the go application and will be set as the CMD for the resulting application image.  To make sure that the system signals are correctly propagated to the container, the application should be started using the __exec__ command.

```shell
exec ${APPROOT}/gobinary
```
#### usage (optional) 
This script will print out instructions on how to use the image.
The _usage_ script is not required, but as the builder image itself is not intended to be used as an standalone container, it is convenient to set the usage script as the default command for this image so that any attempt to run a pod based on the builder image will send a message to stardard output, and the pod will enter a state of CrashLoopBackOff.

## Getting the builder image ready for use
To go from the Dockerfile and S2I scripts described above, to a builder image that can be used to transform go source code into an application image ready to run in Openshift, the following steps need to be taken:

* Make the builder image
* Push the builder image to a container registry
* Create an image stream inside an Openshift project referencing the builder image

Each one of these steps has different alternatives to be executed as will be explained in the following sections.

### Making the builder image
To make the builder image, the Dockerfile must be processed.  Two options will be shown here to do this:
* Running podman or docker in a host
* Running  `oc new-app` in an Openshift cluster

#### Using podman or docker
To create the builder image using podman or docker the requirements are: 
* Having podman or docker installed in the host
```shell
sudo yum install -y podman 
```
* Cloning the git repository containing the Dockerfile and s2i scripts:
```shell
git clone https://github.com/tale-toul/gobuilder-s2i.git
```

Enter the directory where the Dockerfile is located and run the build command.  Both docker and podman use the same options so it doesn't matter which one is used.  In the case of using docker, the docker daemon needs to be running, in the case of podman, the commands must be run as the root user:

```shell
cd gobuilder-s2i/go_builder_image/
sudo podman build -t gobuilder .
```
An image with name _localhost/gobuilder_ with a tag of _latest_ is added to the local image cache:

```shell
sudo podman images
REPOSITORY                            TAG      IMAGE ID       CREATED          SIZE
localhost/gobuilder                   latest   e583efd6a524   20 minutes ago   671 MB
```
This image is not accesible from Openshift, and therefore cannot be used as a builder image yet, it needs to be pushed to a registry.  More on this later.

#### Running 'oc new-app' in an Openshift cluster
S2I supports the creation of container images from a _Dockerfile_ stored in a git repository, therefore it is possible to use the `oc new-app` command to create the builder image directly from Openshift.  The user running these commands does not require any special permissions in the cluster.

The advantage of this method is that, after the creation of the builder image, it will be pushed to the internal Openshift registry, and the image stream will be created in the current project.  

When the `oc new-app` command completes the builder image will be ready to be used from the project where it was created.  If the builder image is to be used from other projects an additional configuration step is required.

The disadvantages of this method are:

* The `oc new-app` command tries to deploy a container based on the image just created, but since this image is not intended to be run standalone, the deployment goes into a _CrashLoopBackOff_ state and the deployment needs to be manually removed or scaled down to zero.  
* A kubernetes service is created in the namespace, but it is useless because of the situation described in the previous point, so it needs to be removed as well.
* Every time a new build is created, for example with the command __oc start-build__, all the steps in the Dockerfile are executed, unlike with podman and docker, no image layers are cached for possible reuse in subsequent builds.  This results in longer build times.

To run the following commands it is assumed that the user has an active session in an Openshift cluster.  Create the project and run the `oc new-app` command using the URL of the git repository and directory (__context-dir__) where the Dockerfile is stored:

```
oc new-project simplebuildergo
oc new-app --name gobuilder https://github.com/tale-toul/gobuilder-s2i --context-dir go_builder_image
```
In case the project is not in the default branch, the new branch can be specified with a command like:

```
oc new-app --name gobuilder https://github.com/user/repo/code#branch
```

The `oc new-app` command will return after a few seconds, but the build process will take a few minutes to complete.  To follow the buil process run:

```shell
oc logs -f bc/gobuilder
```
When the build process finishes with the message `Push successful` a deployment is triggered, but as mentioned earlier, this will eventually fail.  

```shell
oc get pods
NAME                         READY   STATUS             RESTARTS   AGE
gobuilder-1-build            0/1     Completed          0          6m48s
gobuilder-7bf7dbf776-w7f2r   0/1     CrashLoopBackOff   3          2m21s
```
Since this deployment and the service created by `oc new-app` are of no use, they can be removed from the project. 

```shell
$ oc status
In project simplebuildergo on server https://api.cartapacio.lab.pnq2.cee.redhat.com:6443

svc/gobuilder - 172.30.116.41:8080
  deployment/gobuilder deploys istag/gobuilder:latest <-
    bc/gobuilder docker builds https://github.com/tale-toul/gobuilder-s2i on istag/ubi8:8.3 
    deployment #2 running for 3 minutes - 0/1 pods (warning: 4 restarts)
    deployment #1 deployed 8 minutes ago

Errors:
  * pod/gobuilder-7bf7dbf776-w7f2r is crash-looping

$ oc delete deployment gobuilder
deployment.apps "gobuilder" deleted

$ oc delete service gobuilder
service "gobuilder" deleted
```

As a result of the previous `oc new-app` command, the builder image has been created, pushed to the Openshift internal registry and referenced by an image stream in the current project.

```shell
$ oc get imagestream gobuilder
NAME        IMAGE REPOSITORY                                                                                           TAGS     UPDATED
gobuilder   default-route-openshift-image-registry.apps.cartapacio.lab.pnq2.cee.redhat.com/simplebuildergo/gobuilder   latest   12 minutes ago
```
### Pushing the builder image to a container registry
If the builder image needs to be available from an external registry so it can be used from any namespace or even from other Openshift cluster, it has to be pushed to that external registry. This is always required if the image was built using podman or docker.

In the following example the registry is assumed to be private and needs authentication, both for pushing and pulling images.  

If the image was built using podman or docker two different ways to push the image will be shown: using podmand (docker); and using skopeo.  

#### Using podman or docker
The options for podman and docker are the same, however docker requires the docker daemon to be running, and podman requires the commands to be run as the root user:

The following commands must be executed in a host where the image is present in the images local cache.

Tag the local image with the name of the registry and user.  In this case the registry is _quay.io_ and the user is _milponta_:

```shell
sudo podman tag localhost/gobuilder quay.io/milponta/gobuilder
```
Log in to the remote registry, assuming only logged in users can push images to the registry.  

```shell
sudo podman login -u milponta quay.io
```

Push the image to the registry:
```shell
sudo podman push quay.io/milponta/gobuilder
```

#### Using skopeo
The _skopeo_ tool can be used to push images to a registry, the advantage of this command is that with a single command it is possible to do the tagging, log in to the registry, and pushing the image:

```shell
sudo skopeo copy containers-storage:localhost/gobuilder docker://quay.io/milponta/gobuilder --dest-creds milponta:SuperSecretPass
```
If the registry server is using x509 certificates not know by the local system, the following error message will appear and the image is not pushed:

```
x509: certificate signed by unknown authority`
```
In this case the option __--tls-verify=false__ can be used:

```shell
sudo skopeo copy containers-storage:localhost/gobuilder docker://quay.io/milponta/gobuilder --dest-creds milponta:SuperSecretPass --dest-tls-verify=false
```

### Creating the image stream in Openshift
If the image was not build using `oc new-app` and was pushed to an external registry, an image stream needs to be created.  The reason for this is that the S2I build process that creates the go application image always takes the builder image from an image stream.  

If the image registry containing the builder image needs authentication to pull images, the first step is to create a secret containing the credentials to access the registry.  If the registry does not required authenticatino to pull images this step can be skipped.

```shell
oc create secret docker-registry extregistry --docker-server quay.io --docker-username milponta --docker-password SuperSecretPass
```

The image stream can be created in the same project where the application is to be deployed or it can be created in a common or shared project where other projects can also use it.

#### On the application project
In case the go application and the image stream will reside in the same project, create the image stream with the following command.  If the registry requires credentials, the command will look for a secret valid for the remote registry name:

```shell
$ oc import-image gobuilder --confirm --from quay.io/milponta/gobuilder
```
If the command returns an error about invalid x509 certificates, add the option `--tls_verify=false`

Only if the external registry requires credentials to pull images, the _builder_ service account needs to have explicit access to the secret that was created before:

```shell
$ oc secrets link builder extregistry --for pull,mount
```

#### On a common project
If the image stream is created on a common project, from which other projects can use it, the __oc import-image__ command must include the option _--reference-policy=local_.  With this option there is no need to share the secret or access credentials to the external registry with other projects, if credentials are required to pull images from the registry.  

If the registry requires credentials to pull images, the command will look for a secret valid for the remote registry name.  Create this secret as explained in the previous section:

```shell
oc import-image gobuilder --confirm --from quay.io/milponta/gobuilder --reference-policy=local
```
Assuming that no applications will be deployed in this common project, there is no need to link the _extregistry_ secret with the builder service account.

Service accounts in the projects where applications will be deployed using the _gobuilder_ image stream need access to the _extregistry_ secret, for that the role _system:image-puller_ is used.  For example if the project _simpleweb_ is where the application will be deployed, the command to grant access to that project's service accounts to use the _gobuilder_ image stream is:

```shell
oc policy add-role-to-group system:image-puller system:serviceaccounts:simpleweb -n simplebuildergo
```
The above command grants permissions to pull images through the image stream in the project simplebuidergo, where the image stream was created, to all the service accounts in project simpleweb, where the application will be deployed.

## Creating the application image
The S2I process creates an application image that combines the builder image with your applications source code, compiled using the *assemble* script, and run using the *run* script.

The creation of the application image and its deployment can be acomplish with a single `oc new-app`:

If the image stream and the application are going to coexist in the same project:
```shell
oc new-app --name simpleweb gobuilder~https://github.com/tale-toul/gobuilder-s2i
```
The `--name simpleweb` is used to assign a label `app=simpleweb` to the resources created by `oc new-app`

The builder image is specified by prefixing the git repository URL with `gobuilder~`

If the image stream was created on a common project:

```shell
oc new-app --name simpleweb common/gobuilder~https://github.com/tale-toul/gobuilder-s2i
```
The name of project where the image stream was created is prefixed to the name of the image stream.

## Final consideration
* The `oc new-app` command creates a build config that will combine the builder image with the source code, compile the source code and create the application image.
* The `oc new-app` command creates a deployment that will run the resulting application container based on the image created by the build config.
* The `oc new-app` command creates a service resource to provide access to the application service running in the contianer.  The port where the service is listening for connections is obtained from the image definition, in particular from the EXPOSE directive and the label _io.openshift.expose-services_:

```
...
 LABEL io.openshift.expose-services="8080:http" \
...
EXPOSE 8080
```
