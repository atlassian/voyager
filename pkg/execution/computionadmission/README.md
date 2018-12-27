# Compution

## Admission Controller

This is a [ValidatingAdmissionWebhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/),
that denies compute resources if they reference non-compliant (i.e. not under /sox/ ) artifacts when the service requires PRGB compliance.

## Controller (TODO)

This monitors compute resources,
and ensures that Service Central has a compute record for each compute resource.

## See Also

- https://hello.atlassian.net/wiki/spaces/VDEV/pages/325687047/ServiceCentralComputeRecord+Controller

- https://hello.atlassian.net/wiki/spaces/MDEV/pages/318660754/Sync+EC2+Compute+Provider+versus+Microscope

