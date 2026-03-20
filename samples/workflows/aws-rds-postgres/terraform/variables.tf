variable "aws_region" {
  description = "AWS region where the RDS instance will be created"
  type        = string
  default     = "us-east-1"
}

variable "db_identifier" {
  description = "Unique identifier for the RDS instance (also used as the Terraform state key)"
  type        = string
}

variable "db_name" {
  description = "Name of the initial database to create inside the instance"
  type        = string
}

variable "db_username" {
  description = "Master username for the database"
  type        = string
}

variable "db_password" {
  description = "Master password for the database"
  type        = string
  sensitive   = true
}

variable "db_engine_version" {
  description = "PostgreSQL engine major version"
  type        = string
  default     = "16"
}
