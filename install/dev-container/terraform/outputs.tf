output "cilium_status" {
  description = "Status of the Cilium Helm release"
  value       = helm_release.cilium.status
}

output "choreo_status" {
  description = "Status of the Choreo Helm release"
  value       = helm_release.choreo.status
}
