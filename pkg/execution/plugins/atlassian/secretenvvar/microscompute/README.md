# Micros Secret Env Var Plugin for Smith

This looks at any ServiceBinding and explicit Secret dependencies,
converts them into a json object of 'environment variables' in
a form consumable by ParametersFrom (in Service Catalog).

If you squint, it looks like a bit like envFrom in pods, but
is considerably more magical.

The plugin applies filtering to secret keys after they have been prefixed, but
prior to any renaming.

e.g. a ServiceBinding produces a secret like so:

```
Secret:
  myVar: x
  filtered: willbe
```

And with a plugin spec of:

```
plugin:
  spec:
    outputSecretKey: envVarsForCompute
    outputJsonKey: secretEnvVars
    ignoreKeyRegex: PREFIX_FILTERED
```

we end up with:

```
Secret:
  envVarsForCompute: >
    {"secretEnvVars": {"PREFIX_MYVAR": "x"}}
```

For a slightly more real example, see the examples directory.

## How PREFIX is calculated

If you depend on a ServiceBinding, this is:

  RESOURCEPREFIX_SERVICEINSTANCENAME

Where `SERVICEINSTANCENAME` is the Kubernetes object name of the instance
(NOT the Smith bundle resource name). `RESOURCEPREFIX` is usually the
ClusterServiceClassExternalName (i.e. the 'type'), but this can be overrridden
by adding the annotation voyager.atl-paas.net/envResourcePrefix to
the ServiceBinding or ServiceInstance (binding is preferred over instance).

If you depend on a secret, RESOURCEPREFIX is the Kubernetes object name
of the Secret (NOT the Smith bundle resource name).

All dashes are converted to underscores, and everything is uppercased.

## Running

This is run by adding it as a Smith plugin. See `cmd/smith/main.go`,
or run:

    make run-smith-sc-minikube
