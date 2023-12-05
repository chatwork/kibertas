package certmanager

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/hashicorp/go-multierror"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/k8s"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

type CertManager struct {
	*cmd.Checker
	CertName string
	Client   client.Client
}

type certificates struct {
	rootCA      *cmapiv1.Certificate
	issuer      *cmapiv1.Issuer
	certificate *cmapiv1.Certificate
}

func NewCertManager(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) (*CertManager, error) {
	t := time.Now()

	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	logger().Infof("cert-manager check application namespace: %s", namespace)
	chatwork.AddMessage(fmt.Sprintf("cert-manager check application namespace: %s\n", namespace))

	certName := "sample"
	timeout := 20

	if v := os.Getenv("CERT_NAME"); v != "" {
		certName = v
	}
	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	var err error
	if v := os.Getenv("CHECK_TIMEOUT"); v != "" {
		timeout, err = strconv.Atoi(v)
		if err != nil {
			logger().Errorf("strconv.Atoi: %s", err)
			return nil, err
		}
	}

	k8sclientset, err := config.NewK8sClientset()
	if err != nil {
		logger().Errorf("NewK8sClientset: %s", err)
		return nil, err
	}

	k8sclient, err := config.NewK8sClient(client.Options{Scheme: scheme})
	if err != nil {
		logger().Errorf("NewK8sClient: %s", err)
		return nil, err
	}

	return &CertManager{
		Checker:  cmd.NewChecker(namespace, k8sclientset, debug, logger, chatwork, time.Duration(timeout)*time.Minute),
		CertName: certName,
		Client:   k8sclient,
	}, nil
}

func (c *CertManager) Check(ctx context.Context) error {
	cert := c.createCertificateObject()

	c.Chatwork.AddMessage("cert-manager check start\n")
	defer c.Chatwork.Send()

	defer func() {
		if err := c.cleanUpResources(cert); err != nil {
			c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Resources: %s\n", err))
		}
	}()
	if err := c.createResources(ctx, cert); err != nil {
		return err
	}

	c.Chatwork.AddMessage("cert-manager check finished\n")
	return nil
}

func (c *CertManager) createResources(ctx context.Context, cert certificates) error {
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Logger)

	if err := k.CreateNamespace(
		ctx,
		&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.Namespace,
			}}); err != nil {
		c.Logger().Error("Error create namespace:", err)
		c.Chatwork.AddMessage(fmt.Sprint("Error create namespace:", err))
		return err
	}

	if err := c.createCert(ctx, cert); err != nil {
		c.Logger().Error("Error create certificate:", err)
		c.Chatwork.AddMessage(fmt.Sprint("Error create certificate:", err))
		return err
	}
	return nil
}

// cleanUpResources deletes the certificate, issuer, rootCA, and namespace associated with the given certificate.
// It returns an error if any deletion operation fails.
func (c *CertManager) cleanUpResources(cert certificates) error {
	if c.Debug {
		c.Logger().Info("Skip Delete Resources")
		c.Chatwork.AddMessage("Skip Delete Resources\n")
		return nil
	}
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Logger)
	var result *multierror.Error
	var err error

	c.Logger().Error("Delete Certificate:", cert.certificate.ObjectMeta.Name)
	if err := c.Client.Delete(context.Background(), cert.certificate); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Certificate: %s\n", err))
		c.Logger().Error("Error Delete Certificate:", err)
		result = multierror.Append(result, err)
	}

	c.Logger().Error("Delete Issuer:", cert.certificate.ObjectMeta.Name)
	if err := c.Client.Delete(context.Background(), cert.issuer); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Issuer: %s\n", err))
		c.Logger().Error("Error Delete Issuer:", err)
		result = multierror.Append(result, err)
	}

	c.Logger().Error("Delete RootCA:", cert.certificate.ObjectMeta.Name)
	if err := c.Client.Delete(context.Background(), cert.rootCA); err != nil {
		c.Logger().Error("Error Delete RootCA:", err)
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete RootCA: %s\n", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteNamespace(); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Namespace: %s\n", err))
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (c *CertManager) createCertificateObject() certificates {
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
	return certificates{rootCA: rootCA, issuer: issuer, certificate: certificate}
}

// createCert creates a certificate with cert-manager
// CRなので、client-goではなく、client-runtimeを使う
// ここでしか作らないリソースなので、utilのほうには入れない
func (c *CertManager) createCert(ctx context.Context, cert certificates) error {
	c.Logger().Infoln("Create RootCA:", cert.rootCA.ObjectMeta.Name)
	c.Chatwork.AddMessage(fmt.Sprintf("Create RootCA: %s\n", cert.rootCA.ObjectMeta.Name))
	err := c.Client.Create(ctx, cert.rootCA)
	if err != nil {
		return err
	}

	secretClient := c.Clientset.CoreV1().Secrets(c.Namespace)

	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, c.Timeout, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, cert.rootCA.Spec.SecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Errorf("Waiting for secret %s to be ready", cert.rootCA.Spec.SecretName)
			return false, nil
		}
		c.Logger().Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})

	if err != nil {
		if err.Error() == "context canceled" {
			c.Logger().Error("Context canceled waiting for RootCA secret to be ready")
			c.Chatwork.AddMessage("Context canceled waiting for RootCA secret to be ready")
		} else if err.Error() == "context deadline exceeded" {
			c.Logger().Error("Timed out waiting for RootCA secret to be ready")
			c.Chatwork.AddMessage("Timed out waiting for RootCA secret to be ready")
		} else {
			c.Logger().Error("Error waiting for RootCA secret to be ready:", err)
			c.Chatwork.AddMessage(fmt.Sprintf("Error waiting for RootCA secret to be ready: %s\n", err))
		}
		return err
	}

	//Create Issuer
	c.Logger().Infoln("Create Issuer:", cert.issuer.ObjectMeta.Name)
	c.Chatwork.AddMessage(fmt.Sprintf("Create Issuer: %s\n", cert.issuer.ObjectMeta.Name))
	err = c.Client.Create(ctx, cert.issuer)
	if err != nil {
		return err
	}

	c.Logger().Infoln("Create Certificate:", cert.certificate.ObjectMeta.Name)
	c.Chatwork.AddMessage(fmt.Sprintf("Create Certificate: %s\n", cert.certificate.ObjectMeta.Name))
	err = c.Client.Create(ctx, cert.certificate)

	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, c.Timeout, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, cert.certificate.Spec.SecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Errorf("Waiting for secret %s to be ready\n", cert.certificate.Spec.SecretName)
			return false, nil
		}
		c.Logger().Infof("Created secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})

	if err != nil {
		if err.Error() == "context canceled" {
			c.Logger().Error("Context canceled waiting for Certificate secret to be ready")
			c.Chatwork.AddMessage("Context canceled waiting for Certificate secret to be ready")
		} else if err.Error() == "context deadline exceeded" {
			c.Logger().Error("Timed out waiting for Certificate secret to be ready")
			c.Chatwork.AddMessage("Timed out waiting for Certificate secret to be ready")
		} else {
			c.Logger().Error("Error waiting for Certificate secret to be ready:", err)
			c.Chatwork.AddMessage(fmt.Sprintf("Error waiting for Certificate secret to be ready: %s\n", err))
		}
		return err
	}

	return nil
}
