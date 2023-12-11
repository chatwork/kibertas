# kibertas

kibertas Kubernetes E2E はKubernetes環境のE2Eを実現するための、CLIツールです。
ただし、Kubernetes本体のE2Eは行いません。

具体的には、現時点では、Kubernetesの運用に必要な下記のツールのE2Eに対応しています。

- [cluster autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler)
- Ingress
  - 主に[aws-load-balancer-controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.6/)
- [ExternalDNS](https://github.com/kubernetes-sigs/external-dns)
- [cert-manager](https://cert-manager.io/)
- [datadog-agent](https://github.com/DataDog/datadog-agent)

Kubernetes自体のE2Eは[SONOBUOY](https://sonobuoy.io/)のcertified-conformanceでクラスタとして満たすべき機能を確認してください。

また、本ツールはSONOBUOYのpluginではなく、独自の実装です。

## なぜ独自の実装にしたのか

- SONOBUOY pluginも検討したが、SONOBUOY自体の動かし方や、plugin.yamlの書き方などが、Chatworkで求める簡易的なE2Eより大柄だったため
- 作りたかったから

# ローカルでのテスト方法

[kind](https://github.com/kubernetes-sigs/kind)を予め起動し、ingress-nginx, cert-managerをapplyする必要があります。

```
$ kind create cluster
$ make apply-cert-manager
$ make apply-igress-nginx
```

各種起動したら、
```
$ make test
```
を実行してください。ただし、現時点ではすべての対象のテストが実現できていません。

# ローカルでの実行方法

CLIツールなので、ローカルからも実行可能です。ビルドしたら、helpコマンドで確認してください。

```
$ make build
$ ./dist/kibertas test -h
```

たとえば、対象のクラスタのcert-managerのテストを実行するには、

```
$ ./dist/kibertas test cert-manager
```

を実行してください。