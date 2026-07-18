terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }

  # Real environments use a remote backend so state is shared and locked:
  #
  #   backend "s3" {
  #     bucket         = "bank-platform-tfstate"
  #     key            = "dev/bank-service.tfstate"
  #     region         = "us-east-1"
  #     dynamodb_table = "bank-platform-tflock"
  #     encrypt        = true
  #   }
  #
  # Left as the local backend for this exercise since we're not applying
  # against a real account - see DECISIONS.md.
}
