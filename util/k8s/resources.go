package k8s

import (
	"context"
	"log"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

//Createは本当はApplyにしたいんだけど、ApplyがないのでCreateで代用

func CreateNamespace(namespace string, clientset *kubernetes.Clientset) error {
	// Check if namespace exists
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})

	ns := &apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	if err != nil {
		// If namespace does not exist, create it
		_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		log.Println("Namespace created")
	} else {
		log.Println("Namespace already exists")
	}
	return nil
}

func DeleteNamespace(namespace string, clientset *kubernetes.Clientset) error {
	err := clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
	if err != nil {
		log.Fatalf("Namespace %s delete error", namespace)
		return err
	}
	log.Printf("Namespace %s deleted", namespace)
	return nil
}

func CreateDeployment(deployment *appsv1.Deployment, namespace string, clientset *kubernetes.Clientset) error {
	deploymentsClient := clientset.AppsV1().Deployments(namespace)

	// Create Deployment
	log.Println("Creating deployment:", deployment.Name)
	result, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
	log.Printf("Created deployment %q.\n", result.GetObjectMeta().GetName())

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		deployment, err := deploymentsClient.Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas {
			return true, nil
		} else {
			log.Printf("Waiting for pods to be ready, current: %d, desired: %d\n", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
			return false, nil
		}
	})

	if err != nil {
		log.Fatal("Timed out waiting for pods to be ready:", err)
		return err
	}

	log.Println("All pods are ready")
	return nil
}

func DeleteDeployment(deploymentName string, namespace string, clientset *kubernetes.Clientset) error {
	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	deletePolicy := metav1.DeletePropagationForeground
	log.Println("Deleting deployment...: ", deploymentName)
	if err := deploymentsClient.Delete(context.TODO(), deploymentName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return err
	}
	log.Println("Deleted deployment.")
	return nil
}

func CreateService(service *apiv1.Service, namespace string, clientset *kubernetes.Clientset) error {
	serviceClient := clientset.CoreV1().Services(namespace)

	log.Println("Create service:", service.Name)
	_, err := serviceClient.Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func DeleteService(serviceName string, namespace string, clientset *kubernetes.Clientset) error {
	serviceClient := clientset.CoreV1().Services(namespace)
	deletePolicy := metav1.DeletePropagationForeground

	log.Println("Deleting service: ", serviceName)
	if err := serviceClient.Delete(context.TODO(), serviceName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return err
	}
	log.Println("Deleted service.")
	return nil
}

func CreateIngress(ingress *networkingv1.Ingress, namespace string, clientset *kubernetes.Clientset) error {
	ingressClient := clientset.NetworkingV1().Ingresses(namespace)

	_, err := ingressClient.Create(context.TODO(), ingress, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		ingress, err := ingressClient.Get(ctx, ingress.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		for _, address := range ingress.Status.LoadBalancer.Ingress {
			if address.Hostname != "" {
				log.Printf("Ingress is now available at Hostname: %s\n", address.Hostname)
				return true, nil
			}
		}
		log.Println("Ingress is not yet available, retrying...")
		return false, nil
	})

	if err != nil {
		log.Fatal("Timed out waiting for ingress to be ready:", err)
		return err
	}

	return nil
}

func DeleteIngress(ingressName string, namespace string, clientset *kubernetes.Clientset) error {
	ingressClient := clientset.NetworkingV1().Ingresses(namespace)
	deletePolicy := metav1.DeletePropagationForeground

	log.Println("Deleting ingress")
	if err := ingressClient.Delete(context.TODO(), ingressName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		return err
	}
	log.Println("Deleted ingress:", ingressName)
	return nil
}
