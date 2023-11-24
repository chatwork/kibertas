package k8s

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type K8s struct {
	namespace string
	clientset *kubernetes.Clientset
	debug     bool
	logger    func() *logrus.Entry
}

func NewK8s(namespace string, clientset *kubernetes.Clientset, debug bool, logger func() *logrus.Entry) *K8s {
	return &K8s{
		namespace: namespace,
		clientset: clientset,
		debug:     debug,
		logger:    logger,
	}
}

// Createは本当はApplyにしたいんだけど、ApplyがないのでCreateで代用
func (k *K8s) CreateNamespace(ns *apiv1.Namespace) error {
	_, err := k.clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	k.logger().Infof("Creating Namespace: %s", ns.Name)
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Infof("Namespace %s already exists", ns.Name)
		_, err = k.clientset.CoreV1().Namespaces().Update(context.TODO(), ns, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Infof("Error updating namespace %s", ns.Name)
			return err
		}
		return nil
	} else if err != nil {
		k.logger().Infoln("Error creating namespace")
		return err
	}
	k.logger().Infoln("Namespace created")
	return nil
}

func (k *K8s) DeleteNamespace() error {
	err := k.clientset.CoreV1().Namespaces().Delete(context.TODO(), k.namespace, metav1.DeleteOptions{})
	if err != nil {
		k.logger().Errorf("Error Delete Namespace: %s", k.namespace)
		return err
	}
	k.logger().Infof("Deleted Namespace: %s", k.namespace)
	return nil
}

func (k *K8s) CreateDeployment(deployment *appsv1.Deployment) error {
	deploymentsClient := k.clientset.AppsV1().Deployments(k.namespace)

	// Create Deployment
	k.logger().Infof("Creating Deployment: %s", deployment.Name)
	result, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Infof("Already exists, updating deployment: %s", deployment.Name)
		_, err = deploymentsClient.Update(context.TODO(), deployment, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Error("Error Updating Deployment:", err)
			return err
		}
	} else if err != nil {
		k.logger().Infof("Error Creating Deployment: %s", deployment.Name)
	}

	k.logger().Infof("Created Deployment %s", result.GetObjectMeta().GetName())

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		deployment, err := deploymentsClient.Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
			return true, nil
		} else {
			k.logger().Infof("Waiting for pods to be ready, current: %d, desired: %d", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
			return false, nil
		}
	})

	if err != nil {
		k.logger().Error("Timed out waiting for Pods to be ready:", err)
		return err
	}

	k.logger().Infoln("All Pods are ready")
	return nil
}

func (k *K8s) DeleteDeployment(deploymentName string) error {
	deploymentsClient := k.clientset.AppsV1().Deployments(k.namespace)
	deletePolicy := metav1.DeletePropagationForeground

	k.logger().Infof("Deleting Deployment: %s", deploymentName)
	if err := deploymentsClient.Delete(context.TODO(), deploymentName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		k.logger().Error("Error Deleting Deployment: ", err)
		return err
	}
	k.logger().Infoln("Deleted Deployment.")
	return nil
}

func (k *K8s) CreateService(service *apiv1.Service) error {
	serviceClient := k.clientset.CoreV1().Services(k.namespace)

	k.logger().Infof("Create service: %s", service.Name)
	_, err := serviceClient.Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Infof("Already exists, updating service: %s", service.Name)
		_, err = serviceClient.Update(context.TODO(), service, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Error("Error updating service:", err)
			return err
		}
	} else if err != nil {
		k.logger().Errorf("Error creating service: %s", err)
		return err
	}
	k.logger().Infoln("Created service.")
	return nil
}

func (k *K8s) DeleteService(serviceName string) error {
	serviceClient := k.clientset.CoreV1().Services(k.namespace)
	deletePolicy := metav1.DeletePropagationForeground

	k.logger().Infof("Deleting service: %s", serviceName)
	if err := serviceClient.Delete(context.TODO(), serviceName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		k.logger().Error("Error deleting service:", err)
		return err
	}
	k.logger().Infoln("Deleted service.")
	return nil
}

func (k *K8s) CreateIngress(ingress *networkingv1.Ingress, NoAddressCheck bool) error {
	ingressClient := k.clientset.NetworkingV1().Ingresses(k.namespace)

	k.logger().Infof("Creating ingress: %s", ingress.Name)
	_, err := ingressClient.Create(context.TODO(), ingress, metav1.CreateOptions{})
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Infof("Already exists, updating ingress: %s", ingress.Name)
		_, err = ingressClient.Update(context.TODO(), ingress, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Error("Error updating ingress:", err)
			return err
		}
	} else if err != nil {
		k.logger().Error("Error creating ingress:", err)
		return err
	}

	if !NoAddressCheck {
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
			ingress, err := ingressClient.Get(ctx, ingress.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			for _, address := range ingress.Status.LoadBalancer.Ingress {
				if address.Hostname != "" {
					k.logger().Infof("Ingress is now available at Hostname: %s", address.Hostname)
					return true, nil
				}
			}
			k.logger().Infoln("Ingress is not yet available, retrying...")
			return false, nil
		})

		if err != nil {
			k.logger().Error("Timed out waiting for ingress to be ready:", err)
			return err
		}
	}
	return nil
}

func (k *K8s) DeleteIngress(ingressName string) error {
	ingressClient := k.clientset.NetworkingV1().Ingresses(k.namespace)
	deletePolicy := metav1.DeletePropagationForeground

	k.logger().Infof("Deleting ingress: %s", ingressName)
	if err := ingressClient.Delete(context.TODO(), ingressName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		k.logger().Error("Error deleting ingress:", err)
		return err
	}
	k.logger().Infof("Deleted ingress: %s", ingressName)
	return nil
}
