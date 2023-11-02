package fluent

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type Fluent struct {
	env            string
	namespace      string
	logBucketName  string
	deploymentName string
	clientset      *kubernetes.Clientset
	awscfg         aws.Config
}

func NewFluent() *Fluent {
	t := time.Now()

	namespace := fmt.Sprintf("fluent-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	log.Printf("fluent check application namespace: %s\n", namespace)

	deploymentName := "burst-log-generator"
	env := "cwtest"
	logBucketName := "cwtest-kubernetes-logs"

	if v := os.Getenv("DEPLOYMENT_NAME"); v != "" {
		deploymentName = v
	}

	if v := os.Getenv("ENV"); v != "" {
		env = v
	}

	if v := os.Getenv("LOG_BUCKET_NAME"); v != "" {
		logBucketName = v
	}

	return &Fluent{
		env:            env,
		namespace:      namespace,
		deploymentName: deploymentName,
		logBucketName:  logBucketName,
		clientset:      config.InitK8sConfig(),
		awscfg:         config.InitAwsConfig(),
	}
}

func (f *Fluent) Check() error {
	k8s.CreateNamespace(f.namespace, f.clientset)
	defer k8s.DeleteNamespace(f.namespace, f.clientset)

	err := k8s.CreateDeployment(createDeploymentObject(f.deploymentName), f.namespace, f.clientset)
	if err != nil {
		return err
	}
	defer k8s.DeleteDeployment(f.deploymentName, f.namespace, f.clientset)
	err = f.checkS3Object()
	if err != nil {
		return err
	}
	return nil
}

func (f *Fluent) checkS3Object() error {
	client := s3.NewFromConfig(f.awscfg)
	t := time.Now()
	targetBucket := f.logBucketName
	targetPrefix := fmt.Sprintf("fluentd/%s/%s/dt=%d%02d%02d", f.env, f.namespace, t.UTC().Year(), t.UTC().Month(), t.UTC().Day())

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(targetBucket),
		Prefix: aws.String(targetPrefix),
	}

	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		log.Printf("Wait fluentd output to s3://%s/%s ...", targetBucket, targetPrefix)

		result, err := client.ListObjectsV2(context.TODO(), input)
		if err != nil {
			log.Fatal("Got an error retrieving items:", err)
			return false, nil
		}

		if len(result.Contents) != 0 {
			for _, item := range result.Contents {
				if item.LastModified.After(t) {
					log.Println("Name:          ", *item.Key)
					log.Println("Last modified: ", *item.LastModified)
					log.Println("Size:          ", item.Size)
					log.Println("Storage class: ", item.StorageClass)
					return true, nil
				}
			}
		}

		return false, nil
	})

	if err != nil {
		log.Fatal("Timed out waiting for output S3 Object:", err)
		return err
	}

	log.Println("All S3 Objects are available.")

	return nil
}

func createDeploymentObject(deploymentName string) *appsv1.Deployment {
	desireReplicacount := 1

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(desireReplicacount)),
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
							Name:  "ubuntu",
							Image: "ubuntu",
							Args: []string{
								"sh", "-c", "while true; do cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 128; echo; sleep 2; done",
							},
						},
					},
				},
			},
		},
	}

	return deployment
}
