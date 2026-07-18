variable "name" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "allowed_security_group_ids" {
  type        = list(string)
  description = "Security groups (e.g. the EKS node SG) permitted to reach Postgres on 5432."
}

variable "engine_version" {
  type    = string
  default = "16.4"
}

variable "instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "allocated_storage" {
  type    = number
  default = 20
}

variable "database_name" {
  type    = string
  default = "bank"
}

variable "master_username" {
  type    = string
  default = "bank_app"
}

variable "master_password" {
  type      = string
  sensitive = true
}

variable "kms_key_arn" {
  type = string
}

variable "multi_az" {
  type        = bool
  default     = false
  description = "Enable Multi-AZ for automatic failover. Off by default in dev to save cost; on in prod - see DECISIONS.md."
}

variable "backup_retention_period" {
  type    = number
  default = 7
}

variable "deletion_protection" {
  type    = bool
  default = false
}

variable "parameter_group_family" {
  type    = string
  default = "postgres16"
}

variable "tags" {
  type    = map(string)
  default = {}
}
