# Kubernetes Secret Env Var Plugin for Smith

This looks at any ServiceBinding and explicit Secret dependencies 
renames and filters them to match pre-existing naming conventions.

e.g. a ServiceBinding produces a secret like so:

```
Secret:
  myVar: x
  a: y
```

And with a plugin spec of:

```
plugin:
  spec:
    ignoreKeyRegex: "a"
```

we end up with:

```
Secret:
  myVar: x
```

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
