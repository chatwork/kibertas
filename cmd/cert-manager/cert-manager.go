package certmanager

import (
	"fmt"
	"log"
	"os"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type CertManager struct {
	namespace string
	certName  string
	client    client.Client
}

func NewCertManager() *CertManager {
	t := time.Now()

	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	log.Printf("cert-manager check application namespace: %s\n", namespace)

	certName := "sample"

	if v := os.Getenv("CERT_NAME"); v != "" {
		certName = v
	}

	return &CertManager{
		namespace: namespace,
		certName:  certName,
		client:    config.NewK8sClient(),
	}
}

func (c *CertManager) Check() error {
	k8s.CreateNamespace(c.namespace, c.client)
	defer k8s.DeleteNamespace(c.namespace, c.client)

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
			IssuerRef: cmapiv1.ObjectReference{
				Name:  "selfsigned-issuer",
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
		},
	}

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

	certificate := &cmapiv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificateName,
			Namespace: c.namespace,
		},
		Spec: cmapiv1.CertificateSpec{
			SecretName: certificateSecretName,
			IssuerRef: cmapiv1.ObjectReference{
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
	return nil
}
