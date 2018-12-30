# Synchronization

## Purpose

- monitor activity in Service Central: new and updated services
- ensure a ConfigMap with the SSAM access level for a service exists
- where/when a service's Namespace exists, ensure it contains an appropriate ConfigMap containing metadata about the service
- monitor activity in Release Manager (Deployinator): new and updated releases
- where/when a service's Namespace exists, ensure it contains an appropriate ConfigMap containing metadata about its releases
- where/when a service's Namespace exists, ensure it contains a Kubecompute docker secret
- where/when a service's Namespace exists, ensure the namespace has the Kube2iam allowed roles annotation

## Approach

- each instance of Synchronization will poll, retrieving a list of all services from Service Central at regular intervals
- immediately process the list, creating/updating a ConfigMap containing the SSAM access level for the service appropriate for the environment
- immediately process the list, creating/updating ConfigMaps in Namespaces that exist as appropriate
- each instance of Synchronization will poll, retrieving a list of all known namespaces at regular intervals
- immediately process the list, creating/updating ConfigMaps in Namespaces that exist as appropriate
- immediately process the list, creating/updating a secret in the namespace with the Kubecompute docker secret
- immediately process the list, adding/updating the service namespace with the Kube2iam allowed roles annotation
