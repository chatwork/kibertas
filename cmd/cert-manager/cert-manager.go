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
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

type CertManager struct {
	*cmd.Checker
	CertName string
	Client   client.Client
}

func NewCertManager(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) *CertManager {
	t := time.Now()

	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	logger().Infof("cert-manager check application namespace: %s", namespace)
	chatwork.AddMessage(fmt.Sprintf("cert-manager check application namespace: %s\n", namespace))

	certName := "sample"

	if v := os.Getenv("CERT_NAME"); v != "" {
		certName = v
	}
	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	return &CertManager{
		Checker:  cmd.NewChecker(namespace, config.NewK8sClientset(), debug, logger, chatwork),
		CertName: certName,
		Client:   config.NewK8sClient(client.Options{Scheme: scheme}),
	}
}

func (c *CertManager) Check() error {
	c.Chatwork.AddMessage("cert-manager check start\n")
	defer c.Chatwork.Send()

	if err := c.createResources(); err != nil {
		return err
	}

	c.Chatwork.AddMessage("cert-manager check finished\n")
	return nil
}

func (c *CertManager) createResources() error {
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Debug, c.Logger)

	if err := k.CreateNamespace(&apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.Namespace,
		}}); err != nil {
		c.Logger().Error("Error create namespace:", err)
		c.Chatwork.AddMessage(fmt.Sprint("Error create namespace:", err))
		return err
	}
	defer func() {
		c.Chatwork.AddMessage(fmt.Sprintf("Delete Namespace: %s\n", c.Namespace))
		if err := k.DeleteNamespace(); err != nil {
			c.Chatwork.AddMessage(fmt.Sprint("Error Delete namespace:", err))
		}
	}()

	if err := c.createCert(); err != nil {
		c.Logger().Error("Error create certificate:", err)
		c.Chatwork.AddMessage(fmt.Sprint("Error create certificate:", err))
		return err
	}
	return nil
}

// createCert creates a certificate with cert-manager
// CRなので、client-goではなく、client-runtimeを使う
// ここでしか作らないリソースなので、utilのほうには入れない
func (c *CertManager) createCert() error {
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

	//Create RootCA
	c.Logger().Infoln("Create RootCA:", caName)
	c.Chatwork.AddMessage(fmt.Sprintf("Create RootCA: %s\n", caName))
	err := c.Client.Create(context.Background(), rootCA)
	if err != nil {
		return err
	}
	if !c.Debug {
		defer func() {
			c.Logger().Infoln("Delete RootCA:", caName)
			c.Chatwork.AddMessage(fmt.Sprintf("Delete RootCA: %s\n", caName))
			if err := c.Client.Delete(context.Background(), rootCA); err != nil {
				c.Logger().Error("Error delete RootCA:", err)
				c.Chatwork.AddMessage(fmt.Sprintf("Error delete RootCA: %s\n", err))
			}
		}()
	}

	secretClient := c.Clientset.CoreV1().Secrets(c.Namespace)

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, caSecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Errorf("Waiting for secret %s to be ready", caSecretName)
			return false, nil
		}
		c.Logger().Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})
	if err != nil {
		c.Logger().Error("Timed out waiting for RootCA secret to be ready:", err)
		c.Chatwork.AddMessage(fmt.Sprintf("Timed out waiting for RootCA secret to be ready: %s", err))
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
	c.Chatwork.AddMessage(fmt.Sprintf("Create Issuer: %s\n", issuerName))
	err = c.Client.Create(context.Background(), issuer)
	if err != nil {
		return err
	}

	if !c.Debug {
		defer func() {
			c.Logger().Infoln("Delete Issuer:", issuerName)
			c.Chatwork.AddMessage(fmt.Sprintf("Delete Issuer: %s\n", issuerName))
			if err := c.Client.Delete(context.Background(), issuer); err != nil {
				c.Logger().Error("Error delete Issuer:", err)
				c.Chatwork.AddMessage(fmt.Sprintf("Error delete Issuer: %s\n", err))
			}
		}()
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
	c.Chatwork.AddMessage(fmt.Sprintf("Create Certificate: %s\n", certificateName))
	err = c.Client.Create(context.Background(), certificate)

	if !c.Debug {
		defer func() {
			c.Logger().Infoln("Delete Certificate:", certificateName)
			c.Chatwork.AddMessage(fmt.Sprintf("Delete Certificate: %s\n", certificateName))
			if err := c.Client.Delete(context.Background(), certificate); err != nil {
				c.Logger().Error("Error delete Certificate:", err)
				c.Chatwork.AddMessage(fmt.Sprintf("Error delete Certificate: %s\n", err))
			}
		}()
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
		c.Logger().Error("Timed out waiting for Certificate secret to be ready:", err)
		c.Chatwork.AddMessage(fmt.Sprintf("Timed out waiting for Certificate secret to be ready: %s", err))
		return err
	}

	return nil
}
