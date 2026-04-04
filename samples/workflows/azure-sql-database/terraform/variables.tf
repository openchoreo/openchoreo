variable "resource_group_name" {
  description = "Name of the Azure Resource Group to create resources in"
  type        = string
}

variable "server_name" {
  description = "Unique name for the Azure SQL Server (must be globally unique)"
  type        = string
}

variable "db_name" {
  description = "Name of the SQL Database to create on the server"
  type        = string
}

variable "admin_username" {
  description = "Administrator login for the SQL Server"
  type        = string
}

variable "admin_password" {
  description = "Administrator password for the SQL Server"
  type        = string
  sensitive   = true
}

variable "db_sku" {
  description = "SKU name for the database (e.g. 'Basic', 'S0', 'GP_S_Gen5_1')"
  type        = string
  default     = "Basic"
}
