variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "environment" {
  type    = string
  default = "dev"
}

variable "project" {
  type    = string
  default = "bank-service"
}

variable "vpc_cidr" {
  type    = string
  default = "10.20.0.0/16"
}

variable "availability_zones" {
  type    = list(string)
  default = ["us-east-1a", "us-east-1b"]
}

variable "public_subnet_cidrs" {
  type    = list(string)
  default = ["10.20.0.0/24", "10.20.1.0/24"]
}

variable "private_subnet_cidrs" {
  type    = list(string)
  default = ["10.20.10.0/24", "10.20.11.0/24"]
}

variable "kubernetes_version" {
  type    = string
  default = "1.30"
}

variable "eks_endpoint_public_access" {
  type    = bool
  default = true
}

variable "rds_multi_az" {
  type        = bool
  default     = false
  description = "Off in dev to save cost; on in prod."
}

variable "rds_deletion_protection" {
  type    = bool
  default = false
}

variable "k8s_namespace" {
  type    = string
  default = "bank"
}

variable "k8s_service_account_name" {
  type    = string
  default = "bank-service"
}

variable "enable_github_oidc" {
  type    = bool
  default = false
}

variable "github_org" {
  type    = string
  default = "your-org"
}

variable "github_repo" {
  type    = string
  default = "bank-service"
}

# See providers.tf: only relevant because this exercise plans without real
# AWS credentials. Always true here; a real environment never sets this.
variable "offline_demo" {
  type    = bool
  default = true
}
