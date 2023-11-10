package ingress

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cw-sakamoto/kibertas/cmd"
	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	"github.com/cw-sakamoto/kibertas/util/notify"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Ingress struct {
	*cmd.Checker
	ResourceName     string
	ExternalHostname string
}

func NewIngress(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) *Ingress {
	t := time.Now()

	namespace := fmt.Sprintf("ingress-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	logger().Infof("ingress check application namespace: %s", namespace)
	chatwork.AddMessage(fmt.Sprintf("ingress check application namespace: %s\n", namespace))

	resourceName := "sample"
	externalHostName := "sample-skmt.cwtest.info"

	if v := os.Getenv("RESOURCE_NAME"); v != "" {
		resourceName = v
	}
	if v := os.Getenv("EXTERNAL_HOSTNAME"); v != "" {
		externalHostName = v
	}

	return &Ingress{
		Checker:          cmd.NewChecker(namespace, config.NewK8sClientset(), debug, logger, chatwork),
		ResourceName:     resourceName,
		ExternalHostname: externalHostName,
	}
}

func (i *Ingress) Check() error {
	i.Chatwork.AddMessage("ingress check start\n")
	defer i.Chatwork.Send()
	k := k8s.NewK8s(i.Namespace, i.Clientset, i.Debug, i.Logger)

	if err := k.CreateNamespace(&apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: i.Namespace,
		}}); err != nil {
		return err
	}
	defer k.DeleteNamespace()

	if err := k.CreateDeployment(createDeploymentObject(i.ResourceName)); err != nil {
		return err
	}
	defer k.DeleteDeployment(i.ResourceName)

	if err := k.CreateService(createServiceObject(i.ResourceName)); err != nil {
		return err
	}
	defer k.DeleteService(i.ResourceName)

	if err := k.CreateIngress(createIngressObject(i.ResourceName, i.ExternalHostname)); err != nil {
		return err
	}
	defer k.DeleteIngress(i.ResourceName)

	if err := i.checkDNSRecord(); err != nil {
		return err
	}

	i.Chatwork.AddMessage("ingress check finished\n")
	return nil
}

func createDeploymentObject(deploymentName string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": deploymentName,
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.25.2",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	return deployment
}

func createServiceObject(serviceName string) *apiv1.Service {
	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"app": serviceName,
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol:   apiv1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
	return service
}

func createIngressObject(ingressName string, externalHostname string) *networkingv1.Ingress {
	var pathPrefix networkingv1.PathType = networkingv1.PathTypeImplementationSpecific
	ingressClassName := "alb"
	serviceName := ingressName

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressName,
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/backend-protocol":             "HTTP",
				"alb.ingress.kubernetes.io/connection-idle-timeout":      "60",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "5",
				"alb.ingress.kubernetes.io/healthcheck-protocol":         "HTTP",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "2",
				"alb.ingress.kubernetes.io/healthy-threshold-count":      "2",
				"alb.ingress.kubernetes.io/inbound-cidrs":                "0.0.0.0/0",
				"alb.ingress.kubernetes.io/target-type":                  "ip",
				"external-dns.alpha.kubernetes.io/hostname":              externalHostname,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: externalHostname,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathPrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return ingress
}

func (i *Ingress) checkDNSRecord() error {
	c := new(dns.Client)
	m := new(dns.Msg)

	i.Chatwork.AddMessage("ingress create finished\n")
	i.Logger().Println("Check DNS Record for: ", i.ExternalHostname)
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, false, func(ctx context.Context) (bool, error) {
		m.SetQuestion(dns.Fqdn(i.ExternalHostname), dns.TypeA)
		r, _, err := c.Exchange(m, "8.8.8.8:53")

		if err != nil {
			i.Logger().Println(err)
			return false, nil
		}

		if len(r.Answer) == 0 {
			i.Logger().Println("No record.")
			return false, nil
		}

		for _, ans := range r.Answer {
			if a, ok := ans.(*dns.A); ok {
				i.Logger().Println("Record is available:", a.A)
				i.Chatwork.AddMessage(fmt.Sprintf("Record is available: %s\n", a.A))
				return true, nil
			}
		}

		i.Logger().Infof("Record for %s is not yet available, retrying...", i.ExternalHostname)
		return false, nil
	})

	if err != nil {
		i.Logger().Error("Timed out waiting for DNS Record to be ready:", err)
		return err
	}
	return nil
}
