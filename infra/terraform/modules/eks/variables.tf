variable "name" {
  type        = string
  description = "Cluster name."
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "kubernetes_version" {
  type    = string
  default = "1.30"
}

variable "endpoint_public_access" {
  type        = bool
  default     = true
  description = "Whether the EKS API server has a public endpoint. False + VPN/bastion is the recommended prod posture for a regulated workload."
}

variable "kms_key_arn" {
  type        = string
  description = "CMK used for Kubernetes Secret envelope encryption and control-plane log encryption."
}

variable "log_retention_days" {
  type    = number
  default = 90
}

variable "node_instance_types" {
  type    = list(string)
  default = ["m6i.large"]
}

variable "node_desired_size" {
  type    = number
  default = 2
}

variable "node_min_size" {
  type    = number
  default = 2
}

variable "node_max_size" {
  type    = number
  default = 4
}

variable "tags" {
  type    = map(string)
  default = {}
}
