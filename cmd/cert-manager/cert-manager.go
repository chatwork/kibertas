package certmanager

import (
	"context"
	"fmt"
	"os"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/hashicorp/go-multierror"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/k8s"
)

type CertManager struct {
	*cmd.Checker
	Namespace    string
	ResourceName string
	Clientset    *kubernetes.Clientset
	Client       client.Client
}

type certificates struct {
	rootCA      *cmapiv1.Certificate
	issuer      *cmapiv1.Issuer
	certificate *cmapiv1.Certificate
}

func NewCertManager(checker *cmd.Checker) (*CertManager, error) {
	t := time.Now()

	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	checker.Logger().Infof("cert-manager check application Namespace: %s", namespace)
	checker.Chatwork.AddMessage(fmt.Sprintf("cert-manager check application Namespace: %s\n", namespace))

	resourceName := "sample"

	if v := os.Getenv("RESOURCE_NAME"); v != "" {
		resourceName = v
	}

	k8sclientset, err := config.NewK8sClientset()
	if err != nil {
		checker.Logger().Errorf("Error NewK8sClientset: %s ", err)
	}

	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	k8sclient, err := config.NewK8sClient(client.Options{Scheme: scheme})
	if err != nil {
		checker.Logger().Errorf("NewK8sClient: %s", err)
		return nil, err
	}

	return &CertManager{
		Checker:      checker,
		Namespace:    namespace,
		ResourceName: resourceName,
		Clientset:    k8sclientset,
		Client:       k8sclient,
	}, nil
}

func (c *CertManager) Check() error {
	cert := c.createCertificateObject()

	c.Chatwork.AddMessage("cert-manager check start\n")
	defer c.Chatwork.Send()

	defer func() {
		if err := c.cleanUpResources(cert); err != nil {
			c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Resources: %s\n", err))
		}
	}()
	if err := c.createResources(cert); err != nil {
		return err
	}

	c.Chatwork.AddMessage("cert-manager check finished\n")
	return nil
}

func (c *CertManager) createResources(cert certificates) error {
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Logger)

	if err := k.CreateNamespace(
		c.Ctx,
		&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.Namespace,
			}}); err != nil {
		c.Logger().Error("Error create Namespace:", err)
		c.Chatwork.AddMessage(fmt.Sprint("Error create Namespace:", err))
		return err
	}

	if err := c.createCert(cert); err != nil {
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

	c.Logger().Infof("Delete Certificate: %s", cert.certificate.ObjectMeta.Name)
	if err := c.Client.Delete(context.Background(), cert.certificate); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Certificate: %s\n", err))
		c.Logger().Errorf("Error Delete Certificate: %s", err)
		result = multierror.Append(result, err)
	}

	c.Logger().Infof("Delete Issuer: %s", cert.certificate.ObjectMeta.Name)
	if err := c.Client.Delete(context.Background(), cert.issuer); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Issuer: %s\n", err))
		c.Logger().Errorf("Error Delete Issuer: %s", err)
		result = multierror.Append(result, err)
	}

	c.Logger().Infof("Delete RootCA: %s", cert.certificate.ObjectMeta.Name)
	if err := c.Client.Delete(context.Background(), cert.rootCA); err != nil {
		c.Logger().Errorf("Error Delete RootCA: %s", err)
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
	caName := c.ResourceName + "-ca"
	caSecretName := c.ResourceName + "-tls"
	issuerName := c.ResourceName + "-issuer"
	certificateName := c.ResourceName + "-cert"
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
				c.ResourceName,
				c.ResourceName + "." + c.Namespace + ".svc",
				c.ResourceName + "." + c.Namespace + ".svc.cluster.local",
			},
		},
	}
	return certificates{rootCA: rootCA, issuer: issuer, certificate: certificate}
}

// createCert creates a certificate with cert-manager
// CRなので、client-goではなく、client-runtimeを使う
// ここでしか作らないリソースなので、utilのほうには入れない
func (c *CertManager) createCert(cert certificates) error {
	c.Logger().Infof("Create RootCA: %s", cert.rootCA.ObjectMeta.Name)
	c.Chatwork.AddMessage(fmt.Sprintf("Create RootCA: %s\n", cert.rootCA.ObjectMeta.Name))
	err := c.Client.Create(c.Ctx, cert.rootCA)
	if err != nil {
		return err
	}

	secretClient := c.Clientset.CoreV1().Secrets(c.Namespace)

	err = wait.PollUntilContextTimeout(c.Ctx, 5*time.Second, c.Timeout, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, cert.rootCA.Spec.SecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Infof("Waiting for Secret %s to be ready", cert.rootCA.Spec.SecretName)
			return false, nil
		}
		c.Logger().Infof("Created Secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("waiting for RootCA secret to be ready: %w", err)
	}

	//Create Issuer
	c.Logger().Infof("Create Issuer: %s", cert.issuer.ObjectMeta.Name)
	c.Chatwork.AddMessage(fmt.Sprintf("Create Issuer: %s\n", cert.issuer.ObjectMeta.Name))
	err = c.Client.Create(c.Ctx, cert.issuer)
	if err != nil {
		return err
	}

	c.Logger().Infof("Create Certificate: %s", cert.certificate.ObjectMeta.Name)
	c.Chatwork.AddMessage(fmt.Sprintf("Create Certificate: %s\n", cert.certificate.ObjectMeta.Name))
	err = c.Client.Create(c.Ctx, cert.certificate)

	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(c.Ctx, 5*time.Second, c.Timeout, true, func(ctx context.Context) (bool, error) {
		secret, err := secretClient.Get(ctx, cert.certificate.Spec.SecretName, metav1.GetOptions{})
		if err != nil {
			c.Logger().WithError(err).Infof("Waiting for Secret %s to be ready", cert.certificate.Spec.SecretName)
			return false, nil
		}
		c.Logger().Infof("Created Secret:%s at %s", secret.Name, secret.CreationTimestamp)
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("waiting for Certificate Secret to be ready: %w", err)
	}

	return nil
}
