package fluent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/k8s"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/hashicorp/go-multierror"
)

type Fluent struct {
	*cmd.Checker
	Namespace     string
	Clientset     *kubernetes.Clientset
	LogBucketName string
	LogPath       string
	ResourceName  string
	ReplicaCount  int
	Awscfg        aws.Config
}

func NewFluent(checker *cmd.Checker) (*Fluent, error) {
	t := time.Now()

	namespace := fmt.Sprintf("fluent-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	if v := os.Getenv("RESOURCE_NAMESPACE"); v != "" {
		namespace = v
	}

	location, _ := time.LoadLocation("Asia/Tokyo")
	checker.Chatwork.AddMessage(fmt.Sprintf("Start in %s at %s\n", checker.ClusterName, time.Now().In(location).Format("2006-01-02 15:04:05")))

	checker.Logger().Infof("fluent check application Namespace: %s", namespace)
	checker.Chatwork.AddMessage(fmt.Sprintf("fluent check application Namespace: %s\n", namespace))

	resourceName := "burst-log-generator"

	env := "test"

	logBucketName := "kubernetes-logs"

	if v := os.Getenv("RESOURCE_NAME"); v != "" {
		resourceName = v
	}

	if v := os.Getenv("ENV"); v != "" {
		env = v
	}

	if v := os.Getenv("LOG_BUCKET_NAME"); v != "" {
		logBucketName = v
	}

	// path s3bucket/fluentd/env(test,stg,etc...)/namespace/dt=yyyymmdd
	logPath := fmt.Sprintf("fluentd/%s/%s/dt=%d%02d%02d", env, namespace, t.UTC().Year(), t.UTC().Month(), t.UTC().Day())
	if v := os.Getenv("LOG_PATH"); v != "" {
		logPath = v
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		return nil, fmt.Errorf("NewK8sClientset: %s", err)
	}

	awsConfig, err := config.NewAwsConfig(checker.Ctx)
	if err != nil {
		return nil, fmt.Errorf("NewAwsConfig: %s", err)
	}

	return &Fluent{
		Checker:       checker,
		Namespace:     namespace,
		Clientset:     k8sclient,
		ResourceName:  resourceName,
		LogBucketName: logBucketName,
		LogPath:       logPath,
		Awscfg:        awsConfig,
	}, nil
}

func (f *Fluent) Check() error {
	f.Chatwork.AddMessage("fluent check start\n")
	defer f.Chatwork.Send()

	nodeListOption := metav1.ListOptions{
		LabelSelector: "eks.amazonaws.com/capacityType=SPOT",
	}

	nodes, err := f.Clientset.CoreV1().Nodes().List(f.Ctx, nodeListOption)
	if err != nil {
		f.Logger().Errorf("Error List Nodes: %s", err)
		f.Chatwork.AddMessage(fmt.Sprintf("Error List Nodes: %s\n", err))
		return err
	}

	f.ReplicaCount = (len(nodes.Items) / 3) + 1
	f.Logger().Infof("%s replica counts: %d", f.ResourceName, f.ReplicaCount)
	f.Chatwork.AddMessage(fmt.Sprintf("%s replica counts: %d\n", f.ResourceName, f.ReplicaCount))

	defer func() {
		if err := f.cleanUpResources(); err != nil {
			f.Chatwork.AddMessage(fmt.Sprintf("Error Delete Resources: %s\n", err))
		}
	}()

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
	k := k8s.NewK8s(f.Namespace, f.Clientset, f.Logger)

	if err := k.CreateNamespace(
		f.Ctx,
		&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: f.Namespace,
			}}); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Create Namespace: %s\n", err))
		return err
	}

	if err := k.CreateDeployment(f.Ctx, f.createDeploymentObject(), f.Timeout); err != nil {
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

	if err = k.DeleteDeployment(f.ResourceName); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Delete Deployment: %s", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteNamespace(); err != nil {
		f.Chatwork.AddMessage(fmt.Sprintf("Error Delete Namespace: %s", err))
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (f *Fluent) checkS3Object() error {
	client := s3.NewFromConfig(f.Awscfg)
	t := time.Now()
	targetBucket := f.LogBucketName
	targetPrefix := f.LogPath

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(targetBucket),
		Prefix: aws.String(targetPrefix),
	}

	err := wait.PollUntilContextTimeout(f.Ctx, 60*time.Second, f.Timeout, false, func(ctx context.Context) (bool, error) {
		f.Logger().Infof("Wait fluentd output to s3://%s/%s ...", targetBucket, targetPrefix)

		result, err := client.ListObjectsV2(ctx, input)
		if err != nil {
			f.Logger().Warnf("Got an error retrieving items: %s", err)
			return false, nil
		}

		if len(result.Contents) != 0 {
			for _, item := range result.Contents {
				if item.LastModified.After(t) {
					f.Chatwork.AddMessage(fmt.Sprintf("fluentd output to s3://%s/%s/%s\n", targetBucket, targetPrefix, *item.Key))
					f.Logger().Infof("Name: %s ", *item.Key)
					f.Logger().Infof("Last modified: %s", *item.LastModified)
					f.Logger().Infof("Size: %d", item.Size)
					f.Logger().Infof("Storage class: %s", item.StorageClass)
					return true, nil
				}
			}
		}

		return false, nil
	})

	if err != nil {
		return fmt.Errorf("error waiting for S3 objects to be ready: %w", err)
	}

	f.Logger().Info("All S3 Objects are available.")

	return nil
}

func (f *Fluent) createDeploymentObject() *appsv1.Deployment {
	desireReplicacount := f.ReplicaCount

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.ResourceName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(desireReplicacount)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": f.ResourceName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": f.ResourceName,
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
													Values:   []string{f.ResourceName},
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
