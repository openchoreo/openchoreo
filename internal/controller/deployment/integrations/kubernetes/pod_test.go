/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/ptr"
)

var _ = Describe("makePodSpec", func() {
	var (
		deployCtx *dataplane.DeploymentContext
		podSpec   *corev1.PodSpec
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		podSpec = makePodSpec(deployCtx)
	})

	Context("for a Service component", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
		})

		It("should create a PodSpec with correct RestartPolicy", func() {
			Expect(podSpec.RestartPolicy).To(Equal(corev1.RestartPolicyAlways))
		})
	})

	Context("for a Scheduled Task component", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeScheduledTask
		})

		It("should create a PodSpec with correct RestartPolicy", func() {
			Expect(podSpec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
		})
	})

	Context("when the deployable artifact has direct environment variables", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					Env: []choreov1.EnvVar{
						{
							Key:   "LOG_FORMAT",
							Value: "json",
						},
					},
				},
			}
		})

		It("should create a PodSpec with correct environment variables", func() {
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].Env).To(ConsistOf(
				corev1.EnvVar{
					Name:  "LOG_FORMAT",
					Value: "json",
				},
			))
		})
	})

	Context("when the deployable artifact has direct file mounts", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					FileMounts: []choreov1.FileMount{
						{
							MountPath: "/app/config.json",
							Value:     "{\"key\":\"value\"}",
						},
					},
				},
			}
		})

		It("should create a PodSpec with correct file volume and a mount", func() {
			By("checking the volumes")
			Expect(podSpec.Volumes).To(HaveLen(1))
			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("filemount-4ea60343"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-filemount-4ea60343-a80bc9b2",
						},
					},
				}),
			})))

			By("checking the volume mounts")
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("filemount-4ea60343"),
				"MountPath": Equal("/app/config.json"),
				"SubPath":   Equal("content"),
			})))
		})
	})

	Context("when the deployable artifact has environment variables mapped from configuration groups", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					Env: []choreov1.EnvVar{
						{
							Key: "REDIS_HOST",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "host",
								},
							},
						},
						{
							Key: "REDIS_PORT",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "port",
								},
							},
						},
						{
							Key: "REDIS_PASSWORD",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "password",
								},
							},
						},
						{
							Key: "MYSQL_HOST",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "host",
								},
							},
						},
						{
							Key: "MYSQL_PORT",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "port",
								},
							},
						},
						{
							Key: "MYSQL_PASSWORD",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "password",
								},
							},
						},
					},
				},
			}

			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create a PodSpec with correct environment variables", func() {
			Expect(podSpec.Containers).To(HaveLen(1))

			envs := podSpec.Containers[0].Env
			By("checking the environment variables count")
			Expect(envs).To(HaveLen(6))

			By("checking the REDIS_HOST environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("REDIS_HOST"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
						Key: "host",
					},
				}),
			})))

			By("checking the REDIS_PASSWORD environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("REDIS_PASSWORD"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
						Key: "password",
					},
				}),
			})))

			By("checking the MYSQL_PORT environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("MYSQL_PORT"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-mysql-config-group-e7d2f2be",
						},
						Key: "port",
					},
				}),
			})))

			By("checking the MYSQL_PASSWORD environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("MYSQL_PASSWORD"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-mysql-config-group-e7d2f2be",
						},
						Key: "password",
					},
				}),
			})))
		})
	})

	Context("when the deployable artifact has file mounts mapped from configuration groups", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					FileMounts: []choreov1.FileMount{
						{
							MountPath: "/config/redis-host",
							ValueFrom: &choreov1.FileMountValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "host",
								},
							},
						},
						{
							MountPath: "/config/redis-port",
							ValueFrom: &choreov1.FileMountValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "port",
								},
							},
						},
						{
							MountPath: "/config/redis-password",
							ValueFrom: &choreov1.FileMountValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "password",
								},
							},
						},
						{
							MountPath: "/config/mysql-host",
							ValueFrom: &choreov1.FileMountValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "host",
								},
							},
						},
						{
							MountPath: "/config/mysql-port",
							ValueFrom: &choreov1.FileMountValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "port",
								},
							},
						},
						{
							MountPath: "/config/mysql-password",
							ValueFrom: &choreov1.FileMountValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "password",
								},
							},
						},
					},
				},
			}

			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create a PodSpec with correct volume mounts", func() {
			By("checking the volume count")
			Expect(podSpec.Volumes).To(HaveLen(4))

			By("checking the volumes")
			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("redis-config-group-cm-2551b38a"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
					},
				}),
			})))

			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("redis-config-group-csi-5fdf4f87"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "secrets-store.csi.k8s.io",
						ReadOnly: ptr.Bool(true),
						VolumeAttributes: map[string]string{
							"secretProviderClass": "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
					},
				}),
			})))

			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("mysql-config-group-cm-6e0397eb"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-mysql-config-group-e7d2f2be",
						},
					},
				}),
			})))

			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("mysql-config-group-csi-c00642f6"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "secrets-store.csi.k8s.io",
						ReadOnly: ptr.Bool(true),
						VolumeAttributes: map[string]string{
							"secretProviderClass": "my-component-my-main-track-mysql-config-group-e7d2f2be",
						},
					},
				}),
			})))

			By("checking the volume mounts")
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(6))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("redis-config-group-cm-2551b38a"),
				"MountPath": Equal("/config/redis-host"),
				"SubPath":   Equal("host"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("redis-config-group-cm-2551b38a"),
				"MountPath": Equal("/config/redis-port"),
				"SubPath":   Equal("port"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("redis-config-group-csi-5fdf4f87"),
				"MountPath": Equal("/config/redis-password"),
				"SubPath":   Equal("password"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("mysql-config-group-cm-6e0397eb"),
				"MountPath": Equal("/config/mysql-host"),
				"SubPath":   Equal("host"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("mysql-config-group-cm-6e0397eb"),
				"MountPath": Equal("/config/mysql-port"),
				"SubPath":   Equal("port"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("mysql-config-group-csi-c00642f6"),
				"MountPath": Equal("/config/mysql-password"),
				"SubPath":   Equal("password"),
			})))

		})
	})

	// Bulk mapping means that the entire configuration group is mapped to the environment variables
	// without specifying the individual keys. The generator must sanitize the keys.
	// Example Configuration group injection:
	// envFrom:
	//   - configurationGroupRef:
	//       name: redis-config
	Context("when the deployable artifact has environment variables bulk mapping a configuration group", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					EnvFrom: []choreov1.EnvFromSource{
						{
							ConfigurationGroupRef: &choreov1.ConfigurationGroupRef{
								Name: "redis-config-group",
							},
						},
					},
				},
			}

			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create a PodSpec with correct environment variables", func() {
			Expect(podSpec.Containers).To(HaveLen(1))

			envs := podSpec.Containers[0].Env
			By("checking the environment variables count")
			Expect(envs).To(HaveLen(3))

			By("checking the sanitized 'host' environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("HOST"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
						Key: "host",
					},
				}),
			})))

			By("checking the `port` environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("PORT"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
						Key: "port",
					},
				}),
			})))

			By("checking the `password` environment variable")
			By("checking the `password` environment variable")
			Expect(envs).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("PASSWORD"),
				"ValueFrom": BeComparableTo(&corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
						Key: "password",
					},
				}),
			})))
		})
	})

	// Bulk mapping means that the entire configuration group is mapped as file mounts
	// without specifying the individual keys. The generator must sanitize the keys.
	// Example Configuration group injection:
	// fileMountsFrom:
	//   - configurationGroupRef:
	//       name: redis-config
	//		 mountPath: /redis-config
	Context("when the deployable artifact has file mount bulk mapping from a configuration group", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					FileMountsFrom: []choreov1.FileMountsFromSource{
						{
							ConfigurationGroupRef: &choreov1.ConfigurationGroupMountRef{
								Name:      "redis-config-group",
								MountPath: "/config",
							},
						},
					},
				},
			}

			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create a PodSpec with correct volume mounts", func() {
			By("checking the volume count")
			Expect(podSpec.Volumes).To(HaveLen(2))

			By("checking the volumes")
			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("redis-config-group-cm-2551b38a"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
					},
				}),
			})))

			Expect(podSpec.Volumes).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name": Equal("redis-config-group-csi-5fdf4f87"),
				"VolumeSource": BeComparableTo(corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "secrets-store.csi.k8s.io",
						ReadOnly: ptr.Bool(true),
						VolumeAttributes: map[string]string{
							"secretProviderClass": "my-component-my-main-track-redis-config-group-b8ef9df9",
						},
					},
				}),
			})))

			By("checking the volume mounts")
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(3))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("redis-config-group-cm-2551b38a"),
				"MountPath": Equal("/config/host"),
				"SubPath":   Equal("host"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("redis-config-group-cm-2551b38a"),
				"MountPath": Equal("/config/port"),
				"SubPath":   Equal("port"),
			})))

			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("redis-config-group-csi-5fdf4f87"),
				"MountPath": Equal("/config/password"),
				"SubPath":   Equal("password"),
			})))
		})
	})
})
