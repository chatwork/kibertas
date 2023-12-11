# kibertas

kibertas is a CLI tool for achieving end-to-end (E2E) testing of Kubernetes environments. However, it does not perform E2E testing of the Kubernetes core itself.

Specifically, at the moment, it supports E2E testing of the following tools necessary for Kubernetes operations:

- [cluster autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler)
- Ingress
    - [aws-load-balancer-controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.6/)
- [ExternalDNS](https://github.com/kubernetes-sigs/external-dns)
- [cert-manager](https://cert-manager.io/)
- [datadog-agent](https://github.com/DataDog/datadog-agent)
- [fluentd](https://github.com/fluent/fluentd)
  - with S3 Output

For E2E testing of Kubernetes itself, please refer to the `certified-conformance` of [SONOBUOY](https://sonobuoy.io/) to verify the functionality that a cluster should meet.

Also, this tool is not a plugin for SONOBUOY, but a original implementation.

## Why not use SONOBUOY Plugin?

- Considered SONOBUOY plugin, but it was larger in terms of how to run SONOBUOY itself and how to write plugin.yaml than the simple E2E testing desired by us.
- Because we wanted to create it.☺️

# How to test locally

You need to start [kind](https://github.com/kubernetes-sigs/kind) in advance and apply ingress-nginx and cert-manager.

```
$ kind create cluster
$ make apply-cert-manager
$ make apply-ingress-nginx
```

At the moment, not all target tests have been implemented.

# How to execute locallity

You need build command.

```
$ make build
```

and you can execute `kibertas`, example for cert-manager.

```
$ ./dist/kibertas test cert-manager
```

You can coniform this tools options with help command.

```
$ ./dist/kibertas test help
```