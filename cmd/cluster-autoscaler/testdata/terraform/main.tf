// This example provisions
// - a subnet
// - a security group
// - IAM role
// - EKS cluster

// Usage:
//   terraform init
//   terraform plan -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=kibertas-ca -var capacity_type=SPOT -var node_template_app_label_value=sample-for-scale
//   terraform apply -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=kibertas-ca -var capacity_type=SPOT -var node_template_app_label_value=sample-for-scale


terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "prefix" {
    type = string
    description = "The prefix to use for all resources in this example"
}

variable "vpc_id" {
    type = string
    description = "The id of the VPC to use for this example"
}

variable "region" {
    type = string
    description = "The region to use for this example"
}

variable "node_template_app_label_value" {
    type = string
    description = "The value to use for the app label in the cluster-autoscaler node template"
}

variable "capacity_type" {
    type = string
    description = "The capacity type to use for this example. Valid values are ON_DEMAND and SPOT"
}

// vpc cidr block
data "aws_vpc" "vpc" {
    id = var.vpc_id
}

data "aws_caller_identity" "current" {}

resource "aws_eks_cluster" "cluster" {
    name = local.cluster_name
    role_arn = aws_iam_role.cluster.arn
    vpc_config {
        subnet_ids = aws_subnet.public[*].id
        security_group_ids = [aws_security_group.cluster.id]
    }
}

// node group for system pods and cluster-autoscaler
resource "aws_eks_node_group" "sys" {
    cluster_name = aws_eks_cluster.cluster.name
    node_group_name = "${var.prefix}-sys"
    node_role_arn = aws_iam_role.node.arn
    subnet_ids = aws_subnet.public[*].id
    scaling_config {
        desired_size = 2
        max_size = 2
        min_size = 2
    }
    capacity_type = "ON_DEMAND"
    instance_types = ["t3.large"]
    labels = {
        "role" = "sys"
    }
    taint {
        key = "node-role.kubernetes.io/control-plane"
        value  = ""
        effect = "NO_SCHEDULE"
    }
}

// node group based on spot instances
resource "aws_eks_node_group" "spot" {
    cluster_name = aws_eks_cluster.cluster.name
    node_group_name = "${var.prefix}-spot"
    node_role_arn = aws_iam_role.node.arn
    subnet_ids = aws_subnet.public[*].id
    scaling_config {
        desired_size = 1
        # We intend to let cluster-autoscaler discover the cluster and
        # this max_size, scaling nodes to 2.
        max_size = 3
        min_size = 0
    }
    // This is translated to eks.amazonaws.com/capacityType=[SPOT|ON_DEMAND]
    // node label by EKS.
    // The label must match karpenter.sh/capacity-type label in the NodePool.
    // Otherwise karpenter will complain with a message like:
    //   {"level":"ERROR","time":"*snip*","logger":"controller","message":"could not schedule pod","commit":"490ef94","controller":"provisioner","Pod":{"name":"sample-for-scale-68fcbd98cc-gs7g8","namespace":"cluster-autoscaler-test-20240619-pwpm2"},"error":"incompatible with nodepool \"default\", daemonset overhead={\"cpu\":\"150m\",\"pods\":\"2\"}, incompatible requirements, label \"eks.amazonaws.com/capacityType\" does not have known values"}
    capacity_type = var.capacity_type
    instance_types = ["t3.large"]
    labels = {
        "role" = "spot"
    }
    tags =  {
        "k8s.io/cluster-autoscaler/node-template/label/app": var.node_template_app_label_value,
        "k8s.io/cluster-autoscaler/enabled" = "true"
        "k8s.io/cluster-autoscaler/${aws_eks_cluster.cluster.name}" = "owned"
        "eks.amazonaws.com/capacityType" = "SPOT"
    }
}

resource "aws_iam_role" "cluster" {
    name = "${var.prefix}-cluster"
    assume_role_policy = <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": "sts:AssumeRole",
            "Principal": {
                "Service": "eks.amazonaws.com"
            },
            "Effect": "Allow",
            "Sid": ""
        }
    ]
}
EOF
}

// Add AmazonEKSClusterPolicy to cluster
resource "aws_iam_role_policy_attachment" "cluster" {
    role = aws_iam_role.cluster.name
    policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

// node role with necessary IAM permissions for cluster-autoscaler
resource "aws_iam_role" "node" {
    name = "${var.prefix}-node"
    assume_role_policy = <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": "sts:AssumeRole",
            "Principal": {
                "Service": "ec2.amazonaws.com"
            },
            "Effect": "Allow",
            "Sid": ""
        }
    ]
}
EOF
}

// Assign "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy" to nodes
resource "aws_iam_role_policy_attachment" "cni" {
    role = aws_iam_role.node.name
    policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}

// Assign AmazonEC2ContainerRegistryReadOnly to nodes
resource "aws_iam_role_policy_attachment" "ecr" {
    role = aws_iam_role.node.name
    policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

variable "autoscaler_name" {
    type = string
    description = "The name of the autoscaler to use. Valid values are karpenter and cluster-autoscaler"
    default = "cluster-autoscaler"
}

locals {
    cluster_name = "${var.prefix}-cluster"
    aws_account_id = data.aws_caller_identity.current.account_id
    node_policy_ka = <<EOF
{
    "Statement": [
        {
            "Action": [
                "ssm:GetParameter",
                "ec2:DescribeImages",
                "ec2:RunInstances",
                "ec2:DescribeSubnets",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeLaunchTemplates",
                "ec2:DescribeInstances",
                "ec2:DescribeInstanceTypes",
                "ec2:DescribeInstanceTypeOfferings",
                "ec2:DescribeAvailabilityZones",
                "ec2:DeleteLaunchTemplate",
                "ec2:CreateTags",
                "ec2:CreateLaunchTemplate",
                "ec2:CreateFleet",
                "ec2:DescribeSpotPriceHistory",
                "pricing:GetProducts"
            ],
            "Effect": "Allow",
            "Resource": "*",
            "Sid": "Karpenter"
        },
        {
            "Action": "ec2:TerminateInstances",
            "Condition": {
                "StringLike": {
                    "ec2:ResourceTag/karpenter.sh/nodepool": "*"
                }
            },
            "Effect": "Allow",
            "Resource": "*",
            "Sid": "ConditionalEC2Termination"
        },
        {
            "Effect": "Allow",
            "Action": "iam:PassRole",
            "Resource": "${aws_iam_role.node.arn}",
            "Sid": "PassNodeIAMRole"
        },
        {
            "Effect": "Allow",
            "Action": "eks:DescribeCluster",
            "Resource": "arn:aws:eks:${var.region}:${local.aws_account_id}:cluster/${local.cluster_name}",
            "Sid": "EKSClusterEndpointLookup"
        },
        {
            "Sid": "AllowScopedInstanceProfileCreationActions",
            "Effect": "Allow",
            "Resource": "*",
            "Action": [
            "iam:CreateInstanceProfile"
            ],
            "Condition": {
            "StringEquals": {
                "aws:RequestTag/kubernetes.io/cluster/${local.cluster_name}": "owned",
                "aws:RequestTag/topology.kubernetes.io/region": "${var.region}"
            },
            "StringLike": {
                "aws:RequestTag/karpenter.k8s.aws/ec2nodeclass": "*"
            }
            }
        },
        {
            "Sid": "AllowScopedInstanceProfileTagActions",
            "Effect": "Allow",
            "Resource": "*",
            "Action": [
            "iam:TagInstanceProfile"
            ],
            "Condition": {
            "StringEquals": {
                "aws:ResourceTag/kubernetes.io/cluster/${local.cluster_name}": "owned",
                "aws:ResourceTag/topology.kubernetes.io/region": "${var.region}",
                "aws:RequestTag/kubernetes.io/cluster/${local.cluster_name}": "owned",
                "aws:RequestTag/topology.kubernetes.io/region": "${var.region}"
            },
            "StringLike": {
                "aws:ResourceTag/karpenter.k8s.aws/ec2nodeclass": "*",
                "aws:RequestTag/karpenter.k8s.aws/ec2nodeclass": "*"
            }
            }
        },
        {
            "Sid": "AllowScopedInstanceProfileActions",
            "Effect": "Allow",
            "Resource": "*",
            "Action": [
            "iam:AddRoleToInstanceProfile",
            "iam:RemoveRoleFromInstanceProfile",
            "iam:DeleteInstanceProfile"
            ],
            "Condition": {
            "StringEquals": {
                "aws:ResourceTag/kubernetes.io/cluster/${local.cluster_name}": "owned",
                "aws:ResourceTag/topology.kubernetes.io/region": "${var.region}"
            },
            "StringLike": {
                "aws:ResourceTag/karpenter.k8s.aws/ec2nodeclass": "*"
            }
            }
        },
        {
            "Sid": "AllowInstanceProfileReadActions",
            "Effect": "Allow",
            "Resource": "*",
            "Action": "iam:GetInstanceProfile"
        },
        {
            "Sid": "AllowInterruptionQueueActions",
            "Effect": "Allow",
            "Resource": "${aws_sqs_queue.karpenter_interruption_queue.arn}",
            "Action": [
                "sqs:DeleteMessage",
                "sqs:GetQueueUrl",
                "sqs:ReceiveMessage"
            ]
        }
    ],
    "Version": "2012-10-17"
}
EOF
    node_policy_ca = <<EOF
{
    "Statement": [
      {
        "Effect": "Allow",
        "Action": [
            "autoscaling:DescribeAutoScalingGroups",
            "autoscaling:DescribeAutoScalingInstances",
            "autoscaling:DescribeLaunchConfigurations",
            "autoscaling:DescribeScalingActivities",
            "autoscaling:DescribeTags",
            "ec2:DescribeInstanceTypes",
            "ec2:DescribeLaunchTemplateVersions"
        ],
        "Resource": ["*"]
      },
      {
        "Effect": "Allow",
        "Action": [
            "autoscaling:SetDesiredCapacity",
            "autoscaling:TerminateInstanceInAutoScalingGroup",
            "ec2:DescribeImages",
            "ec2:GetInstanceTypesFromInstanceRequirements",
            "eks:DescribeNodegroup"
        ],
        "Resource": ["*"]
      }
    ],
    "Version": "2012-10-17"
}
EOF

    node_policy = var.autoscaler_name == "karpenter" ? local.node_policy_ka : local.node_policy_ca
}

// See https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/cloudprovider/aws/README.md#iam-policy
resource "aws_iam_role_policy" "node" {
    name = "${var.prefix}-node-ca"
    role = aws_iam_role.node.id
    policy = local.node_policy
}

// Give nodes AmazonEKSWorkerNodePolicy
resource "aws_iam_role_policy_attachment" "node" {
    role = aws_iam_role.node.name
    policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}

resource "aws_security_group" "cluster" {
    name = "${var.prefix}-cluster"
    vpc_id = data.aws_vpc.vpc.id
    ingress {
        from_port = 443
        to_port = 443
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }
    egress {
        from_port = 0
        to_port = 0
        protocol = "-1"
        cidr_blocks = ["0.0.0.0/0"]
    }
    tags = {
        // Must match EC2NodeClass securityGroupSelectorTerms.tags
        // Otherwise karpenter complains with a message like:
        //   {"level":"ERROR","time":"*snip*","logger":"controller","message":"failed listing instance types for default","commit":"490ef94","controller":"disruption","error":"no subnets found"}
        "karpenter.sh/discovery" = local.cluster_name
    }
}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_subnet" "public" {
    count = 2
    vpc_id = data.aws_vpc.vpc.id
    cidr_block = "${cidrsubnet(data.aws_vpc.vpc.cidr_block, 4, 10+count.index)}"
    availability_zone = data.aws_availability_zones.available.names[count.index%length(data.aws_availability_zones.available.names)]
    map_public_ip_on_launch = true
    tags = {
        // Must match EC2NodeClass subnetSelectorTerms.tags
        "karpenter.sh/discovery" = local.cluster_name
    }
}

resource "aws_sqs_queue" "karpenter_interruption_queue" {
    name = "${local.cluster_name}"
}
