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

You need to install [kind](https://github.com/kubernetes-sigs/kind) and [cloud-provider-kind](https://github.com/kubernetes-sigs/cloud-provider-kind)in advance.

```
$ make cloud-provider-kind
```

When executing `cloud-provider-kind` in test on a Mac, you are prompted to run `sudo`.

At the moment, not all target tests have been implemented.

# How to execute locallity

Grab the latest binary from [our GitHub releases page](https://github.com/chatwork/kibertas/releases).

Otherwise, you can clone this repository and build it yourself using `make`:

```
$ make build
```

Ensure that you have acess to the cluster, by setting `KUBECONFIG` envvar or properly configuring the default kubeconfig.

Now, run `kibertas`.

`kubertas` has sub-commands for respective test targets- For example, to test that the `cert-manager` on your cluster is working, run:

```
$ ./dist/kibertas test cert-manager
```

For the complete list of available test targets and the options, run:

```
$ ./dist/kibertas test help
```

# How to test kibertas

All the steps above have been for introducing how to use kibertas to test your apps and infrastructures.

How can we test kibertas itself?

It's a two-step process:

- Set environment varibles
- Run `go test`

Some tests require access to external services like Datadog.

You need to set corresponding environment variables before running the tests depending
what you want to test.

In case you want to run all the tests, ensure you have all the environment variables shown below set:

```
DD_API_KEY=...
DD_APP_KEY=...

AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
# Otherwise provide appropriate aws config, instance profile, or IAM role for SA

VPC_ID=...
```

> **VPC requirements:**
>
> The AWS VPC denoted by `VPC_ID` should have an internet gateway attached to it, and the primary route table should have a route to the internet gateway for the cidr block `0.0.0.0/0`.
>
> The VPC does not need to have subnets, as the tests will create them.

Now run everything:

```
go test ./...
```

Or run only a subset of the tests:

```
go test ./cmd/datadog-agent
```

Or even specify the targets via a regexp:

```
go test ./cmd/datadog-agent -test.run TestNewDatadogAgent
```
