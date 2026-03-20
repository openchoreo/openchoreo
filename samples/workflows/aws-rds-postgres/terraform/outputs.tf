output "db_address" {
  description = "Hostname of the RDS instance (without port)"
  value       = aws_db_instance.this.address
}

output "db_port" {
  description = "Port the database is listening on"
  value       = aws_db_instance.this.port
}

output "db_endpoint" {
  description = "Connection endpoint in address:port format"
  value       = aws_db_instance.this.endpoint
}

output "db_name" {
  description = "Name of the initial database"
  value       = aws_db_instance.this.db_name
}

output "db_username" {
  description = "Master username"
  value       = aws_db_instance.this.username
}

output "db_arn" {
  description = "ARN of the RDS instance"
  value       = aws_db_instance.this.arn
}
