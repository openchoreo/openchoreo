# Builds the sample app and imports it into the local k3d cluster.
#
#   make -C samples/progressive-delivery/app images

K3D_CLUSTER_NAME ?= openchoreo
IMAGE ?= openchoreo-samples/pd-app:local

.PHONY: images

images:
	docker build -t $(IMAGE) .
	k3d image import -c $(K3D_CLUSTER_NAME) $(IMAGE)
