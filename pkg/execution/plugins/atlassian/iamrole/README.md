# IAM Role plugin for Smith

This allows ServiceBindings to have IamPolicySnippet in their output, and
to wire all of these up to a single plugin which generates the role. It
attempts to merge policies (e.g. multiple SQS policies with the same set
of actions) in order to stay under the ~10kb limit for policies attached
to a role.

The plugin assumes that osb-aws-provider is installed into the cluster
so it can generate an appropriate ServiceInstance. The IAMRole output
from binding against this resource is the name of the generated role.
