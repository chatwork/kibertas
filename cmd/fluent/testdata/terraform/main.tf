// This example provisions
// - an S3 bucket
// - a subnet
// - a security group
// - IAM role
// - EKS cluster

// Usage:
//   terraform init -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=$PREFIX -backend-config bucket=$TERRAFORM_STATE_BUCKET -backend-config key=$TERRAFORM_STATE_KEY -backend-config region=ap-northeast-1
//   terraform plan -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=$PREFIX
//   terraform apply -var vpc_id=$VPC_ID -var region=ap-northeast-1 -var prefix=$PREFIX

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  backend "s3" {}
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

variable "eks_access_principal_arn" {
    type = string
    description = "The principal arn to use for alternative eks access"
}

// vpc cidr block
data "aws_vpc" "vpc" {
    id = var.vpc_id
}

resource "aws_s3_bucket" "bucket" {
    bucket = "${var.prefix}-bucket"
    force_destroy = true
}

locals {
  service_ipv4_cidr = "10.100.0.0/16"
}

resource "aws_eks_cluster" "cluster" {
    name = "${var.prefix}-cluster"
    role_arn = aws_iam_role.cluster.arn
    vpc_config {
        subnet_ids = aws_subnet.public[*].id
        security_group_ids = [aws_security_group.cluster.id]
    }
    access_config {
        authentication_mode = "API_AND_CONFIG_MAP"
    }
    kubernetes_network_config {
      service_ipv4_cidr = local.service_ipv4_cidr
    }
}

resource "aws_eks_access_entry" "admin" {
  cluster_name      = aws_eks_cluster.cluster.name
  principal_arn     = var.eks_access_principal_arn
}

resource "aws_eks_access_policy_association" "admin" {
  cluster_name = aws_eks_cluster.cluster.name
  policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
  principal_arn = var.eks_access_principal_arn
  access_scope {
    type       = "cluster"
  }
}

module "eks_managed_node_group" {
  source = "terraform-aws-modules/eks/aws//modules/eks-managed-node-group"
  name = "spot"
  cluster_name = aws_eks_cluster.cluster.name
  cluster_version = aws_eks_cluster.cluster.version
  create_iam_role = false
  iam_role_arn = aws_iam_role.node.arn
  subnet_ids = aws_subnet.public[*].id
  cluster_primary_security_group_id = aws_security_group.cluster.id
  vpc_security_group_ids = [aws_security_group.node.id]
  instance_types = ["t3.large"]
  capacity_type = "SPOT"
  cluster_service_cidr = local.service_ipv4_cidr
  metadata_options = {
    http_tokens = "optional"
    # As the default image is al2023 (like amazon-eks-node-al2023-x86_64-standard-1.31-v20250116 as of 2025/01/28),
    # we need this for backward-compatibility.
    http_put_response_hop_limit = 2
  }
  labels = {
    "role" = "spot"
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

// node role with s3 write access for fluentd
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

resource "aws_iam_role_policy" "node" {
    name = "${var.prefix}-node"
    role = aws_iam_role.node.id
    policy = <<EOF
{
    "Statement": [{
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketLocation",
                "s3:ListBucketMultipartUploads",
                "s3:ListBucketVersions"
            ],
            "Effect": "Allow",
            "Resource": [
                "arn:aws:s3:::${aws_s3_bucket.bucket.id}"
            ]
        },
        {
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:AbortMultipartUpload",
                "s3:ListMultipartUploadParts"
            ],
            "Effect": "Allow",
            "Resource": [
                "arn:aws:s3:::${aws_s3_bucket.bucket.id}/*"
            ]
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

resource "aws_security_group" "node" {
    name = "${var.prefix}-node"
    vpc_id = data.aws_vpc.vpc.id
}

resource "aws_security_group_rule" "node_ingress" {
    security_group_id = aws_security_group.node.id
    type = "ingress"
    from_port = 0
    to_port = 0
    protocol = "-1"
    source_security_group_id = aws_security_group.cluster.id
}

resource "aws_security_group_rule" "node_egress" {
    security_group_id = aws_security_group.node.id
    type = "egress"
    from_port = 0
    to_port = 0
    protocol = "-1"
    cidr_blocks = ["0.0.0.0/0"]
}

resource "aws_security_group_rule" "node_ingress_self" {
    security_group_id = aws_security_group.node.id
    type = "ingress"
    from_port = 0
    to_port = 0
    protocol = "-1"
    self = true
}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_subnet" "public" {
    count = 2
    vpc_id = data.aws_vpc.vpc.id
    cidr_block = "${cidrsubnet(data.aws_vpc.vpc.cidr_block, 4, 12+count.index)}"
    availability_zone = data.aws_availability_zones.available.names[count.index%length(data.aws_availability_zones.available.names)]
    map_public_ip_on_launch = true
}
