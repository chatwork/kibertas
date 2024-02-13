// This example provisions
// - a subnet
// - a security group
// - IAM role
// - EKS cluster

// Usage:
//   terraform init
//   terraform plan -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=kibertas-ca-
//   terraform apply -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=kibertas-ca-

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

resource "aws_eks_cluster" "cluster" {
    name = "${var.prefix}-cluster"
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
    capacity_type = var.capacity_type
    instance_types = ["t3.large"]
    labels = {
        "role" = "spot"
    }
    tags =  {
        "k8s.io/cluster-autoscaler/node-template/label/app": var.node_template_app_label_value,
        "k8s.io/cluster-autoscaler/enabled" = "true"
        "k8s.io/cluster-autoscaler/${aws_eks_cluster.cluster.name}" = "owned"
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

// See https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/cloudprovider/aws/README.md#iam-policy
resource "aws_iam_role_policy" "node" {
    name = "${var.prefix}-node"
    role = aws_iam_role.node.id
    policy = <<EOF
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
}
