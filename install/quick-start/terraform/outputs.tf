output "cilium_status" {
  description = "Status of the Cilium Helm release"
  value       = helm_release.cilium.status
}

output "choreo_dataplane_status" {
  description = "Status of the Choreo DataPlane Helm release"
  value       = helm_release.choreo-dp.status
}

output "choreo_control_plane_status" {
  description = "Status of the Choreo ControlPlane Helm release"
  value       = helm_release.choreo-cp.status
}
