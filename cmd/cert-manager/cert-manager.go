package certmanager

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type CertManager struct {
	namespace string
	certName  string
	debug     bool
	logLevel  string
	client    client.Client
	clientset *kubernetes.Clientset
}

func NewCertManager(debug bool, logLevel string) *CertManager {
	t := time.Now()

	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	log.Printf("cert-manager check application namespace: %s\n", namespace)

	certName := "sample"

	if v := os.Getenv("CERT_NAME"); v != "" {
		certName = v
	}
	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	return &CertManager{
		namespace: namespace,
		certName:  certName,
		debug:     debug,
		logLevel:  logLevel,
		clientset: config.NewK8sClientset(),
		client:    config.NewK8sClient(client.Options{Scheme: scheme}),
	}
}

func (c *CertManager) Check() error {
	logr := logrus.New()
	logr.SetFormatter(&logrus.JSONFormatter{})
	level, err := logrus.ParseLevel(c.logLevel)

	if err != nil {
		return errors.Wrap(err, "invalid log level")
	}

	logr.SetLevel(level)

	err = k8s.CreateNamespace(c.namespace, c.clientset)
	if err != nil {
		return err
	}

	if c.debug {
		logr.Infof("Preserve resources in %s", c.namespace)
	} else {
		defer k8s.DeleteNamespace(c.namespace, c.clientset)
	}

	caName := c.certName + "-ca"
	caSecretName := c.certName + "-tls"
	issuerName := c.certName + "-issuer"
	certificateName := c.certName + "-cert"
	certificateSecretName := certificateName

	rootCA := &cmapiv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caName,
			Namespace: c.namespace,
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
	logr.Infoln("Create RootCA:", caName)
	err = c.client.Create(context.Background(), rootCA)
	if err != nil {
		return err
	}
	if !c.debug {
		defer c.client.Delete(context.Background(), rootCA)
	}

	secretClient := c.clientset.CoreV1().Secrets(c.namespace)

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, caSecretName, metav1.GetOptions{})
		if err != nil {
			logr.WithError(err).Errorf("Waiting for secret %s to be ready\n", caSecretName)
			return false, nil
		}
		logr.Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})
	if err != nil {
		return err
	}

	//Create Issuer
	issuer := &cmapiv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuerName,
			Namespace: c.namespace,
		},
		Spec: cmapiv1.IssuerSpec{
			IssuerConfig: cmapiv1.IssuerConfig{
				CA: &cmapiv1.CAIssuer{
					SecretName: caSecretName,
				},
			},
		},
	}

	logr.Infoln("Create Issuer:", issuerName)
	err = c.client.Create(context.Background(), issuer)
	if err != nil {
		return err
	}

	if !c.debug {
		defer c.client.Delete(context.Background(), issuer)
	}

	//Create Certificate
	certificate := &cmapiv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName,
			Namespace: c.namespace,
		},
		Spec: cmapiv1.CertificateSpec{
			SecretName: certificateSecretName,
			IssuerRef: cmmeta.ObjectReference{
				Name: issuerName,
				Kind: "Issuer",
			},
			DNSNames: []string{
				c.certName,
				c.certName + "." + c.namespace + ".svc",
				c.certName + "." + c.namespace + ".svc.cluster.local",
			},
		},
	}

	logr.Infoln("Create Certificate:", certificateName)
	err = c.client.Create(context.Background(), certificate)
	if !c.debug {
		defer c.client.Delete(context.Background(), certificate)
	}

	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, certificateSecretName, metav1.GetOptions{})
		if err != nil {
			logr.WithError(err).Errorf("Waiting for secret %s to be ready\n", certificateSecretName)
			return false, nil
		}
		logr.Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})
	if err != nil {
		return err
	}

	return nil
}
