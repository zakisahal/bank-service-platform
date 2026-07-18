variable "name" {
  type        = string
  description = "ECR repository name."
}

variable "kms_key_arn" {
  type        = string
  description = "KMS CMK ARN used to encrypt images at rest."
}

variable "tags" {
  type    = map(string)
  default = {}
}
