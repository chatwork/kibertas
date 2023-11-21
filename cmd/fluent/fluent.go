package fluent

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
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Fluent struct {
	*cmd.Checker
	Env            string
	LogBucketName  string
	DeploymentName string
	Awscfg         aws.Config
}

func NewFluent(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) (*Fluent, error) {
	t := time.Now()

	namespace := fmt.Sprintf("fluent-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	logger().Infof("fluent check application namespace: %s", namespace)
	chatwork.AddMessage(fmt.Sprintf("fluent check application namespace: %s\n", namespace))

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

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		logger().Errorf("NewK8sClientset: %s", err)
		return nil, err
	}

	return &Fluent{
		Checker:        cmd.NewChecker(namespace, k8sclient, debug, logger, chatwork),
		Env:            env,
		DeploymentName: deploymentName,
		LogBucketName:  logBucketName,
		Awscfg:         config.NewAwsConfig(),
	}, nil
}

func (f *Fluent) Check() error {
	f.Chatwork.AddMessage("fluent check start\n")
	defer f.Chatwork.Send()

	if err := f.createResources(); err != nil {
		return err
	}

	if err := f.checkS3Object(); err != nil {
		return err
	}

	f.Chatwork.AddMessage("fluent check finished\n")
	return nil
}

func (f *Fluent) createResources() error {
	k := k8s.NewK8s(f.Namespace, f.Clientset, f.Debug, f.Logger)

	if err := k.CreateNamespace(&apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.Namespace,
		}}); err != nil {
		f.Chatwork.AddMessage(fmt.Sprint("Error Create Namespace:", err))
		return err
	}
	defer func() {
		if err := k.DeleteNamespace(); err != nil {
			f.Chatwork.AddMessage(fmt.Sprint("Error Delete Namespace:", err))
		}
	}()

	if err := k.CreateDeployment(createDeploymentObject(f.DeploymentName)); err != nil {
		f.Chatwork.AddMessage(fmt.Sprint("Error Create Deployment:", err))
		return err
	}
	defer func() {
		if err := k.DeleteDeployment(f.DeploymentName); err != nil {
			f.Chatwork.AddMessage(fmt.Sprint("Error Delete Deployment:", err))
		}
	}()
	return nil
}

func (f *Fluent) checkS3Object() error {
	client := s3.NewFromConfig(f.Awscfg)
	t := time.Now()
	targetBucket := f.LogBucketName
	targetPrefix := fmt.Sprintf("fluentd/%s/%s/dt=%d%02d%02d", f.Env, f.Namespace, t.UTC().Year(), t.UTC().Month(), t.UTC().Day())

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(targetBucket),
		Prefix: aws.String(targetPrefix),
	}

	err := wait.PollUntilContextTimeout(context.Background(), 60*time.Second, 20*time.Minute, false, func(ctx context.Context) (bool, error) {
		f.Logger().Infof("Wait fluentd output to s3://%s/%s ...", targetBucket, targetPrefix)

		result, err := client.ListObjectsV2(context.TODO(), input)
		if err != nil {
			f.Logger().Error("Got an error retrieving items:", err)
			return false, nil
		}

		if len(result.Contents) != 0 {
			for _, item := range result.Contents {
				if item.LastModified.After(t) {
					f.Chatwork.AddMessage(fmt.Sprintf("fluentd output to s3://%s/%s/%s\n", targetBucket, targetPrefix, *item.Key))
					f.Logger().Println("Name:          ", *item.Key)
					f.Logger().Println("Last modified: ", *item.LastModified)
					f.Logger().Println("Size:          ", item.Size)
					f.Logger().Println("Storage class: ", item.StorageClass)
					return true, nil
				}
			}
		}

		return false, nil
	})

	if err != nil {
		f.Logger().Error("Timed out waiting for output S3 Object:", err)
		return err
	}

	f.Logger().Infoln("All S3 Objects are available.")

	return nil
}

func createDeploymentObject(deploymentName string) *appsv1.Deployment {
	desireReplicacount := 2

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
					Affinity: &apiv1.Affinity{
						PodAntiAffinity: &apiv1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
								{
									TopologyKey: "kubernetes.io/hostname",
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "app.kubernetes.io/instance",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"fluentd"},
											},
										},
									},
								},
							},
						},
						PodAffinity: &apiv1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
								{
									TopologyKey: "kubernetes.io/hostname",
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "app",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{deploymentName},
											},
										},
									},
								},
							},
						},
					},
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
