package certmanager

import (
	"context"
	"fmt"
	"os"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cw-sakamoto/kibertas/cmd"
	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	"github.com/sirupsen/logrus"
)

type CertManager struct {
	*cmd.Checker
	CertName string
	Client   client.Client
}

func NewCertManager(debug bool, logger func() *logrus.Entry) *CertManager {
	t := time.Now()

	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	logger().Infof("cert-manager check application namespace: %s", namespace)

	certName := "sample"

	if v := os.Getenv("CERT_NAME"); v != "" {
		certName = v
	}
	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	return &CertManager{
		Checker:  cmd.NewChecker(namespace, config.NewK8sClientset(), debug, logger),
		CertName: certName,
		Client:   config.NewK8sClient(client.Options{Scheme: scheme}),
	}
}

func (c *CertManager) Check() error {
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Debug, c.Logger)

	err := k.CreateNamespace(&apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.Namespace,
		}})
	if err != nil {
		return err
	}
	defer k.DeleteNamespace()

	caName := c.CertName + "-ca"
	caSecretName := c.CertName + "-tls"
	issuerName := c.CertName + "-issuer"
	certificateName := c.CertName + "-cert"
	certificateSecretName := certificateName

	rootCA := &cmapiv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caName,
			Namespace: c.Namespace,
		},
		Spec: cmapiv1.CertificateSpec{
			SecretName: caSecretName,
			CommonName: caSecretName,
			IsCA:       true,
			PrivateKey: &cmapiv1.CertificatePrivateKey{
				Algorithm: cmapiv1.ECDSAKeyAlgorithm,
				Size:      256,
			},
			IssuerRef: cmmeta.ObjectReference{
				Name:  "selfsigned-issuer",
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
		},
	}

	//Create CA
	c.Logger().Infoln("Create RootCA:", caName)
	err = c.Client.Create(context.Background(), rootCA)
	if err != nil {
		return err
	}
	if !c.Debug {
		defer c.Client.Delete(context.Background(), rootCA)
	}

	secretClient := c.Clientset.CoreV1().Secrets(c.Namespace)

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, caSecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Errorf("Waiting for secret %s to be ready\n", caSecretName)
			return false, nil
		}
		c.Logger().Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})
	if err != nil {
		return err
	}

	//Create Issuer
	issuer := &cmapiv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuerName,
			Namespace: c.Namespace,
		},
		Spec: cmapiv1.IssuerSpec{
			IssuerConfig: cmapiv1.IssuerConfig{
				CA: &cmapiv1.CAIssuer{
					SecretName: caSecretName,
				},
			},
		},
	}

	c.Logger().Infoln("Create Issuer:", issuerName)
	err = c.Client.Create(context.Background(), issuer)
	if err != nil {
		return err
	}

	if !c.Debug {
		defer c.Client.Delete(context.Background(), issuer)
	}

	//Create Certificate
	certificate := &cmapiv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName,
			Namespace: c.Namespace,
		},
		Spec: cmapiv1.CertificateSpec{
			SecretName: certificateSecretName,
			IssuerRef: cmmeta.ObjectReference{
				Name: issuerName,
				Kind: "Issuer",
			},
			DNSNames: []string{
				c.CertName,
				c.CertName + "." + c.Namespace + ".svc",
				c.CertName + "." + c.Namespace + ".svc.cluster.local",
			},
		},
	}

	c.Logger().Infoln("Create Certificate:", certificateName)
	err = c.Client.Create(context.Background(), certificate)
	if !c.Debug {
		defer c.Client.Delete(context.Background(), certificate)
	}

	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, certificateSecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Errorf("Waiting for secret %s to be ready\n", certificateSecretName)
			return false, nil
		}
		c.Logger().Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})
	if err != nil {
		return err
	}

	return nil
}
