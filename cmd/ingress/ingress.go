package ingress

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/k8s"

	"github.com/miekg/dns"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/hashicorp/go-multierror"
)

type Ingress struct {
	*cmd.Checker
	Namespace        string
	Clientset        *kubernetes.Clientset
	NoDnsCheck       bool
	IngressClassName string
	ResourceName     string
	ExternalHostname string
}

func NewIngress(checker *cmd.Checker, noDnsCheck bool) (*Ingress, error) {
	t := time.Now()

	namespace := fmt.Sprintf("ingress-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))

	location, _ := time.LoadLocation("Asia/Tokyo")
	checker.Chatwork.AddMessage(fmt.Sprintf("Start in %s at %s\n", checker.ClusterName, time.Now().In(location).Format("2006-01-02 15:04:05")))

	checker.Logger().Infof("Ingress check application Namespace: %s", namespace)
	checker.Chatwork.AddMessage(fmt.Sprintf("Ingress check application Namespace: %s\n", namespace))

	resourceName := "sample"
	externalHostName := "sample-skmt.cwtest.info"
	ingressClassName := "alb"

	if v := os.Getenv("RESOURCE_NAME"); v != "" {
		resourceName = v
	}
	if v := os.Getenv("EXTERNAL_HOSTNAME"); v != "" {
		externalHostName = v
	}

	if v := os.Getenv("INGRESS_CLASS_NAME"); v != "" {
		ingressClassName = v
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		return nil, fmt.Errorf("error NewK8sClientset: %s", err)
	}

	return &Ingress{
		Checker:          checker,
		Namespace:        namespace,
		Clientset:        k8sclient,
		ResourceName:     resourceName,
		NoDnsCheck:       noDnsCheck,
		IngressClassName: ingressClassName,
		ExternalHostname: externalHostName,
	}, nil
}

func (i *Ingress) Check() error {
	i.Chatwork.AddMessage("Ingress check start\n")
	defer i.Chatwork.Send()

	defer func() {
		if err := i.cleanUpResources(); err != nil {
			i.Chatwork.AddMessage(fmt.Sprintf("Error Delete Resources: %s\n", err))
		}
	}()

	if err := i.createResources(); err != nil {
		return err
	}

	if i.NoDnsCheck {
		i.Chatwork.AddMessage("Skip Dns Check\n")
		i.Logger().Info("Skip Dns Check")
	} else {
		if err := i.checkDNSRecord(); err != nil {
			return err
		}
	}

	i.Chatwork.AddMessage("Ingress check finished\n")
	return nil
}

func (i *Ingress) createResources() error {
	k := k8s.NewK8s(i.Namespace, i.Clientset, i.Logger)

	if err := k.CreateNamespace(
		i.Ctx,
		&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: i.Namespace,
			}}); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Create Namespace: %s\n", err))
		return err
	}
	if err := k.CreateDeployment(i.Ctx, i.createDeploymentObject(), i.Timeout); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Create Deployment: %s\n", err))
		return err
	}
	if err := k.CreateService(i.Ctx, i.createServiceObject()); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Create Service: %s\n", err))
		return err
	}
	if err := k.CreateIngress(i.Ctx, i.createIngressObject(), i.Timeout); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Create Ingress: %s\n", err))
		return err
	}
	return nil
}

func (i *Ingress) cleanUpResources() error {
	if i.Debug {
		i.Logger().Info("Skip Delete Resources")
		i.Chatwork.AddMessage("Skip Delete Resources\n")
		return nil
	}
	k := k8s.NewK8s(i.Namespace, i.Clientset, i.Logger)
	var result *multierror.Error
	var err error
	if err = k.DeleteIngress(i.ResourceName); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Delete Ingress: %s\n", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteService(i.ResourceName); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Delete Service: %s\n", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteDeployment(i.ResourceName); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Delete Deployment: %s\n", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteNamespace(); err != nil {
		i.Chatwork.AddMessage(fmt.Sprintf("Error Delete Namespace: %s\n", err))
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (i *Ingress) createDeploymentObject() *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: i.ResourceName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": i.ResourceName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": i.ResourceName,
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

func (i *Ingress) createServiceObject() *apiv1.Service {
	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: i.ResourceName,
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"app": i.ResourceName,
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

func (i *Ingress) createIngressObject() *networkingv1.Ingress {
	var pathPrefix networkingv1.PathType = networkingv1.PathTypeImplementationSpecific
	serviceName := i.ResourceName

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: i.ResourceName,
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/backend-protocol":             "HTTP",
				"alb.ingress.kubernetes.io/connection-idle-timeout":      "60",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "5",
				"alb.ingress.kubernetes.io/healthcheck-protocol":         "HTTP",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "2",
				"alb.ingress.kubernetes.io/healthy-threshold-count":      "2",
				"alb.ingress.kubernetes.io/inbound-cidrs":                "0.0.0.0/0",
				"alb.ingress.kubernetes.io/target-type":                  "ip",
				"external-dns.alpha.kubernetes.io/hostname":              i.ExternalHostname,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &i.IngressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: i.ExternalHostname,
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

	i.Logger().Infof("Check DNS Record for: %s", i.ExternalHostname)
	err := wait.PollUntilContextTimeout(i.Ctx, 30*time.Second, i.Timeout, false, func(ctx context.Context) (bool, error) {
		m.SetQuestion(dns.Fqdn(i.ExternalHostname), dns.TypeA)
		r, _, err := c.Exchange(m, "8.8.8.8:53")

		if err != nil {
			i.Logger().Warn(err)
			return false, nil
		}

		if len(r.Answer) == 0 {
			i.Logger().Info("No record.")
			return false, nil
		}

		for _, ans := range r.Answer {
			if a, ok := ans.(*dns.A); ok {
				i.Logger().Infof("Record is available: %s", a.A)
				i.Chatwork.AddMessage(fmt.Sprintf("Record is available: %s\n", a.A))
				return true, nil
			}
		}

		i.Logger().Infof("Record for %s is not yet available, retrying...", i.ExternalHostname)
		return false, nil
	})

	if err != nil {
		return fmt.Errorf("waiting for DNS Record to be ready: %w", err)
	}

	return nil
}
