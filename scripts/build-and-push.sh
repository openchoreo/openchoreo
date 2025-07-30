#!/bin/bash

# Build and push script for Backstage OpenChoreo image
# Don't exit on errors initially, we'll handle them manually
set +e

# Configuration
IMAGE_NAME=${IMAGE_NAME:-"backstage-demo"}
IMAGE_TAG=${IMAGE_TAG:-"latest-dev"}
REGISTRY=${REGISTRY:-""}

# Build the full image name
if [ -n "$REGISTRY" ]; then
    FULL_IMAGE_NAME="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
else
    FULL_IMAGE_NAME="${IMAGE_NAME}:${IMAGE_TAG}"
fi

echo "Building Backstage OpenChoreo image: ${FULL_IMAGE_NAME}"

# Build the backend first
echo "Building backend..."

# Install dependencies - handle native module build failures gracefully
echo "Installing dependencies..."
yarn install --immutable
install_result=$?

if [ $install_result -ne 0 ]; then
    echo "Warning: Some dependencies failed to install (likely native modules)."
    echo "This is common with better-sqlite3 and isolated-vm on some systems."
    echo "The build will continue as these are optional for the Docker build."
fi

echo "Running TypeScript compilation..."
yarn tsc
tsc_result=$?

if [ $tsc_result -ne 0 ]; then
    echo "Error: TypeScript compilation failed. Exiting."
    exit 1
fi

echo "Building backend bundle..."
yarn build:backend
build_result=$?

if [ $build_result -ne 0 ]; then
    echo "Error: Backend build failed. Exiting."
    exit 1
fi

# Build the Docker image
echo "Building Docker image..."
yarn build-image
docker_result=$?

if [ $docker_result -ne 0 ]; then
    echo "Error: Docker image build failed. Exiting."
    exit 1
fi

# Tag the image with the desired name
echo "Tagging image as ${FULL_IMAGE_NAME}..."
docker tag backstage "${FULL_IMAGE_NAME}"
tag_result=$?

if [ $tag_result -ne 0 ]; then
    echo "Error: Failed to tag Docker image. Exiting."
    exit 1
fi

# Push if registry is specified
if [ -n "$REGISTRY" ]; then
    echo "Pushing image to registry..."
    docker push "${FULL_IMAGE_NAME}"
    push_result=$?
    
    if [ $push_result -ne 0 ]; then
        echo "Error: Failed to push image to registry. Exiting."
        exit 1
    fi
    
    echo "Image pushed successfully: ${FULL_IMAGE_NAME}"
else
    echo "No registry specified, skipping push. Image available locally as: ${FULL_IMAGE_NAME}"
fi

echo "Build complete!"
echo "To use with Helm chart, update values.yaml:"
echo "  backstage:"
echo "    image:"
echo "      registry: \"${REGISTRY}\""
echo "      repository: \"${IMAGE_NAME}\""  
echo "      tag: \"${IMAGE_TAG}\""
