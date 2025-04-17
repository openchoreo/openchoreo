#!/usr/bin/env bash

checkCommand() {
  cmdName=$1
  if ! command -v "${cmdName}" >/dev/null; then
    echo -e "\e[31mRequired command '${cmdName}' could not be found\e[0m"
    exit 1
  fi
}

checkVersion() {
  commandName=$1
  minVersion=$2
  maxVersion=$3
  versionCommand=$4
  versionLine=$5
  versionExtractRegEx=$6

  cmd=$(echo "${versionCommand}" | cut -d " " -f1)
  checkCommand "$cmd"

  currentVersion=$(eval "${versionCommand}" | sed -n "${versionLine}"p | grep -Eo "${versionExtractRegEx}")

  if [ "$(printf '%s\n%s\n%s\n' "${minVersion}" "${currentVersion}" "${maxVersion}" | sort -V | sed -n 2p)" = "${currentVersion}" ]; then
    echo -e "\033[32mThe '${commandName}' version '${currentVersion}' is OK\033[0m"
  else
    echo -e "\033[31mThe '${commandName}' version '${currentVersion}' needs to be in between '${minVersion}' and '${maxVersion}'\033[0m"
    exit 1
  fi
}

# Format: <Command Display Name> <Min Version> <Max Version> <Version Command Line> <Version Line Number> <Version Extract RegEx>
checkVersion "Go Language" "1.24.0" "1.25.0" "go version" 1 "[0-9]\.[0-9]+\.[0-9]+"
checkVersion "GNU Make" "3.8" "4.5" "make -version" 1 "[0-9]\.[0-9]+"
checkVersion "Kind" "v0.27.0" "v0.27.0" "kind version" 1 "v[0-9]\.[0-9]+\.[0-9]+"
checkVersion "Docker Client" "23.0.0" "28.0.0" "docker version --format '{{.Client.Version}}'" 1 "[0-9]+\.[0-9]+\.[0-9]+"
checkVersion "Docker Server" "23.0.0" "28.0.0" "docker version --format '{{.Server.Version}}'" 1 "[0-9]+\.[0-9]+\.[0-9]+"
checkVersion "Kubectl Client" "v1.31.0" "v1.33.0" "kubectl version" 1 "v[0-9]\.[0-9]+\.[0-9]+"
checkVersion "Kubectl Server (context=$(kubectl config current-context))" "v1.31.0" "v1.33.0" "kubectl version" 3 "v[0-9]\.[0-9]+\.[0-9]+"
checkVersion "Kubebuilder" "4.3.0" "4.4.0" "kubebuilder version" 1 "[0-9]+\.[0-9]+\.[0-9]+"
checkVersion "Helm" "v3.16.0" "v3.30.0" "helm version" 1 "v[0-9]+\.[0-9]+\.[0-9]+"
