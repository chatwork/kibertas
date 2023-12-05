package fluent

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/k8s"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/hashicorp/go-multierror"
)

type Fluent struct {
	*cmd.Checker
	Env            string
	LogBucketName  string
	DeploymentName string
	ReplicaCount   int
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
	timeout := 20

	if v := os.Getenv("DEPLOYMENT_NAME"); v != "" {
		deploymentName = v
	}

	if v := os.Getenv("ENV"); v != "" {
		env = v
	}

	if v := os.Getenv("LOG_BUCKET_NAME"); v != "" {
		logBucketName = v
	}

	var err error
	if v := os.Getenv("TIMEOUT"); v != "" {
		timeout, err = strconv.Atoi(v)
		if err != nil {
			logger().Errorf("strconv.Atoi: %s", err)
			return nil, err
		}
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		logger().Errorf("NewK8sClientset: %s", err)
		return nil, err
	}

	return &Fluent{
		Checker:        cmd.NewChecker(namespace, k8sclient, debug, logger, chatwork, time.Duration(timeout)*time.Minute),
		Env:            env,
		DeploymentName: deploymentName,
		LogBucketName:  logBucketName,
		Awscfg:         config.NewAwsConfig(),
	}, nil
}

func (f *Fluent) Check(ctx context.Context) error {
	f.Chatwork.AddMessage("fluent check start\n")
	defer f.Chatwork.Send()

	nodeListOption := metav1.ListOptions{
		LabelSelector: "eks.amazonaws.com/capacityType=SPOT",
	}

	nodes, err := f.Clientset.CoreV1().Nodes().List(ctx, nodeListOption)
	if err != nil {
		f.Logger().Errorf("Error List Nodes: %s", err)
		f.Chatwork.AddMessage(fmt.Sprintf("Error List Nodes: %s\n", err))
		return err
	}

	f.ReplicaCount = (len(nodes.Items) / 3) + 1
	f.Logger().Infof("%s replica counts: %d", f.DeploymentName, f.ReplicaCount)
	f.Chatwork.AddMessage(fmt.Sprintf("%s replica counts: %d\n", f.DeploymentName, f.ReplicaCount))

	defer func() {
		if err := f.cleanUpResources(); err != nil {
			f.Chatwork.AddMessage(fmt.Sprintf("Error Delete Resources: %s\n", err))
		}
	}()

	if err := f.createResources(ctx); err != nil {
		return err
	}

	if err := f.checkS3Object(ctx); err != nil {
		return err
	}

	f.Chatwork.AddMessage("fluent check finished\n")
	return nil
}

func (f *Fluent) createResources(ctx context.Context) error {
	k := k8s.NewK8s(f.Namespace, f.Clientset, f.Logger)

	if err := k.CreateNamespace(
		ctx,
		&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: f.Namespace,
			}}); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Create Namespace: %s\n", err))
		return err
	}

	if err := k.CreateDeployment(ctx, f.createDeploymentObject(), f.Timeout); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Create Deployment: %s\n", err))
		return err
	}

	return nil
}

func (f *Fluent) cleanUpResources() error {
	if f.Debug {
		f.Logger().Info("Skip Delete Resources")
		f.Chatwork.AddMessage("Skip Delete Resources\n")
		return nil
	}
	k := k8s.NewK8s(f.Namespace, f.Clientset, f.Logger)
	var result *multierror.Error
	var err error

	if err = k.DeleteDeployment(f.DeploymentName); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Delete Deployment: %s", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteNamespace(); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Delete Namespace: %s", err))
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (f *Fluent) checkS3Object(ctx context.Context) error {
	client := s3.NewFromConfig(f.Awscfg)
	t := time.Now()
	targetBucket := f.LogBucketName
	targetPrefix := fmt.Sprintf("fluentd/%s/%s/dt=%d%02d%02d", f.Env, f.Namespace, t.UTC().Year(), t.UTC().Month(), t.UTC().Day())

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(targetBucket),
		Prefix: aws.String(targetPrefix),
	}

	err := wait.PollUntilContextTimeout(ctx, 60*time.Second, f.Timeout, false, func(ctx context.Context) (bool, error) {
		f.Logger().Infof("Wait fluentd output to s3://%s/%s ...", targetBucket, targetPrefix)

		result, err := client.ListObjectsV2(ctx, input)
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
		if err.Error() == "context canceled" {
			f.Logger().Error("Context canceled in waiting for S3 objects")
			f.Chatwork.AddMessage("Context canceled in waiting for S3 objects")
		} else if err.Error() == "context deadline exceeded" {
			f.Logger().Error("Timed out waiting for S3 object")
			f.Chatwork.AddMessage("Timed out waiting for S3 object")
		} else {
			f.Logger().Error("Error waiting for S3 object:", err)
			f.Chatwork.AddMessage(fmt.Sprintf("Error waiting for S3 object: %s\n", err))
		}
		return err
	}

	f.Logger().Infoln("All S3 Objects are available.")

	return nil
}

func (f *Fluent) createDeploymentObject() *appsv1.Deployment {
	desireReplicacount := f.ReplicaCount

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.DeploymentName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(desireReplicacount)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": f.DeploymentName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": f.DeploymentName,
					},
				},
				Spec: apiv1.PodSpec{
					Affinity: &apiv1.Affinity{
						PodAntiAffinity: &apiv1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []apiv1.WeightedPodAffinityTerm{
								{
									Weight: 10,
									PodAffinityTerm: apiv1.PodAffinityTerm{
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
								{
									Weight: 1,
									PodAffinityTerm: apiv1.PodAffinityTerm{
										TopologyKey: "kubernetes.io/hostname",
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "app",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{f.DeploymentName},
												},
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
