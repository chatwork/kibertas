package k8s

import (
	"context"
	"fmt"
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
	logger    func() *logrus.Entry
}

func NewK8s(namespace string, clientset *kubernetes.Clientset, logger func() *logrus.Entry) *K8s {
	return &K8s{
		namespace: namespace,
		clientset: clientset,
		logger:    logger,
	}
}

// Createは本当はApplyにしたいんだけど、ApplyがないのでCreateで代用
func (k *K8s) CreateNamespace(ctx context.Context, ns *apiv1.Namespace) error {
	_, err := k.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	k.logger().Infof("Creating Namespace: %s", ns.Name)
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Warnf("Namespace %s already exists", ns.Name)
		_, err = k.clientset.CoreV1().Namespaces().Update(context.TODO(), ns, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Errorf("Error updating namespace %s", ns.Name)
			return err
		}
		return nil
	} else if err != nil {
		k.logger().Info("Error creating namespace")
		return err
	}
	k.logger().Info("Namespace created")
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

func (k *K8s) CreateDeployment(ctx context.Context, deployment *appsv1.Deployment, timeout time.Duration) error {
	deploymentsClient := k.clientset.AppsV1().Deployments(k.namespace)

	// Create Deployment
	k.logger().Infof("Creating Deployment: %s", deployment.Name)
	result, err := deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Warnf("Already exists, updating deployment: %s", deployment.Name)
		_, err = deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Errorf("Error Updating Deployment: %s", err)
			return err
		}
	} else if err != nil {
		k.logger().Infof("Error Creating Deployment: %s", deployment.Name)
	}

	k.logger().Infof("Created Deployment %s", result.GetObjectMeta().GetName())

	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		deployment, err := deploymentsClient.Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
			return true, nil
		} else {
			k.logger().Infof("Waiting for Pods to be ready, current: %d, desired: %d", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
			return false, nil
		}
	})

	if err != nil {
		return fmt.Errorf("waiting for Pods to be ready: %w", err)
	}

	k.logger().Info("All Pods are ready")
	return nil
}

func (k *K8s) DeleteDeployment(deploymentName string) error {
	deploymentsClient := k.clientset.AppsV1().Deployments(k.namespace)
	deletePolicy := metav1.DeletePropagationForeground

	k.logger().Infof("Deleting Deployment: %s", deploymentName)
	if err := deploymentsClient.Delete(context.TODO(), deploymentName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		k.logger().Errorf("Error Deleting Deployment: %s", err)
		return err
	}
	k.logger().Info("Deleted Deployment.")
	return nil
}

func (k *K8s) CreateService(ctx context.Context, service *apiv1.Service) error {
	serviceClient := k.clientset.CoreV1().Services(k.namespace)

	k.logger().Infof("Create Service: %s", service.Name)
	_, err := serviceClient.Create(ctx, service, metav1.CreateOptions{})
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Warnf("Already exists, updating Service: %s", service.Name)
		_, err = serviceClient.Update(ctx, service, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Errorf("Error updating Service: %s", err)
			return err
		}
	} else if err != nil {
		k.logger().Errorf("Error creating Service: %s", err)
		return err
	}
	k.logger().Info("Created Service.")
	return nil
}

func (k *K8s) DeleteService(serviceName string) error {
	serviceClient := k.clientset.CoreV1().Services(k.namespace)
	deletePolicy := metav1.DeletePropagationForeground

	k.logger().Infof("Deleting Service: %s", serviceName)
	if err := serviceClient.Delete(context.TODO(), serviceName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		k.logger().Errorf("Error deleting Service: %s", err)
		return err
	}
	k.logger().Info("Deleted Service.")
	return nil
}

func (k *K8s) CreateIngress(ctx context.Context, ingress *networkingv1.Ingress, timeout time.Duration) error {
	ingressClient := k.clientset.NetworkingV1().Ingresses(k.namespace)

	k.logger().Infof("Creating Ingress: %s", ingress.Name)
	_, err := ingressClient.Create(ctx, ingress, metav1.CreateOptions{})
	if err != nil && kerrors.IsAlreadyExists(err) {
		k.logger().Warnf("Already exists, updating Ingress: %s", ingress.Name)
		_, err = ingressClient.Update(ctx, ingress, metav1.UpdateOptions{})
		if err != nil {
			k.logger().Errorf("Error updating Ingress: %s", err)
			return err
		}
	} else if err != nil {
		k.logger().Errorf("Error creating Ingress: %s", err)
		return err
	}

	if *ingress.Spec.IngressClassName == "alb" {
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
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
			k.logger().Info("Ingress is not yet available, retrying...")
			return false, nil
		})

		if err != nil {
			return fmt.Errorf("waiting for Ingress to be ready: %w", err)
		}
	}
	return nil
}

func (k *K8s) DeleteIngress(ingressName string) error {
	ingressClient := k.clientset.NetworkingV1().Ingresses(k.namespace)
	deletePolicy := metav1.DeletePropagationForeground

	k.logger().Infof("Deleting Ingress: %s", ingressName)
	if err := ingressClient.Delete(context.TODO(), ingressName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		k.logger().Errorf("Error deleting Ingress: %s", err)
		return err
	}
	k.logger().Infof("Deleted Ingress: %s", ingressName)
	return nil
}
