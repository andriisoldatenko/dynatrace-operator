#!/bin/bash
set -x

if [ -z "$2" ]
then
  echo "Usage: $0 <image_name> <image_tag> <enable_multiplatform>"
  exit 1
fi

image_name=$1
image_tag=$2
multiplatform=$3
image="${image_name}:${image_tag}"



if [ "$multiplatform" == "true" ]
then
#  supported_architectures=("amd64" "arm64" "ppc64le" "s390x")
  supported_architectures=("amd64")
  images=()
  echo "Creating manifest for ${supported_architectures[*]}"

  for architecture in "${supported_architectures[@]}"
  do
    docker pull "${image}-${architecture}"
    images+=("${image}-${architecture}")
  done
  docker manifest create "${image}" "${images[@]}"
  if [[ "$image" =~ gcr.io ]]
  then
    go install github.com/google/go-containerregistry/cmd/crane@latest
    crane mutate "${image}" --annotation "com.googleapis.cloudmarketplace.product.service.name=services/dynatrace-operator-dynatrace-marketplace-prod.cloudpartnerservices.goog"
    docker manifest inspect "${image}"
  fi
else
  echo "Creating manifest for the AMD image "
  docker pull "${image}-amd64"
  docker manifest create "${image}" "${image}-amd64"
  if [[ "$image" =~ gcr.io ]]
  then
    go install github.com/google/go-containerregistry/cmd/crane@latest
    crane mutate "${image}" --annotation "com.googleapis.cloudmarketplace.product.service.name=services/dynatrace-operator-dynatrace-marketplace-prod.cloudpartnerservices.goog"
    docker manifest inspect "${image}"
  fi
fi

sha256=$(docker manifest push "${image}")
echo "digest=${sha256}">> "$GITHUB_OUTPUT"
