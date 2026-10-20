# k8s-tmpns-operator
A learning project focused on building a Kubernetes operator using Kubebuilder and controller-runtime. The operator manages temporary namespaces with configurable expiration times, automatically provisions namespace-scoped RBAC for resource owners, reconciles cluster state, and cleans up expired resources.

## Overview

`TempNamespace` is a cluster-scoped custom resource (`tmpns.io/v1alpha1`). For each one, the operator:

- Creates a Namespace with the same name as the TempNamespace and labels it `tmpns.io/managed=true`.
- Creates a RoleBinding named `<name>-admin` in that Namespace, giving the resource's owner the built-in admin ClusterRole there.
- Reports progress in the resource's status: a `Ready` condition, and `status.namespace` with the name of the Namespace it created.
- Deletes the TempNamespace once `spec.expiresAt` has passed.

The Namespace and the RoleBinding both carry a controller reference to the TempNamespace, so Kubernetes garbage collection removes them when the TempNamespace is deleted, whether it expired or someone deleted it by hand.

A kubernetes [Validating Admission Policy](./config/admission/tempnamespace-owner-policy.yaml) prevents anyone but the owner (excluding cluster admins and the controller service account) from editing a TempNamespace custom resource.

### Namespace Naming

The operator never adopts a Namespace it did not create. If a Namespace with that name already exists and is not controlled by the TempNamespace (for example `kube-system`), reconciliation fails, the Namespace is left untouched, and the TempNamespace reports `Ready=False` with reason `NamespaceCreateOrUpdateFailed`.

### Spec

| Field | Set by | Meaning |
|---|---|---|
|`metadata.name` | you | Name of the custom resource, and name of the namespace TempNamespace will create*.
| `spec.duration` | you | How long the namespace should live: a whole number followed by a unit, for example `4h` or `7d`. Units: `h` (hours), `d` (days), `w` (weeks), `m` (months). Read at admission time and not kept on the stored object. |
| `spec.owner` | admission webhook | The user who created the resource (`username`, `uid`). This user gets `admin` in the Namespace. |
| `spec.createdAt` | admission webhook | When the resource was created. |
| `spec.expiresAt` | admission webhook | When the resource expires and is deleted. |

#### Example

```yaml
# Create a TempNamespace that lives for two hours
apiVersion: tmpns.io/v1alpha1
kind: TempNamespace
metadata:
  name: demo # also becomes the name of the Namespace created
spec:
  duration: 2h
```

### Listing Resources

```sh
$ kubectl get tmpns
NAME                        OWNER   NAMESPACE                   READY   REASON        EXPIRES
demo-temp-namespace-user2   user2   demo-temp-namespace-user2   True    Provisioned   2026-10-20T02:13:30Z
```

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.30+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/k8s-tmpns-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/k8s-tmpns-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/k8s-tmpns-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/k8s-tmpns-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

