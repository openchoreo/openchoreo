output "cilium_status" {
  description = "Status of the Cilium Helm release"
  value       = helm_release.cilium.status
}

output "openchoreo_dataplane_status" {
  description = "Status of the openchoreo-data-plane Helm release"
  value       = helm_release.openchoreo-data-plane.status
}

output "openchoreo_control_plane_status" {
  description = "Status of the openchoreo-control-plane Helm release"
  value       = helm_release.openchoreo-control-plane.status
}

output "openchoreo_build_plane_status" {
  description = "Status of the openchoreo-build-plane Helm release"
  value       = helm_release.openchoreo-build-plane.status
}

output "openchoreo_observability_plane_status" {
  description = "Status of the openchoreo-observability-plane Helm release"
  value       = var.enable-observability-plane ? helm_release.openchoreo-observability-plane[0].status : "disabled"
}

output "openchoreo_identity_provider_status" {
  description = "Status of the openchoreo-identity-provider Helm release"
  value       = helm_release.openchoreo-identity-provider.status
}
