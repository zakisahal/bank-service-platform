variable "name" {
  type        = string
  description = "Path prefix for the secret, e.g. the service name."
}

variable "kms_key_arn" {
  type = string
}

variable "db_username" {
  type = string
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "db_endpoint" {
  type = string
}

variable "db_port" {
  type    = number
  default = 5432
}

variable "db_name" {
  type = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
