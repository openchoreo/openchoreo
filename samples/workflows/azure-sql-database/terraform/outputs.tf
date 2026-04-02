output "server_fqdn" {
  description = "Fully qualified domain name of the SQL Server"
  value       = azurerm_mssql_server.this.fully_qualified_domain_name
}

output "server_name" {
  description = "Name of the SQL Server"
  value       = azurerm_mssql_server.this.name
}

output "db_name" {
  description = "Name of the SQL Database"
  value       = azurerm_mssql_database.this.name
}

output "admin_username" {
  description = "Administrator login"
  value       = azurerm_mssql_server.this.administrator_login
}

output "server_id" {
  description = "Resource ID of the SQL Server"
  value       = azurerm_mssql_server.this.id
}

output "db_id" {
  description = "Resource ID of the SQL Database"
  value       = azurerm_mssql_database.this.id
}
