The Kubernetes executor creates a new Kubernetes pod to run every job it receives. Usually, the agents themselves run inside the Kubernetes cluster, but you can also run Semaphore jobs in Kubernetes pods while the agent runs somewhere else. The Kubernetes executor can be enabled with the `--kubernetes-executor` agent configuration parameter.

- [Pod start timeout](#pod-start-timeout)
- [Specifying containers](#specifying-containers)
- [Decorating the Kubernetes pod configuration](#decorating-the-kubernetes-pod-configuration)
- [Configure the image pull policies](#configure-the-image-pull-policies)
- [Use environment variables](#use-environment-variables)
- [Use files](#use-files)
- [Requests and limits](#requests-and-limits)
- [Use volumes and volume mounts](#use-volumes-and-volume-mounts)
- [Use private images](#use-private-images)
  - [Semaphore secrets](#semaphore-secrets)
  - [Use a manually created Kubernetes secret](#use-a-manually-created-kubernetes-secret)
- [Restrict images](#restrict-images)

## Pod start timeout

The Kubernetes executor will wait for 300s for the pod to be ready to run the Semaphore job. If the pod doesn't come up in time, the Semaphore job will fail. That value can be configured with the Semaphore agent `--kubernetes-pod-start-timeout` parameter.

## Specifying containers

The Kubernetes executor requires the Semaphore YAML to specify the containers to use. If no containers are specified in the YAML, the job will fail. Here are the configurable fields in a container definition in the Semaphore YAML:

```yaml
containers:

  # The first container (main) is where the commands will run.
  # The only thing we need for that container is that it remains up,
  # so we can `kubectl exec` into it, create a PTY, and run the commands.
  # For that reason, we don't allow configuring the `entrypoint` and `command` fields of that container.
  - name: main
    image: ruby:2.7
    env_vars: []

  # For the additional containers, `entrypoint` and `command` can be configured as well.
  - name: db
    image: postgres:9.6
    command: ""
    entrypoint: ""
    env_vars: []
```

More information about how to specify containers in the Semaphore YAML in the [public docs](https://docs.semaphoreci.com/ci-cd-environment/custom-ci-cd-environment-with-docker/).

## Decorating the Kubernetes pod configuration

By default, all the information to create the pod comes from the Semaphore YAML. More specifically, from the containers specified in the Semaphore YAML. However, you might need to configure the pod and the containers in it further. You can do that with the `--kubernetes-pod-spec-decorator-from-config` parameter.

That parameter receives the name of a Kubernetes config map. The config map can have three different keys:

```yaml
apiVersion: core/v1
kind: ConfigMap
metadata:
  name: pod-spec-decorator-for-semaphore-jobs
stringData:
  mainContainer: ""
  sidecarContainers: ""
  pod: ""
```

Each of the keys decorate a specific part of the pod created for the Semaphore job, and receive a string containing a YAML document:
- The `mainContainer` key allows you to decorate the fields in the [Kubernetes container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#container-v1-core) where the job commands will execute.
- The `sidecarContainers` key allows you to decorate the fields in the [Kubernetes container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#container-v1-core) used as sidecars.
- The `pod` key allows you to decorate the fields in the [Kubernetes pod](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#pod-v1-core) created for the Semaphore job.

> **Note**<br/>
> The `--kubernetes-pod-spec-decorator-from-config` parameter does not override what comes from the Semaphore YAML. It only decorates it. If you want to reject jobs that use untrusted images, use the [--kubernetes-allowed-images](#configure-the-allowed-images) parameter.

## Configure the image pull policies

By default, no image pull policy is set on any of the containers in the pod. That means Kubernetes will use its default, which is `IfNotPresent`. You can use the `--kubernetes-pod-spec-decorator-from-config` parameter to specify them:

```yaml
kind: ConfigMap
metadata:
  name: pod-spec-decorator-for-semaphore-jobs
stringData:
  mainContainer: |
    imagePullPolicy: Never
  sidecarContainers: |
    imagePullPolicy: Never
```

## Use environment variables

You can use [Semaphore secrets](https://docs.semaphoreci.com/essentials/using-secrets/) or the Semaphore YAML's [env_vars](https://docs.semaphoreci.com/essentials/environment-variables/) to pass environment variables to your jobs, just like in cloud jobs.

If you want to provide additional environment variables configured on the agent side, you can use the `--kubernetes-pod-spec-decorator-from-config` agent configuration parameter:

```yaml
kind: ConfigMap
metadata:
  name: pod-spec-for-semaphore-job
stringData:
  # Add environment variables to the main container.
  # These will only be available in the container where the Semaphore job runs.
  mainContainer: |
    env:
      - name: FOO
        value: BAR

  # You can also add environment variables to the sidecar containers in the pod.
  # These will be added to all sidecar containers.
  sidecarContainers: |
    env:
      - name: FOO
        value: BAR
```

The environment variables specified with this approach will be appended to the ones specified in the Semaphore YAML (if any).

## Use files

You can use [Semaphore secrets](https://docs.semaphoreci.com/essentials/using-secrets/) to provide files to your job, just like in cloud jobs. Additionally, if you want to provide files to jobs from the agent side, you can use the `--kubernetes-pod-spec-decorator-from-config` agent parameter to decorate the pod spec:

```yaml
kind: ConfigMap
metadata:
  name: pod-spec-for-semaphore-job
stringData:
  mainContainer: |
    volumeMounts:
      - name: myfile
        mountPath: /app/files
  pod: |
    volumes:
      - name: myfile
        secret:
          name: {YOUR_SECRET_NAME}
          items:
            key: {SECRET_KEY_FOR_YOUR_FILE}
            path: {NAME_OF_THE_FILE_MOUNTED}
```

With the above, you'd be able to access your file in the `/app/files` directory.

## Requests and limits

Use the `--kubernetes-pod-spec-decorator-from-config` agent parameter:

```yaml
kind: ConfigMap
metadata:
  name: pod-spec-for-semaphore-job
stringData:
  mainContainer: |
    limits:
      cpu: "0.5"
      memory: 500Mi
    requests:
      cpu: "0.25"
      memory: 250Mi
  sidecarContainers: |
    limits:
      cpu: "0.1"
      memory: 100Mi
    requests:
      cpu: "0.1"
      memory: 100Mi
```

## Use volumes and volume mounts

See the [files](#use-files) section.

## Use private images

If the image being used to run the job is private, authentication is required to pull it.

### Semaphore secrets

You can create a Semaphore secret containing the credentials to authenticate to your registry, and use it in your Semaphore YAML's [image_pull_secrets](https://docs.semaphoreci.com/ci-cd-environment/custom-ci-cd-environment-with-docker/#pulling-private-docker-images-from-dockerhub). When using this appproach, the Kubernetes executor will create a temporary Kubernetes secret to store the credentials, and use it to pull the images. When the job finishes, the Kubernetes will be deleted.

> **Note**<br/>
> This is the only way to use ECR images, since ECR doesn't allow long-lived tokens for authentication.

### Use a manually created Kubernetes secret

You can also manually create a Kubernetes secret with your registry's credentials, and use the `--kubernetes-pod-spec-decorator-from-config` agent configuration parameter to use it:

```yaml
kind: ConfigMap
metadata:
  name: pod-spec-decorator-for-semaphore-jobs
stringData:
  .pod: |
    imagePullSecrets:
      - my-k8s-registry-secret
```

## Restrict images

By default, the Kubernetes executor accepts all images specified in the Semaphore YAML. If you want to restrict the images used in the jobs executed by your agents, you can use the `--kubernetes-allowed-images`.

That parameter takes a list of regular expressions. If the image specified in the Semaphore YAML matches one of the expressions, it is allowed. For example, if you want to restrict jobs to only use images from a `custom-registry-1.com` registry, you can use `--kubernetes-allowed-images ^custom-registry-1\.com\/(.+)`
